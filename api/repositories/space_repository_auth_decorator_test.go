package repositories_test

import (
	"context"
	"errors"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/provider"
)

var _ = Describe("SpaceRepositoryAuthDecorator", func() {
	var (
		spaceRepo              *fake.CFSpaceRepository
		spaceRepoAuthDecorator repositories.CFSpaceRepository
		spaceRepoProvider      *provider.SpaceRepositoryProvider
		nsProvider             *fake.AuthorizedNamespacesProvider
		spaces                 []repositories.SpaceRecord
		err                    error
	)

	BeforeEach(func() {
		spaceRepo = new(fake.CFSpaceRepository)
		nsProvider = new(fake.AuthorizedNamespacesProvider)
		spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{
			{GUID: "space1"},
			{GUID: "space2"},
		}, nil)
		nsProvider.GetAuthorizedSpaceNamespacesReturns([]string{"space2"}, nil)
		spaceRepoProvider = provider.NewSpace(spaceRepo, nsProvider)
	})

	Describe("creation", func() {
		var request *http.Request

		BeforeEach(func() {
			var reqErr error
			request, reqErr = http.NewRequestWithContext(
				authorization.NewContext(context.Background(), &authorization.Info{Token: "the-token"}),
				"",
				"",
				nil,
			)
			Expect(reqErr).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			spaceRepoAuthDecorator, err = spaceRepoProvider.SpaceRepoForRequest(request)
		})

		When("there is no authorization.Info in the request context", func() {
			BeforeEach(func() {
				request = &http.Request{}
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("space repo itself", func() {
		BeforeEach(func() {
			request, setupErr := http.NewRequestWithContext(
				authorization.NewContext(context.Background(), &authorization.Info{Token: "the-token"}),
				"",
				"",
				nil,
			)
			Expect(setupErr).NotTo(HaveOccurred())
			spaceRepoAuthDecorator, setupErr = spaceRepoProvider.SpaceRepoForRequest(request)
			Expect(setupErr).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			spaces, err = spaceRepoAuthDecorator.ListSpaces(context.Background(), []string{"boo", "baz"}, []string{"foo", "bar"})
		})

		It("fetches spaces associated with the identity only", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(spaces).To(ConsistOf(repositories.SpaceRecord{GUID: "space2"}))
		})

		It("calls the space repo with the right parameters", func() {
			Expect(spaceRepo.ListSpacesCallCount()).To(Equal(1))
			_, orgIDs, names := spaceRepo.ListSpacesArgsForCall(0)
			Expect(orgIDs).To(ConsistOf("boo", "baz"))
			Expect(names).To(ConsistOf("foo", "bar"))
		})

		When("fetching spaces fails", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns(nil, errors.New("fetch-spaces-failed"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError("fetch-spaces-failed"))
			})
		})

		When("fetching authorized namespaces fails", func() {
			BeforeEach(func() {
				nsProvider.GetAuthorizedSpaceNamespacesReturns(nil, errors.New("fetch-auth-ns-failed"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError("fetch-auth-ns-failed"))
			})
		})
	})
})
