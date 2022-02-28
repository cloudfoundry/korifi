package e2e_test

import (
	"net/http"
	"time"

	certsv1 "k8s.io/api/certificates/v1"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Service Bindings", func() {
	Describe("List", Ordered, func() {
		var (
			appGUID      string
			orgGUID      string
			spaceGUID    string
			instanceGUID string
			httpResp     *resty.Response
			httpError    error
			queryString  string
			result       resourceListWithInclusion

			spaceDevUserName   string
			spaceDevSigningReq *certsv1.CertificateSigningRequest
			// spaceDevAuthHeader string
			spaceDevPEM    string
			spaceDevClient *resty.Client

			spaceMgrUserName   string
			spaceMgrSigningReq *certsv1.CertificateSigningRequest
			// spaceMgrAuthHeader string
			spaceMgrPEM    string
			spaceMgrClient *resty.Client
			testClient     *resty.Client
		)

		BeforeAll(func() {
			spaceDevUserName = generateGUID("space-dev-user")
			spaceDevSigningReq, spaceDevPEM = obtainClientCert(spaceDevUserName)
			// spaceDevAuthHeader = "ClientCert " + spaceDevPEM
			spaceDevClient = resty.New().SetBaseURL(apiServerRoot).SetAuthScheme("ClientCert").SetAuthToken(spaceDevPEM)

			spaceMgrUserName = generateGUID("space-mgr-user")
			spaceMgrSigningReq, spaceMgrPEM = obtainClientCert(spaceMgrUserName)
			// spaceMgrAuthHeader = "ClientCert " + spaceMgrPEM
			spaceMgrClient = resty.New().SetBaseURL(apiServerRoot).SetAuthScheme("ClientCert").SetAuthToken(spaceMgrPEM)

			orgGUID = createOrg(generateGUID("org"))
			time.Sleep(time.Second) // this appears to reduce flakes, but should be removed once we have better logic to determine org/space readiness
			spaceGUID = createSpace(generateGUID("space1"), orgGUID)
			time.Sleep(time.Second)
			instanceGUID = createServiceInstance(spaceGUID, generateGUID("service-instance"))
			appGUID = createApp(spaceGUID, generateGUID("app"))
			createServiceBinding(appGUID, instanceGUID)

			createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)
			createOrgRole("organization_user", rbacv1.UserKind, spaceDevUserName, orgGUID)
			createOrgRole("organization_user", rbacv1.UserKind, spaceMgrUserName, orgGUID)
			createSpaceRole("space_manager", rbacv1.UserKind, spaceMgrUserName, spaceGUID)
			createSpaceRole("space_developer", rbacv1.UserKind, spaceDevUserName, spaceGUID)
		})

		AfterAll(func() {
			deleteOrg(orgGUID)
			deleteCSR(spaceDevSigningReq)
			deleteCSR(spaceMgrSigningReq)
		})

		BeforeEach(func() {
			testClient = certClient
			queryString = ""
			result = resourceListWithInclusion{}
		})

		JustBeforeEach(func() {
			httpResp, httpError = testClient.R().SetResult(&result).Get("/v3/service_credential_bindings" + queryString)
		})

		It("Returns an empty list", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(HaveLen(0))
		})

		When("the user has space manager role", func() {
			BeforeEach(func() {
				testClient = spaceMgrClient
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(HaveLen(1))
			})
		})

		When("the user has space developer role", func() {
			BeforeEach(func() {
				testClient = spaceDevClient
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(HaveLen(1))
			})

			It("doesn't return anything in the 'included' list", func() {
				Expect(result.Included).To(BeNil())
			})

			When("the 'include=app' querystring is set", func() {
				BeforeEach(func() {
					queryString = `?include=app`
				})

				It("returns an app in the 'included' list", func() {
					Expect(httpError).NotTo(HaveOccurred())
					Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
					Expect(result.Resources).To(HaveLen(1))
					Expect(result.Included).NotTo(BeNil())
					Expect(result.Included.Apps).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(appGUID)}),
					))
				})
			})
		})
	})
})
