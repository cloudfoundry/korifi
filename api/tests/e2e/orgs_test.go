package e2e_test

import (
	"fmt"
	"net/http"
	"sync"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Orgs", func() {
	Describe("creating orgs", func() {
		var (
			client    *resty.Client
			result    presenter.OrgResponse
			resultErr map[string]interface{}
			orgName   string
			httpResp  *resty.Response
			httpErr   error
		)

		BeforeEach(func() {
			client = adminClient
			result = presenter.OrgResponse{}
			orgName = generateGUID("my-org")
		})

		AfterEach(func() {
			if result.GUID != "" {
				deleteSubnamespace(rootNamespace, result.GUID)
			}
		})

		JustBeforeEach(func() {
			httpResp, httpErr = client.R().
				SetBody(fmt.Sprintf(`{"name": "%s"}`, orgName)).
				SetError(&resultErr).
				SetResult(&result).
				Post("/v3/organizations")
		})

		It("succeeds", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusCreated))
			Expect(result.Name).To(Equal(orgName))
			Expect(result.GUID).NotTo(BeEmpty())
		})

		When("the org name already exists", func() {
			BeforeEach(func() {
				createOrg(orgName)
			})

			It("returns an unprocessable entity error", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusUnprocessableEntity))
				Expect(resultErr).To(HaveKeyWithValue("errors", ConsistOf(
					SatisfyAll(
						HaveKeyWithValue("code", BeNumerically("==", 10008)),
						HaveKeyWithValue("detail", MatchRegexp(fmt.Sprintf(`Organization '%s' already exists.`, orgName))),
						HaveKeyWithValue("title", Equal("CF-UnprocessableEntity")),
					),
				)))
			})
		})

		When("not admin", func() {
			BeforeEach(func() {
				client = tokenClient
			})

			It("returns a forbidden error", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusForbidden))
			})
		})
	})

	Describe("listing orgs", func() {
		var (
			org1, org2, org3, org4 presenter.OrgResponse
			orgs                   map[string]interface{}
			httpResp               *resty.Response
			httpErr                error
			query                  map[string]string
		)

		BeforeEach(func() {
			var wg sync.WaitGroup
			errChan := make(chan error, 4)
			query = make(map[string]string)

			wg.Add(4)
			asyncCreateOrg(generateGUID("org1"), adminAuthHeader, &org1, &wg, errChan)
			asyncCreateOrg(generateGUID("org2"), adminAuthHeader, &org2, &wg, errChan)
			asyncCreateOrg(generateGUID("org3"), adminAuthHeader, &org3, &wg, errChan)
			asyncCreateOrg(generateGUID("org4"), adminAuthHeader, &org4, &wg, errChan)
			wg.Wait()

			var err error
			Expect(errChan).ToNot(Receive(&err), func() string { return fmt.Sprintf("unexpected error occurred while creating orgs: %v", err) })
			close(errChan)

			createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org1.GUID, adminAuthHeader)
			createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org2.GUID, adminAuthHeader)
			createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org3.GUID, adminAuthHeader)
		})

		AfterEach(func() {
			var wg sync.WaitGroup
			wg.Add(4)
			for _, id := range []string{org1.GUID, org2.GUID, org3.GUID, org4.GUID} {
				asyncDeleteSubnamespace(rootNamespace, id, &wg)
			}
			wg.Wait()
		})

		JustBeforeEach(func() {
			httpResp, httpErr = tokenClient.R().
				SetQueryParams(query).
				SetResult(&orgs).
				Get("/v3/organizations")
		})

		It("returns orgs that the service account has a role in", func() {
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusOK))
			Expect(orgs).To(SatisfyAll(
				HaveKeyWithValue("pagination", HaveKeyWithValue("total_results", BeNumerically(">=", 3))),
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", org1.Name),
					HaveKeyWithValue("name", org2.Name),
					HaveKeyWithValue("name", org3.Name),
				))))

			Expect(orgs).ToNot(
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", org4.Name),
				)))
		})

		When("org names are filtered", func() {
			BeforeEach(func() {
				query = map[string]string{
					"names": org1.Name + "," + org3.Name,
				}
			})

			It("returns orgs 1 & 3", func() {
				Expect(httpErr).NotTo(HaveOccurred())
				Expect(orgs).To(SatisfyAll(
					HaveKeyWithValue("pagination", HaveKeyWithValue("total_results", BeNumerically(">=", 2))),
					HaveKeyWithValue("resources", ContainElements(
						HaveKeyWithValue("name", org1.Name),
						HaveKeyWithValue("name", org3.Name),
					))))
				Expect(orgs).ToNot(
					HaveKeyWithValue("resources", ContainElements(
						HaveKeyWithValue("name", org2.Name),
					)))
			})
		})
	})
})
