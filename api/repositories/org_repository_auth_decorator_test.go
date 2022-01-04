package repositories_test

import (
	"context"
	"errors"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/provider"
	providerfake "code.cloudfoundry.org/cf-k8s-controllers/api/repositories/provider/fake"
)

var _ = Describe("OrgRepositoryAuthDecorator", func() {
	var (
		orgRepo              *fake.CFOrgRepository
		orgRepoAuthDecorator repositories.CFOrgRepository
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
		orgRepo.ListOrgsReturns([]repositories.OrgRecord{
			{GUID: "org1"},
			{GUID: "org2"},
		}, nil)
		nsProvider.GetAuthorizedNamespacesReturns([]string{"org2"}, nil)
		orgRepoProvider = provider.NewOrg(orgRepo, nsProvider, identityProvider)
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
			orgRepoAuthDecorator, err = orgRepoProvider.OrgRepoForRequest(request)
		})

		It("uses the authorization.Info from the request context to get the identity", func() {
			Expect(identityProvider.GetIdentityCallCount()).To(Equal(1))
			_, actualAuthInfo := identityProvider.GetIdentityArgsForCall(0)
			Expect(actualAuthInfo.Token).To(Equal("the-token"))
		})

		When("identity provider fails", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(authorization.Identity{}, errors.New("id-provider-failure"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError("id-provider-failure"))
			})
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

	Describe("org repo itself", func() {
		BeforeEach(func() {
			request, setupErr := http.NewRequestWithContext(
				authorization.NewContext(context.Background(), &authorization.Info{Token: "the-token"}),
				"",
				"",
				nil,
			)
			Expect(setupErr).NotTo(HaveOccurred())
			orgRepoAuthDecorator, setupErr = orgRepoProvider.OrgRepoForRequest(request)
			Expect(setupErr).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			orgs, err = orgRepoAuthDecorator.ListOrgs(context.Background(), []string{"foo", "bar"})
		})

		It("fetches orgs associated with the identity only", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(orgs).To(ConsistOf(repositories.OrgRecord{GUID: "org2"}))
		})

		When("fetching orgs fails", func() {
			BeforeEach(func() {
				orgRepo.ListOrgsReturns(nil, errors.New("fetch-orgs-failed"))
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
