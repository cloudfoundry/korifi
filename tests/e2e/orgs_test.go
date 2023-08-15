package e2e_test

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"code.cloudfoundry.org/korifi/tests/helpers"
)

var _ = Describe("Orgs", func() {
	var resp *resty.Response

	Describe("create", func() {
		var (
			result    resource
			resultErr cfErrs
			orgName   string
		)

		BeforeEach(func() {
			orgName = generateGUID("my-org")
			result = resource{}
			resultErr = cfErrs{}
		})

		AfterEach(func() {
			deleteOrg(result.GUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(resource{Name: orgName}).
				SetError(&resultErr).
				SetResult(&result).
				Post("/v3/organizations")
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(result.Name).To(Equal(orgName))
			Expect(result.GUID).NotTo(BeEmpty())
			Expect(result.GUID).To(HavePrefix("cf-org-"))
		})
	})

	Describe("list", func() {
		var (
			org1Name, org2Name string
			org1GUID, org2GUID string
			result             resourceList[resource]
			query              map[string]string
			restyClient        *helpers.CorrelatedRestyClient
		)

		BeforeEach(func() {
			restyClient = adminClient

			var wg sync.WaitGroup
			errChan := make(chan error, 4)
			query = make(map[string]string)

			org1Name = generateGUID("org1")
			org2Name = generateGUID("org2")

			wg.Add(2)
			asyncCreateOrg(org1Name, &org1GUID, &wg, errChan)
			asyncCreateOrg(org2Name, &org2GUID, &wg, errChan)
			wg.Wait()

			var err error
			Expect(errChan).ToNot(Receive(&err), func() string { return fmt.Sprintf("unexpected error occurred while creating orgs: %v", err) })
			close(errChan)
		})

		AfterEach(func() {
			for _, id := range []string{org1GUID, org2GUID} {
				deleteOrg(id)
			}
		})

		JustBeforeEach(func() {
			var err error
			resp, err = restyClient.R().
				SetQueryParams(query).
				SetResult(&result).
				Get("/v3/organizations")
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns orgs", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{"Name": Equal(org1Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(org2Name)}),
			))
		})

		It("doesn't set an HTTP warning header for long certs", func() {
			clusterVersionMajor, clusterVersionMinor := helpers.GetClusterVersion()
			if clusterVersionMajor < 1 || (clusterVersionMajor == 1 && clusterVersionMinor < 22) {
				GinkgoWriter.Printf("Skipping certificate warning test as k8s v%d.%d doesn't support creation of short lived test client certificates\n", clusterVersionMajor, clusterVersionMinor)
				return
			}
			Expect(resp.Header().Get("X-Cf-Warnings")).To(BeEmpty())
		})

		// Note: It may seem arbitrary that we check for certificate issues on
		// the /v3/orgs endpoint. Ideally, we would do it on the /whoami endpoint
		// instead.
		// However the CLI doesn't currently check for X-Cf-Warnings headers on
		// the /whoami endpoint, so we settled on the /v3/orgs endpoint because
		// that gets called by the CLI on each login.
		When("The client has a certificate with a long expiry date", func() {
			BeforeEach(func() {
				userName := uuid.NewString()
				restyClient = makeCertClientForUserName(userName, 365*24*time.Hour)
				createOrgRole("organization_manager", userName, org2GUID)
			})

			It("returns orgs that the client has a role in and sets an HTTP warning header", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(resp).To(HaveRestyHeaderWithValue("X-Cf-Warnings", HavePrefix("Warning: The client certificate you provided for user authentication expires at")))
				Expect(resp).To(HaveRestyHeaderWithValue("X-Cf-Warnings", MatchRegexp("\\d{4}-\\d{2}-\\d{2}")))
				Expect(resp).To(HaveRestyHeaderWithValue("X-Cf-Warnings", MatchRegexp("\\d{3}h\\d{1}m\\d{1}s")))
				Expect(resp).To(HaveRestyHeaderWithValue("X-Cf-Warnings", HaveSuffix("to configure your authentication to generate short-lived credentials automatically.")))
				Expect(result.Resources).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{"Name": Equal(org2Name)}),
				))
			})
		})
	})

	Describe("delete", func() {
		var (
			orgName string
			orgGUID string
			errResp cfErrs
		)

		BeforeEach(func() {
			orgName = generateGUID("my-org")
			orgGUID = createOrg(orgName)
			errResp = cfErrs{}
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetError(&errResp).
				Delete("/v3/organizations/" + orgGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds with a job redirect", func() {
			Expect(resp).To(SatisfyAll(
				HaveRestyStatusCode(http.StatusAccepted),
				HaveRestyHeaderWithValue("Location", HaveSuffix("/v3/jobs/org.delete~"+orgGUID)),
			))

			jobURL := resp.Header().Get("Location")
			Eventually(func(g Gomega) {
				jobResp, err := adminClient.R().Get(jobURL)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
			}).Should(Succeed())

			orgResp, err := adminClient.R().Get("/v3/organizations/" + orgGUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(orgResp).To(HaveRestyStatusCode(http.StatusNotFound))
		})

		When("the org contains a space", func() {
			BeforeEach(func() {
				createSpace(generateGUID("some-space"), orgGUID)
			})

			It("can still delete the org and eventually returns a successful job redirect", func() {
				Expect(resp).To(SatisfyAll(
					HaveRestyStatusCode(http.StatusAccepted),
					HaveRestyHeaderWithValue("Location", HaveSuffix("/v3/jobs/org.delete~"+orgGUID)),
				))

				jobURL := resp.Header().Get("Location")
				Eventually(func(g Gomega) {
					jobResp, err := adminClient.R().Get(jobURL)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
				}).Should(Succeed())

				orgResp, err := adminClient.R().Get("/v3/organizations/" + orgGUID)
				Expect(err).NotTo(HaveOccurred())
				Expect(orgResp).To(HaveRestyStatusCode(http.StatusNotFound))
			})
		})
	})

	Describe("list domains", func() {
		var (
			domainName string
			orgGUID    string
			resultList resourceList[responseResource]
			errResp    cfErrs
		)

		BeforeEach(func() {
			orgGUID = createOrg(generateGUID("org"))
			domainName = helpers.GetRequiredEnvVar("APP_FQDN")
		})

		AfterEach(func() {
			deleteOrg(orgGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&resultList).
				SetError(&errResp).
				Get("/v3/organizations/" + orgGUID + "/domains")
			Expect(err).NotTo(HaveOccurred())
		})

		It("can fetch the domain", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(resultList.Resources).To(ContainElement(
				MatchFields(IgnoreExtras, Fields{"Name": Equal(domainName)}),
			))
		})
	})

	Describe("get default domain", func() {
		var (
			result  bareResource
			orgGUID string
		)

		BeforeEach(func() {
			orgGUID = createOrg(generateGUID("org"))
		})

		AfterEach(func() {
			deleteOrg(orgGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&result).
				Get("/v3/organizations/" + orgGUID + "/domains/default")
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			domainName := helpers.GetRequiredEnvVar("APP_FQDN")
			Expect(result.Name).To(Equal(domainName))
			Expect(result.GUID).NotTo(BeEmpty())
		})
	})

	Describe("get", func() {
		var result resource

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&result).
				Get("/v3/organizations/" + commonTestOrgGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the org", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(commonTestOrgGUID))
		})
	})
})
