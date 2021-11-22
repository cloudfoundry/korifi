package repositories_test

import (
	"context"
	"errors"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/provider"
	providerfake "code.cloudfoundry.org/cf-k8s-controllers/api/repositories/provider/fake"
	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("SpaceRepositoryAuthDecorator", func() {
	var (
		spaceRepo              *fake.CFSpaceRepository
		spaceRepoAuthDecorator repositories.CFSpaceRepository
		spaceRepoProvider      *provider.SpaceRepositoryProvider
		nsProvider             *fake.AuthorizedNamespacesProvider
		identity               authorization.Identity
		identityProvider       *providerfake.IdentityProvider
		spaces                 []repositories.SpaceRecord
		err                    error
	)

	BeforeEach(func() {
		identity = authorization.Identity{Kind: rbacv1.UserKind, Name: "alice"}
		identityProvider = new(providerfake.IdentityProvider)
		identityProvider.GetIdentityReturns(identity, nil)
		spaceRepo = new(fake.CFSpaceRepository)
		nsProvider = new(fake.AuthorizedNamespacesProvider)
		spaceRepo.FetchSpacesReturns([]repositories.SpaceRecord{
			{GUID: "space1"},
			{GUID: "space2"},
		}, nil)
		nsProvider.GetAuthorizedNamespacesReturns([]string{"space2"}, nil)
		spaceRepoProvider = provider.NewSpace(spaceRepo, nsProvider, identityProvider)
	})

	Describe("creation", func() {
		JustBeforeEach(func() {
			spaceRepoAuthDecorator, err = spaceRepoProvider.SpaceRepoForRequest(&http.Request{
				Header: http.Header{
					headers.Authorization: []string{"bearer the-token"},
				},
			})
		})

		It("gets built from the Authorization header", func() {
			Expect(identityProvider.GetIdentityCallCount()).To(Equal(1))
			_, bearerToken := identityProvider.GetIdentityArgsForCall(0)
			Expect(bearerToken).To(Equal("bearer the-token"))
		})

		When("identity provider fails", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(authorization.Identity{}, errors.New("id-provider-failure"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError("id-provider-failure"))
			})
		})
	})

	Describe("space repo itself", func() {
		BeforeEach(func() {
			spaceRepoAuthDecorator, err = spaceRepoProvider.SpaceRepoForRequest(&http.Request{
				Header: http.Header{
					headers.Authorization: []string{"bearer the-token"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			spaces, err = spaceRepoAuthDecorator.FetchSpaces(context.Background(), []string{"boo", "baz"}, []string{"foo", "bar"})
		})

		It("fetches spaces associated with the identity only", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(spaces).To(ConsistOf(repositories.SpaceRecord{GUID: "space2"}))
		})

		It("calls the space repo with the right parameters", func() {
			Expect(spaceRepo.FetchSpacesCallCount()).To(Equal(1))
			_, orgIDs, names := spaceRepo.FetchSpacesArgsForCall(0)
			Expect(orgIDs).To(ConsistOf("boo", "baz"))
			Expect(names).To(ConsistOf("foo", "bar"))
		})

		When("fetching spaces fails", func() {
			BeforeEach(func() {
				spaceRepo.FetchSpacesReturns(nil, errors.New("fetch-spaces-failed"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError("fetch-spaces-failed"))
			})
		})

		When("fetching authorized namespaces fails", func() {
			BeforeEach(func() {
				nsProvider.GetAuthorizedNamespacesReturns(nil, errors.New("fetch-auth-ns-failed"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError("fetch-auth-ns-failed"))
			})
		})
	})
})
