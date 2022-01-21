package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/tests/e2e/helpers"

	"github.com/go-http-utils/headers"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Orgs", func() {
	Describe("creating orgs", func() {
		var (
			org              presenter.OrgResponse
			orgName          string
			orgCreateRequest helpers.APIRequest
			createOrgResp    *resty.Response
			createOrgErr     error
		)

		BeforeEach(func() {
			orgName = generateGUID("my-org")
		})

		JustBeforeEach(func() {
			createOrgResp, createOrgErr = api.NewRequest().
				SetHeader(headers.Authorization, tokenAuthHeader).
				SetBody(orgPayload(orgName)).
				Post("/v3/organizations")

			Expect(createOrgErr).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			deleteSubnamespace(rootNamespace, org.GUID)
		})

		It("creates an org", func() {
			Expect(createOrgResp).To(HaveHTTPStatus(http.StatusCreated))

			org = createOrgResp.Result().(presenter.OrgResponse)
			Expect(org.Name).To(Equal(orgName))
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKey{Name: org.GUID}, &corev1.Namespace{})
			}).Should(Succeed())
		})

		When("the org name already exists", func() {
			BeforeEach(func() {
				resp, err := api.NewRequest().
					SetHeader(headers.Authorization, tokenAuthHeader).
					SetBody(orgPayload(orgName)).
					SetResult(&org).
					Post("/v3/organizations")

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(HaveHTTPStatus(http.StatusCreated))
			})

			It("returns an unprocessable entity error", func() {
				Expect(orgCreateRequest.Status()).To(Equal(http.StatusUnprocessableEntity))

				responseMap := map[string]interface{}{}
				orgCreateRequest.DecodeResponseBody(&responseMap)
				Expect(responseMap).To(HaveKeyWithValue("errors", BeAssignableToTypeOf([]interface{}{})))
				errs := responseMap["errors"].([]interface{})
				Expect(errs[0]).To(SatisfyAll(
					HaveKeyWithValue("code", BeNumerically("==", 10008)),
					HaveKeyWithValue("detail", MatchRegexp(fmt.Sprintf(`Organization '%s' already exists.`, orgName))),
					HaveKeyWithValue("title", Equal("CF-UnprocessableEntity")),
				))
			})
		})
	})

	Describe("listing orgs", func() {
		var org1, org2, org3, org4 presenter.OrgResponse

		BeforeEach(func() {
			var wg sync.WaitGroup

			wg.Add(4)
			asyncCreateOrg(generateGUID("org1"), adminAuthHeader, &org1, &wg)
			asyncCreateOrg(generateGUID("org2"), adminAuthHeader, &org2, &wg)
			asyncCreateOrg(generateGUID("org3"), adminAuthHeader, &org3, &wg)
			asyncCreateOrg(generateGUID("org4"), adminAuthHeader, &org4, &wg)
			wg.Wait()
		})

		AfterEach(func() {
			var wg sync.WaitGroup
			wg.Add(4)
			for _, id := range []string{org1.GUID, org2.GUID, org3.GUID, org4.GUID} {
				asyncDeleteSubnamespace(rootNamespace, id, &wg)
			}
			wg.Wait()
		})

		Context("with a bearer token auth header", func() {
			BeforeEach(func() {
				createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org1.GUID, adminAuthHeader)
				createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org2.GUID, adminAuthHeader)
				createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org3.GUID, adminAuthHeader)
			})

			It("returns all 3 orgs that the service account has a role in", func() {
				orgs := map[string]interface{}{}

				resp, err := api.NewRequest().
					SetHeader(headers.Authorization, tokenAuthHeader).
					SetResult(&orgs).
					Get("/v3/organizations")

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(HaveHTTPStatus(http.StatusOK))

				Expect(orgs).To(SatisfyAll(
					HaveKeyWithValue("pagination", HaveKeyWithValue("total_results", BeNumerically(">=", 3))),
					HaveKeyWithValue("resources", ContainElements(
						HaveKeyWithValue("name", org1.Name),
						HaveKeyWithValue("name", org2.Name),
						HaveKeyWithValue("name", org3.Name),
					))))
			})

			It("does not return orgs the service account does not have a role in", func() {
				orgs := map[string]interface{}{}

				resp, err := api.NewRequest().
					SetHeader(headers.Authorization, tokenAuthHeader).
					SetResult(&orgs).
					Get("/v3/organizations")

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(HaveHTTPStatus(http.StatusOK))

				Expect(orgs).ToNot(
					HaveKeyWithValue("resources", ContainElements(
						HaveKeyWithValue("name", org4.Name),
					)))
			})

			When("org names are filtered", func() {
				It("returns orgs 1 & 3", func() {
					orgs := map[string]interface{}{}

					resp, err := api.NewRequest().
						SetHeader(headers.Authorization, tokenAuthHeader).
						SetResult(&orgs).
						SetQueryParams(map[string]string{"names": fmt.Sprintf("%s,%s", org1.Name, org3.Name)}).
						Get("/v3/organizations")

					Expect(err).NotTo(HaveOccurred())
					Expect(resp).To(HaveHTTPStatus(http.StatusOK))

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
})
