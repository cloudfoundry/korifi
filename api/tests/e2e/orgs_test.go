package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Orgs", func() {
	Describe("creating orgs", func() {
		var (
			org     presenter.OrgResponse
			orgName string
		)

		BeforeEach(func() {
			org = presenter.OrgResponse{}
			orgName = generateGUID("my-org")
		})

		AfterEach(func() {
			if org.GUID != "" {
				deleteSubnamespace(rootNamespace, org.GUID)
			}
		})

		It("creates an org", func() {
			org = createOrg(orgName, adminAuthHeader)
			Expect(org.Name).To(Equal(orgName))
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKey{Name: org.GUID}, &corev1.Namespace{})
			}).Should(Succeed())
		})

		When("the org name already exists", func() {
			BeforeEach(func() {
				org = createOrg(orgName, adminAuthHeader)
			})

			It("returns an unprocessable entity error", func() {
				resp, err := createOrgRaw(orgName, adminAuthHeader)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
				responseMap := map[string]interface{}{}
				Expect(json.NewDecoder(resp.Body).Decode(&responseMap)).To(Succeed())
				Expect(responseMap).To(HaveKeyWithValue("errors", BeAssignableToTypeOf([]interface{}{})))
				errs := responseMap["errors"].([]interface{})
				Expect(errs[0]).To(SatisfyAll(
					HaveKeyWithValue("code", BeNumerically("==", 10008)),
					HaveKeyWithValue("detail", MatchRegexp(fmt.Sprintf(`Organization '%s' already exists.`, orgName))),
					HaveKeyWithValue("title", Equal("CF-UnprocessableEntity")),
				))
			})
		})

		When("not admin", func() {
			It("returns a forbidden error", func() {
				resp, err := createOrgRaw(orgName, tokenAuthHeader)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp).To(HaveHTTPStatus(http.StatusForbidden))
			})
		})
	})

	Describe("listing orgs", func() {
		var org1, org2, org3, org4 presenter.OrgResponse

		BeforeEach(func() {
			var wg sync.WaitGroup
			errChan := make(chan error, 4)

			wg.Add(4)
			asyncCreateOrg(generateGUID("org1"), adminAuthHeader, &org1, &wg, errChan)
			asyncCreateOrg(generateGUID("org2"), adminAuthHeader, &org2, &wg, errChan)
			asyncCreateOrg(generateGUID("org3"), adminAuthHeader, &org3, &wg, errChan)
			asyncCreateOrg(generateGUID("org4"), adminAuthHeader, &org4, &wg, errChan)
			wg.Wait()

			Expect(errChan).ToNot(Receive())
			close(errChan)
		})

		AfterEach(func() {
			var wg sync.WaitGroup
			wg.Add(4)
			for _, id := range []string{org1.GUID, org2.GUID, org3.GUID, org4.GUID} {
				asyncDeleteSubnamespace(rootNamespace, id, &wg)
			}
			wg.Wait()
		})

		BeforeEach(func() {
			createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org1.GUID, adminAuthHeader)
			createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org2.GUID, adminAuthHeader)
			createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org3.GUID, adminAuthHeader)
		})

		It("returns all 3 orgs that the service account has a role in", func() {
			orgs, err := get(apis.OrgsEndpoint, tokenAuthHeader)
			Expect(err).NotTo(HaveOccurred())
			Expect(orgs).To(SatisfyAll(
				HaveKeyWithValue("pagination", HaveKeyWithValue("total_results", BeNumerically(">=", 3))),
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", org1.Name),
					HaveKeyWithValue("name", org2.Name),
					HaveKeyWithValue("name", org3.Name),
				))))
		})

		It("does not return orgs the service account does not have a role in", func() {
			orgs, err := get(apis.OrgsEndpoint, tokenAuthHeader)
			Expect(err).NotTo(HaveOccurred())
			Expect(orgs).ToNot(
				HaveKeyWithValue("resources", ContainElements(
					HaveKeyWithValue("name", org4.Name),
				)))
		})

		When("org names are filtered", func() {
			It("returns orgs 1 & 3", func() {
				orgs, err := getWithQuery(
					apis.OrgsEndpoint,
					tokenAuthHeader,
					map[string]string{
						"names": fmt.Sprintf("%s,%s", org1.Name, org3.Name),
					},
				)
				Expect(err).NotTo(HaveOccurred())
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
