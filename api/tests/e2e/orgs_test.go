package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Orgs", func() {
	Describe("creating orgs", func() {
		var (
			org     presenter.OrgResponse
			orgName string
		)

		BeforeEach(func() {
			orgName = generateGUID("my-org")
		})

		AfterEach(func() {
			deleteSubnamespace(rootNamespace, org.GUID)
		})

		It("creates an org", func() {
			org = createOrg(orgName, tokenAuthHeader)
			Expect(org.Name).To(Equal(orgName))
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKey{Name: org.GUID}, &corev1.Namespace{})
			}).Should(Succeed())
		})

		When("the org name already exists", func() {
			BeforeEach(func() {
				org = createOrg(orgName, tokenAuthHeader)
			})

			It("returns an unprocessable entity error", func() {
				resp := createOrgRaw(orgName, tokenAuthHeader)
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
	})

	Describe("listing orgs", func() {
		var org1, org2, org3, org4 presenter.OrgResponse

		BeforeEach(func() {
			org1 = createOrg(generateGUID("org1"), tokenAuthHeader)
			org2 = createOrg(generateGUID("org2"), tokenAuthHeader)
			org3 = createOrg(generateGUID("org3"), tokenAuthHeader)
			org4 = createOrg(generateGUID("org4"), tokenAuthHeader)
		})

		AfterEach(func() {
			deleteSubnamespace(rootNamespace, org1.GUID)
			deleteSubnamespace(rootNamespace, org2.GUID)
			deleteSubnamespace(rootNamespace, org3.GUID)
			deleteSubnamespace(rootNamespace, org4.GUID)
		})

		Context("with a bearer token auth header", func() {
			BeforeEach(func() {
				createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org1.GUID, tokenAuthHeader)
				createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org2.GUID, tokenAuthHeader)
				createOrgRole("organization_manager", rbacv1.ServiceAccountKind, serviceAccountName, org3.GUID, tokenAuthHeader)
			})

			It("returns all 3 orgs that the service account has a role in", func() {
				Eventually(getOrgsFn(tokenAuthHeader)).Should(ContainElements(
					MatchFields(IgnoreExtras, Fields{"Name": Equal(org1.Name)}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal(org2.Name)}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal(org3.Name)}),
				))
			})

			It("does not return orgs the service account does not have a role in", func() {
				Consistently(getOrgsFn(tokenAuthHeader)).ShouldNot(ContainElements(
					MatchFields(IgnoreExtras, Fields{"Name": Equal(org4.Name)}),
				))
			})

			When("org names are filtered", func() {
				It("returns orgs 1 & 3", func() {
					Eventually(getOrgsFn(tokenAuthHeader, org1.Name, org3.Name)).Should(ContainElements(
						MatchFields(IgnoreExtras, Fields{"Name": Equal(org1.Name)}),
						MatchFields(IgnoreExtras, Fields{"Name": Equal(org3.Name)}),
					))
					Consistently(getOrgsFn(tokenAuthHeader, org1.Name, org3.Name), "2s").ShouldNot(ContainElement(
						MatchFields(IgnoreExtras, Fields{"Name": Equal(org2.Name)}),
					))
				})
			})
		})

		Context("with a client certificate auth header", func() {
			BeforeEach(func() {
				createOrgRole("organization_manager", rbacv1.UserKind, certUserName, org1.GUID, certAuthHeader)
				createOrgRole("organization_manager", rbacv1.UserKind, certUserName, org2.GUID, certAuthHeader)
				createOrgRole("organization_manager", rbacv1.UserKind, certUserName, org3.GUID, certAuthHeader)
			})

			It("returns all 3 orgs that the service account has a role in", func() {
				Eventually(getOrgsFn(certAuthHeader)).Should(ContainElements(
					MatchFields(IgnoreExtras, Fields{"Name": Equal(org1.Name)}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal(org2.Name)}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal(org3.Name)}),
				))
			})

			It("does not return orgs the service account does not have a role in", func() {
				Consistently(getOrgsFn(certAuthHeader)).ShouldNot(ContainElements(
					MatchFields(IgnoreExtras, Fields{"Name": Equal(org4.Name)}),
				))
			})

			When("org names are filtered", func() {
				It("returns orgs 1 & 3", func() {
					Eventually(getOrgsFn(certAuthHeader, org1.Name, org3.Name)).Should(ContainElements(
						MatchFields(IgnoreExtras, Fields{"Name": Equal(org1.Name)}),
						MatchFields(IgnoreExtras, Fields{"Name": Equal(org3.Name)}),
					))
					Consistently(getOrgsFn(certAuthHeader, org1.Name, org3.Name), "2s").ShouldNot(ContainElement(
						MatchFields(IgnoreExtras, Fields{"Name": Equal(org2.Name)}),
					))
				})
			})
		})

		When("no Authorization header is available in the request", func() {
			It("returns unauthorized error", func() {
				orgsUrl := apiServerRoot + apis.OrgsEndpoint
				req, err := http.NewRequest(http.MethodGet, orgsUrl, nil)
				Expect(err).NotTo(HaveOccurred())
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			})
		})
	})
})

func getOrgsFn(authHeaderValue string, names ...string) func() ([]presenter.OrgResponse, error) {
	return func() ([]presenter.OrgResponse, error) {
		orgsUrl := apiServerRoot + apis.OrgsEndpoint

		if len(names) > 0 {
			orgsUrl += "?names=" + strings.Join(names, ",")
		}

		resp, err := httpReq(http.MethodGet, orgsUrl, authHeaderValue, nil)
		if err != nil {
			return nil, err
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
		}

		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		orgList := presenter.OrgListResponse{}
		err = json.Unmarshal(bodyBytes, &orgList)
		if err != nil {
			return nil, err
		}

		return orgList.Resources, nil
	}
}
