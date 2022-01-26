package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	workloads "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

var _ = Describe("App Handler", func() {
	var (
		apiHandler *AppHandler
		namespace  *corev1.Namespace
	)

	BeforeEach(func() {
		clientFactory := repositories.NewUnprivilegedClientFactory(k8sConfig)
		identityProvider := new(fake.IdentityProvider)
		nsPermissions := authorization.NewNamespacePermissions(k8sClient, identityProvider, "root-ns")
		appRepo := repositories.NewAppRepo(k8sClient, clientFactory, nsPermissions)
		dropletRepo := repositories.NewDropletRepo(k8sClient)
		processRepo := repositories.NewProcessRepo(k8sClient)
		routeRepo := repositories.NewRouteRepo(k8sClient, clientFactory)
		domainRepo := repositories.NewDomainRepo(k8sClient)
		podRepo := repositories.NewPodRepo(k8sClient)
		scaleProcess := actions.NewScaleProcess(processRepo).Invoke
		scaleAppProcess := actions.NewScaleAppProcess(appRepo, processRepo, scaleProcess).Invoke

		apiHandler = NewAppHandler(
			logf.Log.WithName("integration tests"),
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			podRepo,
			scaleAppProcess,
		)
		apiHandler.RegisterRoutes(router)

		namespaceGUID := generateGUID()
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceGUID}}
		Expect(
			k8sClient.Create(ctx, namespace),
		).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	Describe("POST /v3/apps endpoint", func() {
		When("on the happy path", func() {
			const (
				appName = "my-test-app"
			)
			var testEnvironmentVariables map[string]string

			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace.Name)

				testEnvironmentVariables = map[string]string{"foo": "foo", "bar": "bar"}
				envJSON, _ := json.Marshal(&testEnvironmentVariables)
				requestBody := fmt.Sprintf(`{
                    "name": %q,
                    "relationships": {
                        "space": {
                            "data": {
                                "guid": %q
                            }
                        }
                    },
                    "environment_variables": %s
                }`, appName, namespace.Name, envJSON)

				var err error
				req, err = http.NewRequestWithContext(ctx, "POST", serverURI("/v3/apps"), strings.NewReader(requestBody))
				Expect(err).NotTo(HaveOccurred())

				req.Header.Add("Content-type", "application/json")
			})

			JustBeforeEach(func() {
				router.ServeHTTP(rr, req)
			})

			It("creates a CFApp and Secret, returns 201 and an App object as JSON", func() {
				Expect(rr.Code).To(Equal(201))

				var parsedBody map[string]interface{}
				body, err := ioutil.ReadAll(rr.Body)
				Expect(err).NotTo(HaveOccurred())

				Expect(
					json.Unmarshal(body, &parsedBody),
				).To(Succeed())

				Expect(parsedBody).To(MatchKeys(IgnoreExtras, Keys{
					"guid":       Not(BeEmpty()),
					"name":       Equal("my-test-app"),
					"state":      Equal("STOPPED"),
					"created_at": Not(BeEmpty()),
					"relationships": Equal(map[string]interface{}{
						"space": map[string]interface{}{
							"data": map[string]interface{}{
								"guid": namespace.Name,
							},
						},
					}),
				}))

				appGUID := parsedBody["guid"].(string)
				appNSName := types.NamespacedName{
					Name:      appGUID,
					Namespace: namespace.Name,
				}
				var appRecord workloads.CFApp
				Eventually(func() error {
					return k8sClient.Get(ctx, appNSName, &appRecord)
				}).Should(Succeed())

				Expect(appRecord.Spec.Name).To(Equal("my-test-app"))
				Expect(appRecord.Spec.DesiredState).To(BeEquivalentTo("STOPPED"))
				Expect(appRecord.Spec.EnvSecretName).NotTo(BeEmpty())

				secretNSName := types.NamespacedName{
					Name:      appRecord.Spec.EnvSecretName,
					Namespace: namespace.Name,
				}
				var secretCR corev1.Secret
				Eventually(func() error {
					return k8sClient.Get(ctx, secretNSName, &secretCR)
				}).Should(Succeed())

				Expect(secretCR.Data).To(MatchAllKeys(Keys{
					"foo": BeEquivalentTo(testEnvironmentVariables["foo"]),
					"bar": BeEquivalentTo(testEnvironmentVariables["bar"]),
				}))
			})
		})
	})

	Describe("app sub-resources", func() {
		var (
			app         *workloads.CFApp
			dropletGUID string
		)

		BeforeEach(func() {
			appGUID := generateGUID()
			dropletGUID = generateGUID()

			app = &workloads.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appGUID,
					Namespace: namespace.Name,
				},
				Spec: workloads.CFAppSpec{
					Name:         generateGUID(),
					DesiredState: "STOPPED",
					Lifecycle: workloads.Lifecycle{
						Type: "buildpack",
					},
					CurrentDropletRef: corev1.LocalObjectReference{
						Name: dropletGUID,
					},
				},
			}

			Expect(k8sClient.Create(ctx, app)).To(Succeed())
		})

		Describe("get processes", func() {
			JustBeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, http.MethodGet, serverURI("/v3/apps/"+app.Name+"/processes"), nil)
				Expect(err).NotTo(HaveOccurred())

				router.ServeHTTP(rr, req)
			})

			When("the user is not authorized in the space", func() {
				It("returns a not found status", func() {
					Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
				})
			})

			When("the user is a space developer", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace.Name)
				})

				It("returns the (empty) process list", func() {
					Expect(rr).To(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(ContainSubstring(`"resources":[]`)), rr.Body.String())
				})
			})
		})

		Describe("get routes", func() {
			JustBeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, http.MethodGet, serverURI("/v3/apps/"+app.Name+"/routes"), nil)
				Expect(err).NotTo(HaveOccurred())

				router.ServeHTTP(rr, req)
			})

			When("the user is not authorized in the space", func() {
				It("returns a not found status", func() {
					Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
					Expect(rr).To(HaveHTTPBody(ContainSubstring("App not found")), rr.Body.String())
				})
			})

			When("the user is a space developer", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace.Name)
				})

				It("returns the (empty) route list", func() {
					Expect(rr).To(HaveHTTPStatus(http.StatusOK))
					Expect(rr).To(HaveHTTPBody(ContainSubstring(`"resources":[]`)), rr.Body.String())
				})
			})
		})

		Describe("droplets", func() {
			var droplet *workloads.CFBuild

			BeforeEach(func() {
				droplet = &workloads.CFBuild{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dropletGUID,
						Namespace: namespace.Name,
					},
					Spec: workloads.CFBuildSpec{
						AppRef: corev1.LocalObjectReference{
							Name: app.Name,
						},
						Lifecycle: workloads.Lifecycle{
							Type: "buildpack",
						},
					},
				}
				Expect(k8sClient.Create(ctx, droplet)).To(Succeed())
				droplet.Status = workloads.CFBuildStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "Staging",
							Status:             metav1.ConditionFalse,
							Reason:             "foo",
							LastTransitionTime: metav1.NewTime(time.Now()),
						},
						{
							Type:               "Succeeded",
							Status:             metav1.ConditionTrue,
							Reason:             "foo",
							LastTransitionTime: metav1.NewTime(time.Now()),
						},
					},
					BuildDropletStatus: &workloads.BuildDropletStatus{
						ProcessTypes: []workloads.ProcessType{},
						Ports:        []int32{},
					},
				}
				Expect(k8sClient.Status().Update(ctx, droplet)).To(Succeed())
			})

			Describe("get current droplet", func() {
				JustBeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, http.MethodGet, serverURI("/v3/apps/"+app.Name+"/droplets/current"), nil)
					Expect(err).NotTo(HaveOccurred())

					router.ServeHTTP(rr, req)
				})

				When("having the space developer role", func() {
					BeforeEach(func() {
						createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace.Name)
					})

					It("gets the droplet", func() {
						Expect(rr).To(HaveHTTPStatus(http.StatusOK))
					})
				})

				When("not a space developer or admin", func() {
					It("returns a 404", func() {
						Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
						Expect(rr).To(HaveHTTPBody(ContainSubstring("App not found")))
					})
				})
			})

			Describe("set current droplet", func() {
				JustBeforeEach(func() {
					payload, err := json.Marshal(payloads.AppSetCurrentDroplet{
						Relationship: payloads.Relationship{
							Data: &payloads.RelationshipData{
								GUID: droplet.Name,
							},
						},
					})
					Expect(err).NotTo(HaveOccurred())

					req, err = http.NewRequestWithContext(ctx, http.MethodPatch, serverURI("/v3/apps/"+app.Name+"/relationships/current_droplet"), bytes.NewReader(payload))
					Expect(err).NotTo(HaveOccurred())

					router.ServeHTTP(rr, req)
				})

				When("having the space developer role", func() {
					BeforeEach(func() {
						createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace.Name)
					})

					It("sets the droplet", func() {
						Expect(rr).To(HaveHTTPStatus(http.StatusOK))
					})
				})

				When("no access to app", func() {
					It("returns a 404", func() {
						Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
						Expect(rr).To(HaveHTTPBody(ContainSubstring("App not found")), rr.Body.String())
					})
				})

				When("access to app but no write permissions", func() {
					BeforeEach(func() {
						createRoleBinding(ctx, userName, spaceManagerRole.Name, namespace.Name)
					})

					It("returns a 403", func() {
						Expect(rr).To(HaveHTTPStatus(http.StatusForbidden))
						Expect(rr).To(HaveHTTPBody(ContainSubstring("CF-NotAuthorized")), rr.Body.String())
					})
				})
			})
		})
	})
})
