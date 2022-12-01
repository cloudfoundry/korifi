package repositories

import (
	"context"

	"code.cloudfoundry.org/korifi/tools"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
)

type RepoMgr struct {
	Client *ecr.Client
}

func (m *RepoMgr) Create(ctx context.Context, name string) error {
	_, err := m.Client.CreateRepository(context.Background(), &ecr.CreateRepositoryInput{
		RepositoryName:     tools.PtrTo(name),
		ImageTagMutability: "IMMUTABLE",
	})

	return err
}

func (m *RepoMgr) Delete(ctx context.Context, name string) error {
	_, err := m.Client.DeleteRepository(context.Background(), &ecr.DeleteRepositoryInput{
		RepositoryName: tools.PtrTo(name),
		Force:          true,
	})

	return err
}
