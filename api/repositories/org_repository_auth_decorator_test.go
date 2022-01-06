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

var _ = Describe("OrgRepositoryAuthDecorator", func() {
	var (
		orgRepo              *fake.CFOrgRepository
		orgRepoAuthDecorator repositories.CFOrgRepository
		orgRepoProvider      *provider.OrgRepositoryProvider
		nsProvider           *fake.AuthorizedNamespacesProvider
		orgs                 []repositories.OrgRecord
		err                  error
	)

	BeforeEach(func() {
		orgRepo = new(fake.CFOrgRepository)
		nsProvider = new(fake.AuthorizedNamespacesProvider)
		orgRepo.ListOrgsReturns([]repositories.OrgRecord{
			{GUID: "org1"},
			{GUID: "org2"},
		}, nil)
		nsProvider.GetAuthorizedOrgNamespacesReturns([]string{"org2"}, nil)
		orgRepoProvider = provider.NewOrg(orgRepo, nsProvider)
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
				nsProvider.GetAuthorizedOrgNamespacesReturns(nil, errors.New("fetch-auth-ns-failed"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError("fetch-auth-ns-failed"))
			})
		})
	})
})
