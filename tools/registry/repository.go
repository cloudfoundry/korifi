package registry

import (
	"context"
	"errors"
	"os"
	"strings"

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

func NewRepositoryCreator(registryType string) RepositoryCreator {
	if registryType == ECRContainerRegistryType {
		return NewECRRepositoryCreator(createECRClient())
	}

	return NoopRepositoryCreator{}
}

type ECRRepositoryCreator struct {
	ecrClient ECRClient
}

func NewECRRepositoryCreator(ecrClient ECRClient) ECRRepositoryCreator {
	return ECRRepositoryCreator{
		ecrClient: ecrClient,
	}
}

func (c ECRRepositoryCreator) CreateRepository(ctx context.Context, ref string) error {
	_, path, _ := strings.Cut(ref, "/")

	_, err := c.ecrClient.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName: tools.PtrTo(path),
	})
	if err != nil {
		var alreadyExists *types.RepositoryAlreadyExistsException
		if errors.As(err, &alreadyExists) {
			return nil
		}
	}

	return err
}

type NoopRepositoryCreator struct{}

func (c NoopRepositoryCreator) CreateRepository(_ context.Context, _ string) error {
	return nil
}
