package release

import (
	"net/http"

	"github.com/porter-dev/porter/api/server/authz"
	"github.com/porter-dev/porter/api/server/handlers"
	"github.com/porter-dev/porter/api/server/shared"
	"github.com/porter-dev/porter/api/server/shared/apierrors"
	"github.com/porter-dev/porter/api/types"
	"github.com/porter-dev/porter/internal/models"
	"github.com/porter-dev/porter/internal/templater/parser"
	"helm.sh/helm/v3/pkg/release"
)

type ReleaseGetHandler struct {
	handlers.PorterHandlerWriter
	authz.KubernetesAgentGetter
}

func NewReleaseGetHandler(
	config *shared.Config,
	writer shared.ResultWriter,
) *ReleaseGetHandler {
	return &ReleaseGetHandler{
		PorterHandlerWriter:   handlers.NewDefaultPorterHandler(config, nil, writer),
		KubernetesAgentGetter: authz.NewOutOfClusterAgentGetter(config),
	}
}

func (c *ReleaseGetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	helmRelease, _ := r.Context().Value(types.ReleaseScope).(*release.Release)

	res := &types.Release{
		HelmRelease: helmRelease,
	}

	// look up the release in the database; if not found, do not populate Porter fields
	cluster, _ := r.Context().Value(types.ClusterScope).(*models.Cluster)
	release, err := c.Repo().Release().ReadRelease(cluster.ID, helmRelease.Name, helmRelease.Namespace)

	if err == nil {
		res.ID = release.ID
		res.WebhookToken = release.WebhookToken

		if release.GitActionConfig != nil {
			res.GitActionConfig = release.GitActionConfig.ToGitActionConfigType()
		}
	}

	// look for the form using the dynamic client
	dynClient, err := c.GetDynamicClient(r, cluster)

	if err != nil {
		c.HandleAPIError(w, apierrors.NewErrInternal(err))
	}

	parserDef := &parser.ClientConfigDefault{
		DynamicClient: dynClient,
		HelmChart:     helmRelease.Chart,
		HelmRelease:   helmRelease,
	}

	form, err := parser.GetFormFromRelease(parserDef, helmRelease)

	if err != nil {
		// TODO: log non-fatal parsing error
	} else {
		res.Form = form
	}

	c.WriteResult(w, res)
}
