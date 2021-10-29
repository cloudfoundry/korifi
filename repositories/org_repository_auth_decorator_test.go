package repositories_test

import (
	"context"
	"errors"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-api/apis"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"code.cloudfoundry.org/cf-k8s-api/repositories/authorization"
	"code.cloudfoundry.org/cf-k8s-api/repositories/fake"
	"code.cloudfoundry.org/cf-k8s-api/repositories/provider"
	providerfake "code.cloudfoundry.org/cf-k8s-api/repositories/provider/fake"
	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("OrgRepositoryAuthDecorator", func() {
	var (
		orgRepo              *fake.CFOrgRepository
		orgRepoAuthDecorator apis.CFOrgRepository
		orgRepoProvider      *provider.OrgRepositoryProvider
		nsProvider           *fake.AuthorizedNamespacesProvider
		identity             authorization.Identity
		identityProvider     *providerfake.IdentityProvider
		orgs                 []repositories.OrgRecord
		err                  error
	)

	BeforeEach(func() {
		identity = authorization.Identity{Kind: rbacv1.UserKind, Name: "alice"}
		identityProvider = new(providerfake.IdentityProvider)
		identityProvider.GetIdentityReturns(identity, nil)
		orgRepo = new(fake.CFOrgRepository)
		nsProvider = new(fake.AuthorizedNamespacesProvider)
		orgRepo.FetchOrgsReturns([]repositories.OrgRecord{
			{GUID: "org1"},
			{GUID: "org2"},
		}, nil)
		nsProvider.GetAuthorizedNamespacesReturns([]string{"org2"}, nil)
		orgRepoProvider = provider.NewOrg(orgRepo, nsProvider, identityProvider)
	})

	Describe("creation", func() {
		JustBeforeEach(func() {
			orgRepoAuthDecorator, err = orgRepoProvider.OrgRepoForRequest(&http.Request{
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

	Describe("org repo itself", func() {
		BeforeEach(func() {
			orgRepoAuthDecorator, err = orgRepoProvider.OrgRepoForRequest(&http.Request{
				Header: http.Header{
					headers.Authorization: []string{"bearer the-token"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			orgs, err = orgRepoAuthDecorator.FetchOrgs(context.Background(), []string{"foo", "bar"})
		})

		It("fetches orgs associated with the identity only", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(orgs).To(ConsistOf(repositories.OrgRecord{GUID: "org2"}))
		})

		When("fetching orgs fails", func() {
			BeforeEach(func() {
				orgRepo.FetchOrgsReturns(nil, errors.New("fetch-orgs-failed"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError("fetch-orgs-failed"))
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
