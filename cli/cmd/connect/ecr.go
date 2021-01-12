package connect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/porter-dev/porter/cli/cmd/api"
	awsLocal "github.com/porter-dev/porter/cli/cmd/providers/aws/local"
	"github.com/porter-dev/porter/cli/cmd/utils"
)

// ECR creates an ECR integration
func ECR(
	client *api.Client,
	projectID uint,
) (uint, error) {
	// if project ID is 0, ask the user to set the project ID or create a project
	if projectID == 0 {
		return 0, fmt.Errorf("no project set, please run porter project set [id]")
	}

	// query for the region
	region, err := utils.PromptPlaintext(fmt.Sprintf(`Please provide the AWS region where the ECR instance is located.
AWS Region: `))

	if err != nil {
		return 0, err
	}

	userResp, err := utils.PromptPlaintext(
		fmt.Sprintf(`Porter can set up an IAM user in your AWS account to connect to this ECR instance automatically.
Would you like to proceed? %s `,
			color.New(color.FgCyan).Sprintf("[y/n]"),
		),
	)

	if err != nil {
		return 0, err
	}

	if userResp := strings.ToLower(userResp); userResp == "y" || userResp == "yes" {
		agent := awsLocal.NewDefaultAgent()

		creds, err := agent.CreateIAMECRUser(region)

		if err != nil {
			color.New(color.FgRed).Printf("Automatic creation failed, manual input required. Error was: %v\n", err)
			return ecrManual(client, projectID, region)
		}

		// sleep for a few seconds to allow aws to reconfigure
		time.Sleep(3 * time.Second)

		integration, err := client.CreateAWSIntegration(
			context.Background(),
			projectID,
			&api.CreateAWSIntegrationRequest{
				AWSAccessKeyID:     creds.AWSAccessKeyID,
				AWSSecretAccessKey: creds.AWSSecretAccessKey,
				AWSRegion:          region,
			},
		)

		if err != nil {
			return 0, err
		}

		color.New(color.FgGreen).Printf("created aws integration with id %d\n", integration.ID)

		return linkRegistry(client, projectID, integration.ID)
	}

	return ecrManual(client, projectID, region)
}

func ecrManual(
	client *api.Client,
	projectID uint,
	region string,
) (uint, error) {
	// if project ID is 0, ask the user to set the project ID or create a project
	if projectID == 0 {
		return 0, fmt.Errorf("no project set, please run porter project set [id]")
	}

	// query for the access key id
	accessKeyID, err := utils.PromptPlaintext(fmt.Sprintf(`AWS Access Key ID: `))

	if err != nil {
		return 0, err
	}

	// query for the secret access key
	secretKey, err := utils.PromptPlaintext(fmt.Sprintf(`AWS Secret Access Key: `))

	if err != nil {
		return 0, err
	}

	// create the aws integration
	integration, err := client.CreateAWSIntegration(
		context.Background(),
		projectID,
		&api.CreateAWSIntegrationRequest{
			AWSAccessKeyID:     accessKeyID,
			AWSSecretAccessKey: secretKey,
			AWSRegion:          region,
		},
	)

	if err != nil {
		return 0, err
	}

	color.New(color.FgGreen).Printf("created aws integration with id %d\n", integration.ID)

	return linkRegistry(client, projectID, integration.ID)
}

func linkRegistry(client *api.Client, projectID uint, intID uint) (uint, error) {
	// create the registry
	// query for registry name
	regName, err := utils.PromptPlaintext(fmt.Sprintf(`Give this registry a name: `))

	if err != nil {
		return 0, err
	}

	reg, err := client.CreateECR(
		context.Background(),
		projectID,
		&api.CreateECRRequest{
			Name:             regName,
			AWSIntegrationID: intID,
		},
	)

	if err != nil {
		return 0, err
	}

	color.New(color.FgGreen).Printf("created registry with id %d and name %s\n", reg.ID, reg.Name)

	return reg.ID, nil
}
