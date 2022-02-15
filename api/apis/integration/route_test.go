package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Route Handler", func() {
	var (
		apiHandler    *RouteHandler
		namespace     *corev1.Namespace
		namespaceGUID string
	)

	BeforeEach(func() {
		appRepo := repositories.NewAppRepo(k8sClient, clientFactory, nsPermissions)
		orgRepo := repositories.NewOrgRepo("root-ns", k8sClient, clientFactory, nsPermissions, time.Minute, true)
		routeRepo := repositories.NewRouteRepo(k8sClient, clientFactory)
		domainRepo := repositories.NewDomainRepo(k8sClient, clientFactory)
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler = NewRouteHandler(
			logf.Log.WithName("TestRouteHandler"),
			*serverURL,
			routeRepo,
			domainRepo,
			appRepo,
			orgRepo,
			decoderValidator,
		)
		apiHandler.RegisterRoutes(router)

		namespaceGUID = generateGUID()
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceGUID}}
		Expect(
			k8sClient.Create(ctx, namespace),
		).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	Describe("GET /v3/routes endpoint", func() {
		When("a space developer calls list on routes in namespace they have permission for", func() {
			var (
				domainGUID string
				routeGUID1 string
				cfRoute1   *networkingv1alpha1.CFRoute
				routeGUID2 string
				cfRoute2   *networkingv1alpha1.CFRoute
			)

			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace.Name)

				domainGUID = generateGUID()
				routeGUID1 = generateGUID()
				routeGUID2 = generateGUID()

				cfDomain := &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      domainGUID,
						Namespace: namespaceGUID,
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: "foo.domain",
					},
				}
				Expect(
					k8sClient.Create(ctx, cfDomain),
				).To(Succeed())

				cfRoute1 = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeGUID1,
						Namespace: namespace.Name,
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-1",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.ObjectReference{
							Name:      domainGUID,
							Namespace: namespace.Name,
						},
						Destinations: []networkingv1alpha1.Destination{
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
				Expect(
					k8sClient.Create(ctx, cfRoute1),
				).To(Succeed())
				DeferCleanup(func() {
					Expect(
						k8sClient.Delete(ctx, cfRoute1),
					).To(Succeed())
				})

				cfRoute2 = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeGUID2,
						Namespace: namespace.Name,
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-2",
						Path:     "foo",
						Protocol: "http",
						DomainRef: corev1.ObjectReference{
							Name:      domainGUID,
							Namespace: namespace.Name,
						},
						Destinations: []networkingv1alpha1.Destination{
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
				Expect(
					k8sClient.Create(ctx, cfRoute2),
				).To(Succeed())
				DeferCleanup(func() {
					Expect(
						k8sClient.Delete(ctx, cfRoute2),
					).To(Succeed())
				})

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

				XIt("returns 200 and returns 2 records", func() {
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
				cfRoute    *networkingv1alpha1.CFRoute
			)

			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace.Name)

				routeGUID = generateGUID()
				domainGUID = generateGUID()

				cfDomain := &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      domainGUID,
						Namespace: namespaceGUID,
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: "foo.domain",
					},
				}
				Expect(
					k8sClient.Create(ctx, cfDomain),
				).To(Succeed())

				cfRoute = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeGUID,
						Namespace: namespace.Name,
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-1",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.ObjectReference{
							Name:      domainGUID,
							Namespace: namespace.Name,
						},
						Destinations: []networkingv1alpha1.Destination{
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
				Expect(
					k8sClient.Create(ctx, cfRoute),
				).To(Succeed())

				var err error
				req, err = http.NewRequestWithContext(ctx, "DELETE", serverURI("/v3/routes/"+routeGUID), strings.NewReader(""))
				Expect(err).NotTo(HaveOccurred())

				req.Header.Add("Content-type", "application/json")
			})

			JustBeforeEach(func() {
				router.ServeHTTP(rr, req)
			})

			It("returns 202 and eventually deletes the route", func() {
				testCtx := context.Background()
				Expect(rr.Code).To(Equal(202))

				Eventually(func() error {
					route := &networkingv1alpha1.CFRoute{}
					return k8sClient.Get(testCtx, client.ObjectKey{Namespace: namespace.Name, Name: routeGUID}, route)
				}).Should(MatchError(ContainSubstring("not found")))
			})
		})
	})

	Describe("POST /v3/routes/{guid}/destinations endpoint", func() {

		var (
			domainGUID string
			routeGUID1 string
			cfRoute1   *networkingv1alpha1.CFRoute

			newDestinationAppGUID string
		)

		BeforeEach(func() {

			domainGUID = generateGUID()
			routeGUID1 = generateGUID()

			cfDomain := &networkingv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      domainGUID,
					Namespace: rootNamespace,
				},
				Spec: networkingv1alpha1.CFDomainSpec{
					Name: "foo.domain",
				},
			}
			Expect(
				k8sClient.Create(ctx, cfDomain),
			).To(Succeed())
			// give test user access to the Domain
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, rootNamespace)

			cfRoute1 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeGUID1,
					Namespace: namespace.Name,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []networkingv1alpha1.Destination{
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
			Expect(
				k8sClient.Create(ctx, cfRoute1),
			).To(Succeed())

			newDestinationAppGUID = "new-destination-app-guid"
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
			req, err = http.NewRequestWithContext(ctx, "POST", serverURI("/v3/routes/"+routeGUID1+"/destinations"), strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Content-type", "application/json")
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
				createRoleBinding(ctx, userName, spaceManagerRole.Name, namespace.Name)
			})

			It("returns a forbidden error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusForbidden))
			})
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace.Name)
			})

			It("updates the route", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPBody(ContainSubstring(newDestinationAppGUID)), rr.Body.String())

				// TODO: add an eventually against K8s client?
			})
		})

	})
})
