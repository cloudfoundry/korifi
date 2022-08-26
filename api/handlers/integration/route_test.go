package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Route Handler", func() {
	var (
		apiHandler *RouteHandler
		space      *korifiv1alpha1.CFSpace
	)

	BeforeEach(func() {
		appRepo := repositories.NewAppRepo(namespaceRetriever, clientFactory, nsPermissions, conditions.NewConditionAwaiter[*korifiv1alpha1.CFApp, korifiv1alpha1.CFAppList](2*time.Second))
		orgRepo := repositories.NewOrgRepo(rootNamespace, k8sClient, clientFactory, nsPermissions, time.Minute)
		spaceRepo := repositories.NewSpaceRepo(namespaceRetriever, orgRepo, clientFactory, nsPermissions, time.Minute)
		routeRepo := repositories.NewRouteRepo(namespaceRetriever, clientFactory, nsPermissions)
		domainRepo := repositories.NewDomainRepo(clientFactory, namespaceRetriever, rootNamespace)
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler = NewRouteHandler(
			*serverURL,
			routeRepo,
			domainRepo,
			appRepo,
			spaceRepo,
			decoderValidator,
		)
		apiHandler.RegisterRoutes(router)

		org := createOrgWithCleanup(ctx, generateGUID())
		space = createSpaceWithCleanup(ctx, org.Name, "spacename-"+generateGUID())
	})

	Describe("GET /v3/routes endpoint", func() {
		When("a space developer calls list on routes in namespace they have permission for", func() {
			var (
				domainGUID string
				routeGUID1 string
				cfDomain   *korifiv1alpha1.CFDomain
				cfRoute1   *korifiv1alpha1.CFRoute
				routeGUID2 string
				cfRoute2   *korifiv1alpha1.CFRoute
			)

			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)

				domainGUID = generateGUIDWithPrefix("domain")
				routeGUID1 = generateGUIDWithPrefix("route1")
				routeGUID2 = generateGUIDWithPrefix("route2")

				cfDomain = &korifiv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Spec: korifiv1alpha1.CFDomainSpec{
						Name: "foo.domain",
					},
				}
				Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())

				cfRoute1 = &korifiv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeGUID1,
						Namespace: space.Name,
					},
					Spec: korifiv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-1",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.ObjectReference{
							Name:      domainGUID,
							Namespace: rootNamespace,
						},
						Destinations: []korifiv1alpha1.Destination{
							{
								GUID: "destination-guid",
								Port: 8080,
								AppRef: corev1.LocalObjectReference{
									Name: "some-app-guid",
								},
								ProcessType: "web",
								Protocol:    "http1",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, cfRoute1)).To(Succeed())

				cfRoute2 = &korifiv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeGUID2,
						Namespace: space.Name,
					},
					Spec: korifiv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-2",
						Path:     "foo",
						Protocol: "http",
						DomainRef: corev1.ObjectReference{
							Name:      domainGUID,
							Namespace: rootNamespace,
						},
						Destinations: []korifiv1alpha1.Destination{
							{
								GUID: "destination-guid",
								Port: 8080,
								AppRef: corev1.LocalObjectReference{
									Name: "some-app-guid",
								},
								ProcessType: "web",
								Protocol:    "http1",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, cfRoute2)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, cfRoute1)).To(Succeed())
				Expect(k8sClient.Delete(ctx, cfRoute2)).To(Succeed())
				Expect(k8sClient.Delete(ctx, cfDomain)).To(Succeed())
			})

			JustBeforeEach(func() {
				router.ServeHTTP(rr, req)
			})

			When("no query parameters are specified", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", serverURI("/v3/routes"), strings.NewReader(""))
					Expect(err).NotTo(HaveOccurred())

					req.Header.Add("Content-type", "application/json")
				})

				It("returns 200 and returns 2 records", func() {
					Expect(rr.Code).To(Equal(http.StatusOK))

					response := map[string]interface{}{}
					err := json.Unmarshal(rr.Body.Bytes(), &response)
					Expect(err).NotTo(HaveOccurred())
					Expect(response).To(HaveKeyWithValue("resources", HaveLen(2)))
				})
			})

			When("the hosts query parameter is specified with a value", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", serverURI("/v3/routes?hosts=my-subdomain-1"), strings.NewReader(""))
					Expect(err).NotTo(HaveOccurred())

					req.Header.Add("Content-type", "application/json")
				})

				It("returns 200 and returns 1 record", func() {
					Expect(rr.Code).To(Equal(http.StatusOK))

					response := map[string]interface{}{}
					err := json.Unmarshal(rr.Body.Bytes(), &response)
					Expect(err).NotTo(HaveOccurred())
					Expect(response).To(HaveKeyWithValue("resources", HaveLen(1)))
				})
			})

			When("the paths query parameter is specified with no value", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", serverURI("/v3/routes?paths="), strings.NewReader(""))
					Expect(err).NotTo(HaveOccurred())

					req.Header.Add("Content-type", "application/json")
				})

				It("returns 200 and returns 1 record", func() {
					Expect(rr.Code).To(Equal(http.StatusOK))

					response := map[string]interface{}{}
					err := json.Unmarshal(rr.Body.Bytes(), &response)
					Expect(err).NotTo(HaveOccurred())
					Expect(response).To(HaveKeyWithValue("resources", HaveLen(1)))
				})
			})
		})
	})

	Describe("DELETE /v3/route endpoint", func() {
		When("a space developer calls delete on a route in namespace they have permission for", func() {
			var (
				routeGUID  string
				domainGUID string
				cfDomain   *korifiv1alpha1.CFDomain
				cfRoute    *korifiv1alpha1.CFRoute
			)

			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)

				routeGUID = generateGUIDWithPrefix("route")
				domainGUID = generateGUIDWithPrefix("domain")

				cfDomain = &korifiv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Spec: korifiv1alpha1.CFDomainSpec{
						Name: "foo.domain",
					},
				}
				Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())

				cfRoute = &korifiv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeGUID,
						Namespace: space.Name,
					},
					Spec: korifiv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-1",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.ObjectReference{
							Name:      domainGUID,
							Namespace: rootNamespace,
						},
						Destinations: []korifiv1alpha1.Destination{
							{
								GUID: "destination-guid",
								Port: 8080,
								AppRef: corev1.LocalObjectReference{
									Name: "some-app-guid",
								},
								ProcessType: "web",
								Protocol:    "http1",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())

				var err error
				req, err = http.NewRequestWithContext(ctx, "DELETE", serverURI("/v3/routes/"+routeGUID), strings.NewReader(""))
				Expect(err).NotTo(HaveOccurred())

				req.Header.Add("Content-type", "application/json")
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, cfDomain)).To(Succeed())
			})

			JustBeforeEach(func() {
				router.ServeHTTP(rr, req)
			})

			It("returns 202 and eventually deletes the route", func() {
				testCtx := context.Background()
				Expect(rr.Code).To(Equal(202))

				Eventually(func() error {
					route := &korifiv1alpha1.CFRoute{}
					return k8sClient.Get(testCtx, client.ObjectKey{Namespace: space.Name, Name: routeGUID}, route)
				}).Should(MatchError(ContainSubstring("not found")))
			})
		})
	})

	Describe("POST /v3/routes/{guid}/destinations endpoint", func() {
		const (
			newDestinationAppGUID = "new-destination-app-guid"
		)

		var (
			domainGUID string
			cfDomain   *korifiv1alpha1.CFDomain
			cfRoute    *korifiv1alpha1.CFRoute
		)

		BeforeEach(func() {
			domainGUID = generateGUIDWithPrefix("domain")

			cfDomain = &korifiv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      domainGUID,
					Namespace: rootNamespace,
				},
				Spec: korifiv1alpha1.CFDomainSpec{
					Name: "foo.domain",
				},
			}
			Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())

			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      generateGUIDWithPrefix("route"),
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []korifiv1alpha1.Destination{
						{
							GUID: "destination-guid",
							Port: 8080,
							AppRef: corev1.LocalObjectReference{
								Name: "some-app-guid",
							},
							ProcessType: "web",
							Protocol:    "http1",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())

			requestBody := fmt.Sprintf(`{
						"destinations": [
							{
								"app": {
									"guid": %q
								},
								"protocol": "http1"
							}
						]
					}`, newDestinationAppGUID)

			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", serverURI("/v3/routes/"+cfRoute.Name+"/destinations"), strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Content-type", "application/json")
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, cfRoute)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cfDomain)).To(Succeed())
		})

		JustBeforeEach(func() {
			router.ServeHTTP(rr, req)
		})

		When("the user is not authorized in the space", func() {
			It("returns a not found status", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
				Expect(rr).To(HaveHTTPBody(ContainSubstring("Route not found")), rr.Body.String())
			})
		})

		When("the user has readonly access to the route", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceManagerRole.Name, space.Name)

				getReq, err := http.NewRequestWithContext(ctx, "GET", serverURI("/v3/routes/"+cfRoute.Name), strings.NewReader(""))
				Expect(err).NotTo(HaveOccurred())

				getReqRR := httptest.NewRecorder()
				router.ServeHTTP(getReqRR, getReq)
				Expect(getReqRR.Code).To(Equal(http.StatusOK))
			})

			It("returns a forbidden error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusForbidden))
			})
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)

				getReq, err := http.NewRequestWithContext(ctx, "GET", serverURI("/v3/routes/"+cfRoute.Name), strings.NewReader(""))
				Expect(err).NotTo(HaveOccurred())

				getReqRR := httptest.NewRecorder()
				router.ServeHTTP(getReqRR, getReq)
				Expect(getReqRR.Code).To(Equal(http.StatusOK))
			})

			It("updates the route", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPBody(ContainSubstring(newDestinationAppGUID)), rr.Body.String())

				cfRouteLookupKey := client.ObjectKeyFromObject(cfRoute)
				updatedCFRoute := new(korifiv1alpha1.CFRoute)
				Expect(k8sClient.Get(context.Background(), cfRouteLookupKey, updatedCFRoute)).To(Succeed())

				Expect(updatedCFRoute.Spec.Destinations).To(ConsistOf(
					Equal(cfRoute.Spec.Destinations[0]),
					MatchFields(IgnoreExtras,
						Fields{
							"AppRef": Equal(corev1.LocalObjectReference{
								Name: newDestinationAppGUID,
							}),
							"Protocol": Equal("http1"),
						},
					),
				))
			})
		})
	})
})
