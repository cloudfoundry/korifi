package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"gopkg.in/yaml.v3"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Spaces", func() {
	var resp *resty.Response

	Describe("create", func() {
		var (
			result     resource
			orgGUID    string
			parentGUID string
			spaceName  string
			resultErr  cfErrs
		)

		BeforeEach(func() {
			spaceName = generateGUID("space")
			orgGUID = createOrg(generateGUID("org"))
			parentGUID = orgGUID
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, orgGUID)
			resultErr = cfErrs{}
		})

		AfterEach(func() {
			deleteOrg(orgGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(resource{
					Name: spaceName,
					Relationships: relationships{
						"organization": {Data: resource{GUID: parentGUID}},
					},
				}).
				SetError(&resultErr).
				SetResult(&result).
				Post("/v3/spaces")
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a space", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(result.Name).To(Equal(spaceName))
			Expect(result.GUID).NotTo(BeEmpty())
		})

		When("the space name already exists", func() {
			BeforeEach(func() {
				createSpace(spaceName, orgGUID)
			})

			It("returns an unprocessable entity error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(resultErr.Errors).To(ConsistOf(cfErr{
					Detail: fmt.Sprintf(`Space '%s' already exists.`, spaceName),
					Title:  "CF-UnprocessableEntity",
					Code:   10008,
				}))
			})
		})

		When("the organization relationship references a space guid", func() {
			BeforeEach(func() {
				otherSpaceGUID := createSpace(generateGUID("some-other-space"), orgGUID)
				parentGUID = otherSpaceGUID
			})

			It("denies the request", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(resultErr.Errors).To(ConsistOf(cfErr{
					Detail: "Invalid organization. Ensure the organization exists and you have access to it.",
					Title:  "CF-UnprocessableEntity",
					Code:   10008,
				}))
			})
		})
	})

	Describe("list", func() {
		var (
			org1GUID, org2GUID, org3GUID          string
			space11GUID, space12GUID, space13GUID string
			space21GUID, space22GUID, space23GUID string
			space31GUID, space32GUID, space33GUID string
			space11Name, space12Name, space13Name string
			space21Name, space22Name, space23Name string
			space31Name, space32Name, space33Name string
			result                                resourceList
			query                                 map[string]string
		)

		BeforeEach(func() {
			var orgWG sync.WaitGroup
			orgErrChan := make(chan error, 3)
			query = make(map[string]string)

			orgWG.Add(3)
			asyncCreateOrg(generateGUID("org1"), &org1GUID, &orgWG, orgErrChan)
			asyncCreateOrg(generateGUID("org2"), &org2GUID, &orgWG, orgErrChan)
			asyncCreateOrg(generateGUID("org3"), &org3GUID, &orgWG, orgErrChan)
			orgWG.Wait()

			var err error
			Expect(orgErrChan).ToNot(Receive(&err), func() string { return fmt.Sprintf("unexpected error occurred while creating orgs: %v", err) })
			close(orgErrChan)

			var spaceWG sync.WaitGroup

			space11Name, space12Name, space13Name = generateGUID("space11"), generateGUID("space12"), generateGUID("space13")
			space21Name, space22Name, space23Name = generateGUID("space21"), generateGUID("space22"), generateGUID("space23")
			space31Name, space32Name, space33Name = generateGUID("space31"), generateGUID("space32"), generateGUID("space33")

			spaceErrChan := make(chan error, 9)
			spaceWG.Add(9)
			asyncCreateSpace(space11Name, org1GUID, &space11GUID, &spaceWG, spaceErrChan)
			asyncCreateSpace(space12Name, org1GUID, &space12GUID, &spaceWG, spaceErrChan)
			asyncCreateSpace(space13Name, org1GUID, &space13GUID, &spaceWG, spaceErrChan)

			asyncCreateSpace(space21Name, org2GUID, &space21GUID, &spaceWG, spaceErrChan)
			asyncCreateSpace(space22Name, org2GUID, &space22GUID, &spaceWG, spaceErrChan)
			asyncCreateSpace(space23Name, org2GUID, &space23GUID, &spaceWG, spaceErrChan)

			asyncCreateSpace(space31Name, org3GUID, &space31GUID, &spaceWG, spaceErrChan)
			asyncCreateSpace(space32Name, org3GUID, &space32GUID, &spaceWG, spaceErrChan)
			asyncCreateSpace(space33Name, org3GUID, &space33GUID, &spaceWG, spaceErrChan)
			spaceWG.Wait()

			Expect(spaceErrChan).ToNot(Receive(&err), func() string { return fmt.Sprintf("unexpected error occurred while creating spaces: %v", err) })
			close(spaceErrChan)

			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org1GUID)
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org2GUID)
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, org3GUID)

			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space12GUID)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space11GUID)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space21GUID)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space22GUID)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space31GUID)
			createSpaceRole("space_developer", rbacv1.ServiceAccountKind, serviceAccountName, space32GUID)
		})

		AfterEach(func() {
			orgIDs := []string{org1GUID, org2GUID, org3GUID}
			var wg sync.WaitGroup
			wg.Add(len(orgIDs))
			for _, id := range orgIDs {
				asyncDeleteOrg(id, &wg)
			}
			wg.Wait()
		})

		JustBeforeEach(func() {
			var err error
			resp, err = tokenClient.R().
				SetQueryParams(query).
				SetResult(&result).
				Get("/v3/spaces")
			Expect(err).NotTo(HaveOccurred())
		})

		It("lists the spaces the user has role in", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space11Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space12Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space21Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space22Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space31Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space32Name)}),
			))
			Expect(result.Resources).ToNot(ContainElements(
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space13Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space23Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space33Name)}),
			))
		})

		When("filtering by organization GUIDs", func() {
			BeforeEach(func() {
				query = map[string]string{
					"organization_guids": fmt.Sprintf("%s,%s", org1GUID, org3GUID),
				}
			})

			It("only lists spaces beloging to the orgs", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"Name": Equal(space11Name)}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal(space12Name)}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal(space31Name)}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal(space32Name)}),
				))
			})
		})

		When("filtering by name", func() {
			BeforeEach(func() {
				query = map[string]string{
					"names": strings.Join([]string{space12Name, space13Name, space32Name}, ","),
				}
			})

			It("only lists those matching and available", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"Name": Equal(space12Name)}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal(space32Name)}),
				))
			})
		})
	})

	Describe("delete", func() {
		var (
			orgGUID   string
			spaceGUID string
			resultErr cfErrs
		)

		BeforeEach(func() {
			orgGUID = createOrg(generateGUID("org"))
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, orgGUID)
			spaceGUID = createSpace(generateGUID("space"), orgGUID)
			resultErr = cfErrs{}
		})

		AfterEach(func() {
			deleteOrg(orgGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetError(&resultErr).
				Delete("/v3/spaces/" + spaceGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds with a job redirect", func() {
			Expect(resp).To(SatisfyAll(
				HaveRestyStatusCode(http.StatusAccepted),
				HaveRestyHeaderWithValue("Location", HaveSuffix("/v3/jobs/space.delete-"+spaceGUID)),
			))

			jobURL := resp.Header().Get("Location")
			Eventually(func(g Gomega) {
				jobResp, err := adminClient.R().Get(jobURL)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
			}).Should(Succeed())
		})

		When("the space does not exist", func() {
			BeforeEach(func() {
				spaceGUID = "nope"
			})

			It("returns a not found error", func() {
				expectNotFoundError(resp, resultErr, "Space")
			})
		})
	})

	Describe("manifests", func() {
		var (
			orgGUID       string
			spaceGUID     string
			resultErr     cfErrs
			manifestBytes []byte
		)

		BeforeEach(func() {
			orgGUID = createOrg(generateGUID("org"))
			createOrgRole("organization_user", rbacv1.ServiceAccountKind, serviceAccountName, orgGUID)
			spaceGUID = createSpace(generateGUID("space"), orgGUID)
			resultErr = cfErrs{}

			route := "manifested-app.vcap.me"
			command := "whatever"
			var err error
			manifestBytes, err = yaml.Marshal(manifestResource{
				Version: 1,
				Applications: []applicationResource{{
					Name: "manifested-app",
					Processes: []manifestApplicationProcessResource{{
						Type:    "web",
						Command: &command,
					}},
					Routes: []manifestRouteResource{{
						Route: &route,
					}},
				}},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			deleteOrg(orgGUID)
		})

		Describe("apply manifest", func() {
			JustBeforeEach(func() {
				var err error
				resp, err = adminClient.R().
					SetHeader("Content-type", "application/x-yaml").
					SetBody(manifestBytes).
					SetError(&resultErr).
					Post("/v3/spaces/" + spaceGUID + "/actions/apply_manifest")
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds with a job redirect", func() {
				Expect(resp).To(SatisfyAll(
					HaveRestyStatusCode(http.StatusAccepted),
					HaveRestyHeaderWithValue("Location", HaveSuffix("/v3/jobs/space.apply_manifest-"+spaceGUID)),
				))

				jobURL := resp.Header().Get("Location")
				Eventually(func(g Gomega) {
					jobResp, err := adminClient.R().Get(jobURL)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
				}).Should(Succeed())
			})
		})

		Describe("manifest diff", func() {
			JustBeforeEach(func() {
				var err error
				resp, err = adminClient.R().
					SetHeader("Content-type", "application/x-yaml").
					SetBody(manifestBytes).
					SetError(&resultErr).
					Post("/v3/spaces/" + spaceGUID + "/manifest_diff")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns JSON diff", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusAccepted))

				diff := map[string]interface{}{}
				Expect(json.Unmarshal(resp.Body(), &diff)).To(Succeed())
				// The `manifest_diff` endpoint is currently a stub to return
				// an empty diff
				Expect(diff).To(HaveKeyWithValue("diff", BeEmpty()))
			})
		})
	})
})
