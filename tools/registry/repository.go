package registry

import (
	"context"
	"errors"
	"os"

	"code.cloudfoundry.org/korifi/tools"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const ECRContainerRegistryType = "ECR"

//counterfeiter:generate -o fake -fake-name ECRClient . ECRClient

type ECRClient interface {
	CreateRepository(ctx context.Context, params *ecr.CreateRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error)
}

type RepositoryCreator interface {
	CreateRepository(ctx context.Context, name string) error
}

func createECRClient() *ecr.Client {
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		ctrl.Log.Error(err, "error creating the ECR client")
		os.Exit(1)
	}

	return ecr.NewFromConfig(awsConfig)
}

func NewRegistryCreator(registryType string) RepositoryCreator {
	if registryType == ECRContainerRegistryType {
		return NewECRRegistryCreator(createECRClient())
	}

	return NoopRegistryCreator{}
}

type ECRRegistryCreator struct {
	ecrClient ECRClient
}

func NewECRRegistryCreator(ecrClient ECRClient) ECRRegistryCreator {
	return ECRRegistryCreator{
		ecrClient: ecrClient,
	}
}

func (c ECRRegistryCreator) CreateRepository(ctx context.Context, name string) error {
	_, err := c.ecrClient.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName: tools.PtrTo(name),
	})
	if err != nil {
		var alreadyExists *types.RepositoryAlreadyExistsException
		if errors.As(err, &alreadyExists) {
			return nil
		}
	}

	return err
}

type NoopRegistryCreator struct{}

func (c NoopRegistryCreator) CreateRepository(_ context.Context, _ string) error {
	return nil
}
