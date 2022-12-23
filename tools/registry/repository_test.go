package registry_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/registry"
	"code.cloudfoundry.org/korifi/tools/registry/fake"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("ECR Repository Creator", func() {
	var (
		ecrClient *fake.ECRClient
		creator   registry.RepositoryCreator
		createErr error
	)

	BeforeEach(func() {
		ecrClient = new(fake.ECRClient)
		ecrClient.CreateRepositoryReturns(&ecr.CreateRepositoryOutput{
			Repository: &types.Repository{
				RepositoryUri: tools.PtrTo("repo-uri"),
			},
		}, nil)
		creator = registry.NewECRRepositoryCreator(ecrClient)
	})

	JustBeforeEach(func() {
		createErr = creator.CreateRepository(context.Background(), "my.registry/my-repo")
	})

	It("succeeds", func() {
		Expect(createErr).NotTo(HaveOccurred())
	})

	It("creates the repo", func() {
		Expect(ecrClient.CreateRepositoryCallCount()).To(Equal(1))
		_, actualCreateInput, _ := ecrClient.CreateRepositoryArgsForCall(0)
		Expect(actualCreateInput).To(gstruct.PointTo(Equal(ecr.CreateRepositoryInput{
			RepositoryName: tools.PtrTo("my-repo"),
		})))
	})

	When("registry creation fails", func() {
		BeforeEach(func() {
			ecrClient.CreateRepositoryReturns(nil, errors.New("registry create err"))
		})

		It("returns an error", func() {
			Expect(createErr).To(MatchError("registry create err"))
		})
	})
})
