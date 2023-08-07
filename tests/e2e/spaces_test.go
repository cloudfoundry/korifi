package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"gopkg.in/yaml.v3"
)

var _ = Describe("Spaces", func() {
	var resp *resty.Response

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
	})

	Describe("list", func() {
		var (
			org1GUID, org2GUID       string
			space11GUID, space12GUID string
			space21GUID, space22GUID string
			space11Name, space12Name string
			space21Name, space22Name string
			result                   resourceList[resource]
		)

		BeforeEach(func() {
			org1GUID, org2GUID = "", ""
			space11GUID, space12GUID = "", ""
			space21GUID, space22GUID = "", ""
			result = resourceList[resource]{}

			var orgWG sync.WaitGroup
			orgErrChan := make(chan error, 3)

			orgWG.Add(2)
			asyncCreateOrg(generateGUID("org1"), &org1GUID, &orgWG, orgErrChan)
			asyncCreateOrg(generateGUID("org2"), &org2GUID, &orgWG, orgErrChan)
			orgWG.Wait()

			var err error
			Expect(orgErrChan).ToNot(Receive(&err), func() string { return fmt.Sprintf("unexpected error occurred while creating orgs: %v", err) })
			close(orgErrChan)

			var spaceWG sync.WaitGroup

			space11Name, space12Name = generateGUID("space11"), generateGUID("space12")
			space21Name, space22Name = generateGUID("space21"), generateGUID("space22")

			spaceErrChan := make(chan error, 4)
			spaceWG.Add(4)
			asyncCreateSpace(space11Name, org1GUID, &space11GUID, &spaceWG, spaceErrChan)
			asyncCreateSpace(space12Name, org1GUID, &space12GUID, &spaceWG, spaceErrChan)

			asyncCreateSpace(space21Name, org2GUID, &space21GUID, &spaceWG, spaceErrChan)
			asyncCreateSpace(space22Name, org2GUID, &space22GUID, &spaceWG, spaceErrChan)

			spaceWG.Wait()

			Expect(spaceErrChan).ToNot(Receive(&err), func() string { return fmt.Sprintf("unexpected error occurred while creating spaces: %v", err) })
			close(spaceErrChan)
		})

		AfterEach(func() {
			orgIDs := []string{org1GUID, org2GUID}
			for _, id := range orgIDs {
				deleteOrg(id)
			}
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&result).
				Get("/v3/spaces")
			Expect(err).NotTo(HaveOccurred())
		})

		It("lists spaces", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Resources).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space11Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space12Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space21Name)}),
				MatchFields(IgnoreExtras, Fields{"Name": Equal(space22Name)}),
			))
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
				HaveRestyHeaderWithValue("Location", HaveSuffix("/v3/jobs/space.delete~"+spaceGUID)),
			))

			jobURL := resp.Header().Get("Location")
			Eventually(func(g Gomega) {
				jobResp, err := adminClient.R().Get(jobURL)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
			}).Should(Succeed())

			spaceResp, err := adminClient.R().Get("/v3/spaces/" + spaceGUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(spaceResp).To(HaveRestyStatusCode(http.StatusNotFound))
		})
	})

	Describe("manifests", func() {
		var (
			spaceGUID                string
			resultErr                cfErrs
			manifestBytes            []byte
			manifest                 manifestResource
			app1Name, app2Name       string
			serviceName, serviceGUID string
			serviceBindingName       string
		)

		BeforeEach(func() {
			spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
			resultErr = cfErrs{}
			serviceName = uuid.NewString()
			serviceGUID = createServiceInstance(spaceGUID, serviceName, map[string]string{})
			app1Name = generateGUID("manifested-app-1")
			app2Name = generateGUID("manifested-app-2")
			serviceBindingName = uuid.NewString()

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
					Services: []serviceResource{{
						Name:        serviceName,
						BindingName: serviceBindingName,
					}},
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
				resp, err = adminClient.R().
					SetHeader("Content-type", "application/x-yaml").
					SetBody(manifestBytes).
					SetError(&resultErr).
					Post("/v3/spaces/" + spaceGUID + "/actions/apply_manifest")
				Expect(err).NotTo(HaveOccurred())
			})

			It("succeeds", func() {
				Expect(resp).To(SatisfyAll(
					HaveRestyStatusCode(http.StatusAccepted),
					HaveRestyHeaderWithValue("Location", HaveSuffix("/v3/jobs/space.apply_manifest~"+spaceGUID)),
				))

				jobURL := resp.Header().Get("Location")
				Eventually(func(g Gomega) {
					jobResp, err := adminClient.R().Get(jobURL)
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

				app1ServiceBindings := getServiceBindingsForApp(app1GUID)
				Expect(app1ServiceBindings).To(HaveLen(1))
				Expect(app1ServiceBindings[0].Name).To(Equal(serviceBindingName))
				Expect(app1ServiceBindings[0].Relationships["app"].Data.GUID).To(Equal(app1GUID))
				Expect(app1ServiceBindings[0].Relationships["service_instance"].Data.GUID).To(Equal(serviceGUID))

				app1Env := getAppEnv(app1GUID)
				Expect(app1Env).To(
					HaveKeyWithValue("system_env_json",
						HaveKeyWithValue("VCAP_SERVICES",
							HaveKeyWithValue("user-provided",
								ContainElement(SatisfyAll(
									HaveKeyWithValue("instance_guid", serviceGUID),
									HaveKeyWithValue("instance_name", serviceName),
									HaveKeyWithValue("binding_name", serviceBindingName),
								)),
							),
						),
					),
				)

				app2GUID := getAppGUIDFromName(app2Name)
				Expect(getProcess(app2GUID, "web").Instances).To(Equal(1))
				Expect(getProcess(app2GUID, "bob").Instances).To(Equal(0))
			})

			When("the app already exists", func() {
				applyManifest := func(m manifestResource) {
					mBytes, err := yaml.Marshal(m)
					Expect(err).NotTo(HaveOccurred())

					var requestErr error
					r, err := adminClient.R().
						SetHeader("Content-type", "application/x-yaml").
						SetBody(mBytes).
						SetError(&requestErr).
						Post("/v3/spaces/" + spaceGUID + "/actions/apply_manifest")
					Expect(err).NotTo(HaveOccurred())
					Expect(requestErr).NotTo(HaveOccurred())

					Expect(r).To(HaveRestyStatusCode(http.StatusAccepted))

					jobURL := r.Header().Get("Location")
					Eventually(func(g Gomega) {
						jobResp, err := adminClient.R().Get(jobURL)
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
						jobResp, err := adminClient.R().Get(jobURL)
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
	})

	Describe("manifest diff", func() {
		var (
			spaceGUID     string
			errResp       cfErrs
			manifestBytes []byte
		)

		BeforeEach(func() {
			spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
			manifestBytes = []byte{}
		})

		AfterEach(func() {
			deleteSpace(spaceGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetHeader("Content-type", "application/x-yaml").
				SetBody(manifestBytes).
				SetError(&errResp).
				Post("/v3/spaces/" + spaceGUID + "/manifest_diff")
			Expect(err).NotTo(HaveOccurred())
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

	Describe("get", func() {
		var (
			spaceGUID string
			result    resource
		)

		BeforeEach(func() {
			spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetResult(&result).
				Get("/v3/spaces/" + spaceGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the space", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.GUID).To(Equal(spaceGUID))
		})
	})
})
