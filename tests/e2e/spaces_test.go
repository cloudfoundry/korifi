package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"code.cloudfoundry.org/korifi/tests/e2e/helpers"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"gopkg.in/yaml.v3"
)

var _ = Describe("Spaces", func() {
	var (
		resp        *resty.Response
		restyClient *helpers.CorrelatedRestyClient
	)

	BeforeEach(func() {
		restyClient = certClient
	})

	Describe("create", func() {
		var (
			result     resource
			parentGUID string
			spaceName  string
			createErr  cfErrs
		)

		BeforeEach(func() {
			spaceName = generateGUID("space")
			parentGUID = commonTestOrgGUID
			createErr = cfErrs{}

			restyClient = adminClient
		})

		JustBeforeEach(func() {
			var err error
			resp, err = restyClient.R().
				SetBody(resource{
					Name: spaceName,
					Relationships: relationships{
						"organization": {Data: resource{GUID: parentGUID}},
					},
				}).
				SetError(&createErr).
				SetResult(&result).
				Post("/v3/spaces")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if len(createErr.Errors) == 0 {
				deleteSpace(result.GUID)
			}
		})

		It("creates a space", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(result.Name).To(Equal(spaceName))
			Expect(result.GUID).To(HavePrefix("cf-space-"))
			Expect(result.GUID).NotTo(BeEmpty())
		})

		When("the space name already exists", func() {
			BeforeEach(func() {
				createSpace(spaceName, commonTestOrgGUID)
			})

			It("returns an unprocessable entity error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(createErr.Errors).To(ConsistOf(cfErr{
					Detail: fmt.Sprintf(`Space '%s' already exists. Name must be unique per organization.`, spaceName),
					Title:  "CF-UnprocessableEntity",
					Code:   10008,
				}))
			})
		})

		When("the organization relationship references a space guid", func() {
			var otherSpaceGUID string

			BeforeEach(func() {
				otherSpaceGUID = createSpace(generateGUID("some-other-space"), commonTestOrgGUID)
				parentGUID = otherSpaceGUID
			})

			AfterEach(func() {
				deleteSpace(otherSpaceGUID)
			})

			It("denies the request", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(createErr.Errors).To(ConsistOf(cfErr{
					Detail: "Invalid organization. Ensure the organization exists and you have access to it.",
					Title:  "CF-UnprocessableEntity",
					Code:   10008,
				}))
			})
		})
	})

	Describe("create as a ServiceAccount user", func() {
		var (
			result    resource
			orgName   string
			orgGUID   string
			spaceName string
			createErr cfErrs
		)

		BeforeEach(func() {
			orgName = generateGUID("org")
			orgGUID = createOrg(orgName)
			spaceName = generateGUID("space")
			createErr = cfErrs{}

			restyClient = tokenClient
		})

		AfterEach(func() {
			deleteOrg(orgGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = restyClient.R().
				SetBody(resource{
					Name: spaceName,
					Relationships: relationships{
						"organization": {Data: resource{GUID: orgGUID}},
					},
				}).
				SetError(&createErr).
				SetResult(&result).
				Post("/v3/spaces")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if len(createErr.Errors) == 0 {
				deleteSpace(result.GUID)
			}
		})

		When("the user is an org manager", func() {
			BeforeEach(func() {
				createOrgRole("organization_manager", serviceAccountName, orgGUID)
			})

			It("creates a space", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
				Expect(result.Name).To(Equal(spaceName))
				Expect(result.GUID).To(HavePrefix("cf-space-"))
				Expect(result.GUID).NotTo(BeEmpty())
			})
		})

		When("the user is an org user", func() {
			BeforeEach(func() {
				createOrgRole("organization_user", serviceAccountName, orgGUID)
			})

			It("cannot create a space", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
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
			result                                resourceList[resource]
			query                                 map[string]string
		)

		BeforeEach(func() {
			org1GUID, org2GUID, org3GUID = "", "", ""
			space11GUID, space12GUID, space13GUID = "", "", ""
			space21GUID, space22GUID, space23GUID = "", "", ""
			space31GUID, space32GUID, space33GUID = "", "", ""
			result = resourceList[resource]{}

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

			createOrgRole("organization_user", certUserName, org1GUID)
			createOrgRole("organization_user", certUserName, org2GUID)
			createOrgRole("organization_user", certUserName, org3GUID)

			createSpaceRole("space_developer", certUserName, space12GUID)
			createSpaceRole("space_developer", certUserName, space11GUID)
			createSpaceRole("space_developer", certUserName, space21GUID)
			createSpaceRole("space_developer", certUserName, space22GUID)
			createSpaceRole("space_developer", certUserName, space31GUID)
			createSpaceRole("space_developer", certUserName, space32GUID)
		})

		AfterEach(func() {
			orgIDs := []string{org1GUID, org2GUID, org3GUID}
			for _, id := range orgIDs {
				deleteOrg(id)
			}
		})

		JustBeforeEach(func() {
			var err error
			resp, err = restyClient.R().
				SetQueryParams(query).
				SetResult(&result).
				Get("/v3/spaces")
			Expect(err).NotTo(HaveOccurred())
		})

		It("lists the spaces the user has role in", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space11Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space12Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space21Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space22Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space31Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space32Name)}),
			))
			Expect(result.Resources).ToNot(ContainElement(MatchFields(IgnoreExtras, Fields{"Name": Equal(space13Name)})))
			Expect(result.Resources).ToNot(ContainElement(MatchFields(IgnoreExtras, Fields{"Name": Equal(space23Name)})))
			Expect(result.Resources).ToNot(ContainElement(MatchFields(IgnoreExtras, Fields{"Name": Equal(space33Name)})))
		})

		When("filtering by organization GUIDs", func() {
			BeforeEach(func() {
				query = map[string]string{
					"organization_guids": fmt.Sprintf("%s,%s", org1GUID, org3GUID),
				}
			})

			It("only lists spaces belonging to the orgs", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.Resources).To(ContainElements(
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
				Expect(result.Resources).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{"Name": Equal(space12Name)}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal(space32Name)}),
				))
			})
		})
	})

	Describe("delete", func() {
		var (
			spaceGUID string
			resultErr cfErrs
		)

		BeforeEach(func() {
			spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
			resultErr = cfErrs{}

			restyClient = adminClient
		})

		JustBeforeEach(func() {
			var err error
			resp, err = restyClient.R().
				SetError(&resultErr).
				Delete("/v3/spaces/" + spaceGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds with a job redirect", func() {
			Expect(resp).To(SatisfyAll(
				HaveRestyStatusCode(http.StatusAccepted),
				HaveRestyHeaderWithValue("Location", HaveSuffix("/v3/jobs/space.delete~"+spaceGUID)),
			))

			jobURL := resp.Header().Get("Location")
			Eventually(func(g Gomega) {
				jobResp, err := restyClient.R().Get(jobURL)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
			}).Should(Succeed())

			spaceResp, err := restyClient.R().Get("/v3/spaces/" + spaceGUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(spaceResp).To(HaveRestyStatusCode(http.StatusNotFound))
		})

		When("the space does not exist", func() {
			var originalSpaceGUID string

			BeforeEach(func() {
				originalSpaceGUID = spaceGUID
				spaceGUID = "nope"
			})

			AfterEach(func() {
				deleteSpace(originalSpaceGUID)
			})

			It("returns a not found error", func() {
				expectNotFoundError(resp, resultErr, "Space")
			})
		})
	})

	Describe("manifests", func() {
		var (
			spaceGUID          string
			resultErr          cfErrs
			manifestBytes      []byte
			manifest           manifestResource
			app1Name, app2Name string
		)

		BeforeEach(func() {
			spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
			resultErr = cfErrs{}
			app1Name = generateGUID("manifested-app-1")
			app2Name = generateGUID("manifested-app-2")

			route := fmt.Sprintf("%s.%s", app1Name, appFQDN)
			manifest = manifestResource{
				Version: 1,
				Applications: []applicationResource{{
					Name:    app1Name,
					Command: "whatever",
					Routes: []manifestRouteResource{{
						Route: &route,
					}},
					Metadata: metadata{
						Labels:      map[string]string{"foo": "FOO"},
						Annotations: map[string]string{"bar": "BAR"},
					},
				}, {
					Name: app2Name,
					Processes: []manifestApplicationProcessResource{{
						Type: "bob",
					}},
				}},
			}
		})

		AfterEach(func() {
			deleteSpace(spaceGUID)
		})

		Describe("apply manifest", func() {
			JustBeforeEach(func() {
				var err error
				manifestBytes, err = yaml.Marshal(manifest)
				Expect(err).NotTo(HaveOccurred())
				resp, err = restyClient.R().
					SetHeader("Content-type", "application/x-yaml").
					SetBody(manifestBytes).
					SetError(&resultErr).
					Post("/v3/spaces/" + spaceGUID + "/actions/apply_manifest")
				Expect(err).NotTo(HaveOccurred())
			})

			When("the user has space developer role in the space", func() {
				BeforeEach(func() {
					createSpaceRole("space_developer", certUserName, spaceGUID)
				})

				It("succeeds", func() {
					Expect(resp).To(SatisfyAll(
						HaveRestyStatusCode(http.StatusAccepted),
						HaveRestyHeaderWithValue("Location", HaveSuffix("/v3/jobs/space.apply_manifest~"+spaceGUID)),
					))

					jobURL := resp.Header().Get("Location")
					Eventually(func(g Gomega) {
						jobResp, err := restyClient.R().Get(jobURL)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
					}).Should(Succeed())

					app1GUID := getAppGUIDFromName(app1Name)

					app1 := getApp(app1GUID)
					Expect(app1.Metadata).NotTo(BeNil())
					Expect(app1.Metadata.Labels).To(HaveKeyWithValue("foo", "FOO"))
					Expect(app1.Metadata.Annotations).To(HaveKeyWithValue("bar", "BAR"))

					app1Process := getProcess(app1GUID, "web")
					Expect(app1Process.Instances).To(Equal(1))
					Expect(app1Process.Command).To(Equal("whatever"))

					app2GUID := getAppGUIDFromName(app2Name)
					Expect(getProcess(app2GUID, "web").Instances).To(Equal(1))
					Expect(getProcess(app2GUID, "bob").Instances).To(Equal(0))
				})

				When("the app already exists", func() {
					applyManifest := func(m manifestResource) {
						mBytes, err := yaml.Marshal(m)
						Expect(err).NotTo(HaveOccurred())

						var requestErr error
						r, err := restyClient.R().
							SetHeader("Content-type", "application/x-yaml").
							SetBody(mBytes).
							SetError(&requestErr).
							Post("/v3/spaces/" + spaceGUID + "/actions/apply_manifest")
						Expect(err).NotTo(HaveOccurred())
						Expect(requestErr).NotTo(HaveOccurred())

						Expect(r).To(HaveRestyStatusCode(http.StatusAccepted))

						jobURL := r.Header().Get("Location")
						Eventually(func(g Gomega) {
							jobResp, err := restyClient.R().Get(jobURL)
							g.Expect(err).NotTo(HaveOccurred())
							g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
						}).Should(Succeed())
					}

					BeforeEach(func() {
						applyManifest(manifestResource{
							Version: 1,
							Applications: []applicationResource{{
								Name:    app1Name,
								Command: "ogWhatever",
								Metadata: metadata{
									Labels:      map[string]string{"foo": "ogFOO", "baz": "luhrmann"},
									Annotations: map[string]string{"bar": "ogBAR", "fizz": "buzz"},
								},
								Processes: []manifestApplicationProcessResource{{
									Type: "worker",
								}},
							}},
						})
					})

					It("applies the changes correctly", func() {
						Expect(resp).To(HaveRestyStatusCode(http.StatusAccepted))

						jobURL := resp.Header().Get("Location")
						Eventually(func(g Gomega) {
							jobResp, err := restyClient.R().Get(jobURL)
							g.Expect(err).NotTo(HaveOccurred())
							g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
						}).Should(Succeed())

						app1GUID := getAppGUIDFromName(app1Name)

						app1 := getApp(app1GUID)
						Expect(app1.Metadata).NotTo(BeNil())
						Expect(app1.Metadata.Labels).To(SatisfyAll(
							HaveKeyWithValue("foo", "FOO"),
							HaveKeyWithValue("baz", "luhrmann"),
						))
						Expect(app1.Metadata.Annotations).To(SatisfyAll(
							HaveKeyWithValue("bar", "BAR"),
							HaveKeyWithValue("fizz", "buzz"),
						))

						app1Process := getProcess(app1GUID, "web")
						Expect(app1Process.Instances).To(Equal(1))
						Expect(app1Process.Command).To(Equal("whatever"))
						Expect(getProcess(app1GUID, "worker").Instances).To(Equal(0))

						app2GUID := getAppGUIDFromName(app2Name)
						Expect(getProcess(app2GUID, "web").Instances).To(Equal(1))
						Expect(getProcess(app2GUID, "bob").Instances).To(Equal(0))
					})
				})
			})

			When("the user has space manager role in the space", func() {
				BeforeEach(func() {
					createSpaceRole("space_manager", certUserName, spaceGUID)
				})

				It("returns 403", func() {
					Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
					Expect(resultErr.Errors).To(ConsistOf(cfErr{
						Detail: "You are not authorized to perform the requested action",
						Title:  "CF-NotAuthorized",
						Code:   10003,
					}))
				})
			})

			When("the user has no role in the space", func() {
				It("returns 404", func() {
					Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
					Expect(resultErr.Errors).To(ConsistOf(
						cfErr{
							Detail: "Space not found. Ensure it exists and you have access to it.",
							Title:  "CF-ResourceNotFound",
							Code:   10010,
						},
					))
				})
			})
		})
	})

	Describe("manifest diff", func() {
		var (
			spaceGUID     string
			errResp       cfErrs
			manifestBytes []byte
		)

		BeforeEach(func() {
			spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
			restyClient = certClient
			manifestBytes = []byte{}
		})

		AfterEach(func() {
			deleteSpace(spaceGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = restyClient.R().
				SetHeader("Content-type", "application/x-yaml").
				SetBody(manifestBytes).
				SetError(&errResp).
				Post("/v3/spaces/" + spaceGUID + "/manifest_diff")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user has space-developer permissions in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("returns the diff response JSON", func() {
				Expect(resp.StatusCode()).To(Equal(http.StatusAccepted))

				diff := map[string]interface{}{}
				Expect(json.Unmarshal(resp.Body(), &diff)).To(Succeed())
				// The `manifest_diff` endpoint is currently a stub to return
				// an empty diff
				Expect(diff).To(HaveKeyWithValue("diff", BeEmpty()))
			})
		})

		When("the user has no permissions", func() {
			BeforeEach(func() {
				restyClient = unprivilegedServiceAccountClient
			})

			It("returns a Not-Found error", func() {
				expectNotFoundError(resp, errResp, "Space")
			})
		})
	})
})
