package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi"

	"github.com/porter-dev/porter/internal/forms"
	"github.com/porter-dev/porter/internal/kubernetes"
	"github.com/porter-dev/porter/internal/kubernetes/provisioner"
	"github.com/porter-dev/porter/internal/models"

	"github.com/porter-dev/porter/internal/adapter"
)

// HandleProvisionTest will create a test resource by deploying a provisioner
// container pod
func (app *App) HandleProvisionTest(w http.ResponseWriter, r *http.Request) {
	projID, err := strconv.ParseUint(chi.URLParam(r, "project_id"), 0, 64)

	if err != nil || projID == 0 {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	// create a new agent
	agent, err := kubernetes.GetAgentInClusterConfig()

	if err != nil {
		app.handleErrorDataRead(err, w)
		return
	}

	_, err = agent.ProvisionTest(
		uint(projID),
		provisioner.Apply,
		&provisioner.PostgresConf{
			Host: app.DBConf.Host,
			Port: app.DBConf.Port,
		},
		app.RedisConf,
	)

	if err != nil {
		app.handleErrorInternal(err, w)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleProvisionAWSECRInfra provisions a new aws ECR instance for a project
func (app *App) HandleProvisionAWSECRInfra(w http.ResponseWriter, r *http.Request) {
	projID, err := strconv.ParseUint(chi.URLParam(r, "project_id"), 0, 64)

	if err != nil || projID == 0 {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	form := &forms.CreateECRInfra{
		ProjectID: uint(projID),
	}

	// decode from JSON to form value
	if err := json.NewDecoder(r.Body).Decode(form); err != nil {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	// validate the form
	if err := app.validator.Struct(form); err != nil {
		app.handleErrorFormValidation(err, ErrProjectValidateFields, w)
		return
	}

	// convert the form to an aws infra instance
	infra, err := form.ToAWSInfra()

	if err != nil {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	// handle write to the database
	infra, err = app.Repo.AWSInfra.CreateAWSInfra(infra)

	if err != nil {
		app.handleErrorDataWrite(err, w)
		return
	}

	awsInt, err := app.Repo.AWSIntegration.ReadAWSIntegration(infra.AWSIntegrationID)

	if err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorDataRead(err, w)
		return
	}

	// launch provisioning pod
	agent, err := kubernetes.GetAgentInClusterConfig()

	if err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorDataRead(err, w)
		return
	}

	_, err = agent.ProvisionECR(
		uint(projID),
		awsInt,
		form.ECRName,
		infra,
		provisioner.Apply,
		&provisioner.PostgresConf{
			Host: app.DBConf.Host,
			Port: app.DBConf.Port,
		},
		app.RedisConf,
	)

	if err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorInternal(err, w)
		return
	}

	app.Logger.Info().Msgf("New aws ecr infra created: %d", infra.ID)

	w.WriteHeader(http.StatusCreated)

	infraExt := infra.Externalize()

	if err := json.NewEncoder(w).Encode(infraExt); err != nil {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}
}

// HandleDestroyAWSECRInfra destroys ecr infra
func (app *App) HandleDestroyAWSECRInfra(w http.ResponseWriter, r *http.Request) {
	// get path parameters
	infraID, err := strconv.ParseUint(chi.URLParam(r, "infra_id"), 10, 64)

	if err != nil {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	// read infra to get id
	infra, err := app.Repo.AWSInfra.ReadAWSInfra(uint(infraID))

	if err != nil {
		app.handleErrorDataRead(err, w)
		return
	}

	awsInt, err := app.Repo.AWSIntegration.ReadAWSIntegration(infra.AWSIntegrationID)

	form := &forms.DestroyECRInfra{}

	// decode from JSON to form value
	if err := json.NewDecoder(r.Body).Decode(form); err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	// validate the form
	if err := app.validator.Struct(form); err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorFormValidation(err, ErrProjectValidateFields, w)
		return
	}

	// launch provisioning destruction pod
	agent, err := kubernetes.GetAgentInClusterConfig()

	if err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorDataRead(err, w)
		return
	}

	// mark infra for deletion
	infra.Status = models.StatusDestroying
	infra, err = app.Repo.AWSInfra.UpdateAWSInfra(infra)

	if err != nil {
		app.handleErrorDataWrite(err, w)
		return
	}

	_, err = agent.ProvisionECR(
		infra.ProjectID,
		awsInt,
		form.ECRName,
		infra,
		provisioner.Destroy,
		&provisioner.PostgresConf{
			Host: app.DBConf.Host,
			Port: app.DBConf.Port,
		},
		app.RedisConf,
	)

	if err != nil {
		app.handleErrorInternal(err, w)
		return
	}

	app.Logger.Info().Msgf("AWS ECR infra marked for destruction: %d", infra.ID)

	w.WriteHeader(http.StatusOK)
}

// HandleProvisionAWSEKSInfra provisions a new aws EKS instance for a project
func (app *App) HandleProvisionAWSEKSInfra(w http.ResponseWriter, r *http.Request) {
	projID, err := strconv.ParseUint(chi.URLParam(r, "project_id"), 0, 64)

	if err != nil || projID == 0 {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	form := &forms.CreateEKSInfra{
		ProjectID: uint(projID),
	}

	// decode from JSON to form value
	if err := json.NewDecoder(r.Body).Decode(form); err != nil {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	// validate the form
	if err := app.validator.Struct(form); err != nil {
		app.handleErrorFormValidation(err, ErrProjectValidateFields, w)
		return
	}

	// convert the form to an aws infra instance
	infra, err := form.ToAWSInfra()

	if err != nil {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	// handle write to the database
	infra, err = app.Repo.AWSInfra.CreateAWSInfra(infra)

	if err != nil {
		app.handleErrorDataWrite(err, w)
		return
	}

	awsInt, err := app.Repo.AWSIntegration.ReadAWSIntegration(infra.AWSIntegrationID)

	if err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorDataRead(err, w)
		return
	}

	// launch provisioning pod
	agent, err := kubernetes.GetAgentInClusterConfig()

	if err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorDataRead(err, w)
		return
	}

	_, err = agent.ProvisionEKS(
		uint(projID),
		awsInt,
		form.EKSName,
		infra,
		provisioner.Apply,
		&provisioner.PostgresConf{
			Host: app.DBConf.Host,
			Port: app.DBConf.Port,
		},
		app.RedisConf,
	)

	if err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorInternal(err, w)
		return
	}

	app.Logger.Info().Msgf("New aws eks infra created: %d", infra.ID)

	w.WriteHeader(http.StatusCreated)

	infraExt := infra.Externalize()

	if err := json.NewEncoder(w).Encode(infraExt); err != nil {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}
}

// HandleDestroyAWSEKSInfra destroys eks infra
func (app *App) HandleDestroyAWSEKSInfra(w http.ResponseWriter, r *http.Request) {
	// get path parameters
	infraID, err := strconv.ParseUint(chi.URLParam(r, "infra_id"), 10, 64)

	if err != nil {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	// read infra to get id
	infra, err := app.Repo.AWSInfra.ReadAWSInfra(uint(infraID))

	if err != nil {
		app.handleErrorDataRead(err, w)
		return
	}

	awsInt, err := app.Repo.AWSIntegration.ReadAWSIntegration(infra.AWSIntegrationID)

	form := &forms.DestroyEKSInfra{}

	// decode from JSON to form value
	if err := json.NewDecoder(r.Body).Decode(form); err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	// validate the form
	if err := app.validator.Struct(form); err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorFormValidation(err, ErrProjectValidateFields, w)
		return
	}

	// launch provisioning destruction pod
	agent, err := kubernetes.GetAgentInClusterConfig()

	if err != nil {
		infra.Status = models.StatusError
		infra, _ = app.Repo.AWSInfra.UpdateAWSInfra(infra)

		app.handleErrorDataRead(err, w)
		return
	}

	// mark infra for deletion
	infra.Status = models.StatusDestroying
	infra, err = app.Repo.AWSInfra.UpdateAWSInfra(infra)

	if err != nil {
		app.handleErrorDataWrite(err, w)
		return
	}

	_, err = agent.ProvisionEKS(
		infra.ProjectID,
		awsInt,
		form.EKSName,
		infra,
		provisioner.Destroy,
		&provisioner.PostgresConf{
			Host: app.DBConf.Host,
			Port: app.DBConf.Port,
		},
		app.RedisConf,
	)

	if err != nil {
		app.handleErrorInternal(err, w)
		return
	}

	app.Logger.Info().Msgf("AWS EKS infra marked for destruction: %d", infra.ID)

	w.WriteHeader(http.StatusOK)
}

// HandleGetProvisioningLogs returns real-time logs of the provisioning process via websockets
func (app *App) HandleGetProvisioningLogs(w http.ResponseWriter, r *http.Request) {
	// get path parameters
	infraID, err := strconv.ParseUint(chi.URLParam(r, "infra_id"), 10, 64)

	if err != nil {
		app.handleErrorFormDecoding(err, ErrProjectDecode, w)
		return
	}

	// read infra to get id
	infra, err := app.Repo.AWSInfra.ReadAWSInfra(uint(infraID))

	if err != nil {
		app.handleErrorDataRead(err, w)
		return
	}

	streamName := infra.GetID()

	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	// upgrade to websocket.
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		app.handleErrorUpgradeWebsocket(err, w)
	}

	client, err := adapter.NewRedisClient(app.RedisConf)

	if err != nil {
		app.handleErrorInternal(err, w)
		return
	}

	err = provisioner.ResourceStream(client, streamName, conn)

	if err != nil {
		app.handleErrorWebsocketWrite(err, w)
		return
	}
}
