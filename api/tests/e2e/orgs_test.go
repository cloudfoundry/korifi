package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"

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
			org1 = createOrg(generateGUID("org1"), adminAuthHeader)
			org2 = createOrg(generateGUID("org2"), adminAuthHeader)
			org3 = createOrg(generateGUID("org3"), adminAuthHeader)
			org4 = createOrg(generateGUID("org4"), adminAuthHeader)
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
				orgs, err := getOrgs(tokenAuthHeader, nil)
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
				orgs, err := getOrgs(tokenAuthHeader, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(orgs).ToNot(
					HaveKeyWithValue("resources", ContainElements(
						HaveKeyWithValue("name", org4.Name),
					)))
			})

			When("org names are filtered", func() {
				It("returns orgs 1 & 3", func() {
					orgs, err := getOrgs(tokenAuthHeader, map[string]string{"names": fmt.Sprintf("%s,%s", org1.Name, org3.Name)})
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

		Context("with a client certificate auth header", func() {
			BeforeEach(func() {
				createOrgRole("organization_manager", rbacv1.UserKind, certUserName, org1.GUID, adminAuthHeader)
				createOrgRole("organization_manager", rbacv1.UserKind, certUserName, org2.GUID, adminAuthHeader)
				createOrgRole("organization_manager", rbacv1.UserKind, certUserName, org3.GUID, adminAuthHeader)
			})

			It("returns all 3 orgs that the service account has a role in", func() {
				orgs, err := getOrgs(certAuthHeader, nil)
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
				orgs, err := getOrgs(certAuthHeader, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(orgs).ToNot(
					HaveKeyWithValue("resources", ContainElements(
						HaveKeyWithValue("name", org4.Name),
					)))
			})

			When("org names are filtered", func() {
				It("returns orgs 1 & 3", func() {
					orgs, err := getOrgs(certAuthHeader, map[string]string{"names": fmt.Sprintf("%s,%s", org1.Name, org3.Name)})
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

		When("no Authorization header is available in the request", func() {
			It("returns unauthorized error", func() {
				_, err := getOrgs("", nil)
				Expect(err).To(MatchError(ContainSubstring(strconv.Itoa(http.StatusUnauthorized))))
			})
		})
	})
})

func getOrgs(authHeaderValue string, query map[string]string) (map[string]interface{}, error) {
	orgsUrl, err := url.Parse(apiServerRoot)
	if err != nil {
		return nil, err
	}

	orgsUrl.Path = apis.OrgsEndpoint
	values := url.Values{}
	for key, val := range query {
		values.Set(key, val)
	}
	orgsUrl.RawQuery = values.Encode()

	resp, err := httpReq(http.MethodGet, orgsUrl.String(), authHeaderValue, nil)
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

	response := map[string]interface{}{}
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, err
	}

	return response, nil
}
