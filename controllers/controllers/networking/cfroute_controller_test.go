package networking_test

import (
	"context"
	"errors"
	"time"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/networking"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/networking/fake"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegaTypes "github.com/onsi/gomega/types"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	testNamespace            = "test-ns"
	testDomainGUID           = "test-domain-guid"
	testDomainName           = "test.domain.name"
	testRouteGUID            = "test-route-guid"
	testRouteHost            = "test-route-host"
	testRouteDestinationGUID = "test-route-destination-guid"
	testFQDN                 = testRouteHost + "." + testDomainName
	testServiceGUID          = "s-" + testRouteDestinationGUID
	routeGUIDLabelKey        = "networking.cloudfoundry.org/route-guid"
)

var _ = Describe("CFRouteReconciler.Reconcile", func() {
	var (
		fakeClient       *fake.Client
		fakeStatusWriter *fake.StatusWriter

		cfDomain       *networkingv1alpha1.CFDomain
		cfRoute        *networkingv1alpha1.CFRoute
		httpProxyList  *contourv1.HTTPProxyList
		fqdnHTTPProxy  *contourv1.HTTPProxy
		routeHTTPProxy *contourv1.HTTPProxy
		serviceList    *v1.ServiceList

		getDomainError            error
		getRouteError             error
		listHTTPProxiesError      error
		getHTTPProxyError         error
		createFQDNHTTPProxyError  error
		createRouteHTTPProxyError error
		createServiceError        error
		patchCFRouteError         error
		patchHTTPProxyError       error
		updateCFRouteError        error
		deleteServiceErr          error
		listServicesError         error
		updateCFRouteStatusError  error

		cfRouteReconciler *CFRouteReconciler
		ctx               context.Context
		req               ctrl.Request

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		fakeClient = new(fake.Client)

		cfDomain = &networkingv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name: testDomainGUID,
			},
			Spec: networkingv1alpha1.CFDomainSpec{
				Name: testDomainName,
			},
		}

		cfRoute = &networkingv1alpha1.CFRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testRouteGUID,
				Namespace: testNamespace,
				Finalizers: []string{
					"cfRoute.networking.cloudfoundry.org",
				},
			},
			Spec: networkingv1alpha1.CFRouteSpec{
				Host: testRouteHost,
				DomainRef: v1.LocalObjectReference{
					Name: testDomainGUID,
				},
				Destinations: []networkingv1alpha1.Destination{
					{
						GUID: testRouteDestinationGUID,
						AppRef: v1.LocalObjectReference{
							Name: "test-app-guid",
						},
						ProcessType: "web",
					},
				},
			},
		}

		httpProxyList = &contourv1.HTTPProxyList{}
		fqdnHTTPProxy = &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testFQDN,
				Namespace: testNamespace,
			},
		}
		routeHTTPProxy = &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testRouteGUID,
				Namespace: testNamespace,
			},
		}
		serviceList = &v1.ServiceList{}

		getDomainError = nil
		getRouteError = nil
		listHTTPProxiesError = nil
		getHTTPProxyError = apierrors.NewNotFound(schema.GroupResource{}, testFQDN)
		createFQDNHTTPProxyError = nil
		createRouteHTTPProxyError = nil
		createServiceError = nil
		patchCFRouteError = nil
		patchHTTPProxyError = nil
		updateCFRouteError = nil
		deleteServiceErr = nil
		listServicesError = nil
		updateCFRouteStatusError = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			switch obj.(type) {
			case *networkingv1alpha1.CFDomain:
				cfDomain.DeepCopyInto(obj.(*networkingv1alpha1.CFDomain))
				return getDomainError
			case *networkingv1alpha1.CFRoute:
				cfRoute.DeepCopyInto(obj.(*networkingv1alpha1.CFRoute))
				return getRouteError
			case *contourv1.HTTPProxy:
				if obj.GetName() == testFQDN {
					fqdnHTTPProxy.DeepCopyInto(obj.(*contourv1.HTTPProxy))
				} else {
					routeHTTPProxy.DeepCopyInto(obj.(*contourv1.HTTPProxy))
				}
				return getHTTPProxyError
			case *v1.Service:
				return apierrors.NewNotFound(schema.GroupResource{}, testRouteDestinationGUID)
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			switch list.(type) {
			case *contourv1.HTTPProxyList:
				httpProxyList.DeepCopyInto(list.(*contourv1.HTTPProxyList))
				return listHTTPProxiesError
			case *v1.ServiceList:
				serviceList.DeepCopyInto(list.(*v1.ServiceList))
				return listServicesError
			default:
				panic("TestClient List provided an unexpected object type")
			}
		}

		fakeClient.CreateStub = func(ctx context.Context, obj client.Object, option ...client.CreateOption) error {
			switch obj.(type) {
			case *contourv1.HTTPProxy:
				if obj.GetName() == testFQDN {
					return createFQDNHTTPProxyError
				} else {
					return createRouteHTTPProxyError
				}
			case *v1.Service:
				return createServiceError
			default:
				panic("TestClient Create provided an unexpected object type")
			}
		}

		fakeClient.PatchStub = func(ctx context.Context, obj client.Object, patch client.Patch, option ...client.PatchOption) error {
			switch obj.(type) {
			case *networkingv1alpha1.CFRoute:
				return patchCFRouteError
			case *contourv1.HTTPProxy:
				return patchHTTPProxyError
			default:
				panic("TestClient Patch provided an unexpected object type")
			}
		}

		fakeClient.UpdateStub = func(ctx context.Context, obj client.Object, option ...client.UpdateOption) error {
			switch obj.(type) {
			case *networkingv1alpha1.CFRoute:
				return updateCFRouteError
			default:
				panic("TestClient Update provided an unexpected object type")
			}
		}

		fakeClient.DeleteStub = func(ctx context.Context, obj client.Object, option ...client.DeleteOption) error {
			switch obj.(type) {
			case *v1.Service:
				return deleteServiceErr
			default:
				panic("TestClient Delete provided an unexpected object type")
			}
		}

		fakeStatusWriter = &fake.StatusWriter{}
		fakeClient.StatusReturns(fakeStatusWriter)

		fakeStatusWriter.UpdateStub = func(ctx context.Context, obj client.Object, option ...client.UpdateOption) error {
			return updateCFRouteStatusError
		}

		Expect(networkingv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfRouteReconciler = &CFRouteReconciler{
			Client: fakeClient,
			Scheme: scheme.Scheme,
			Log:    zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
		}

		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      testRouteGUID,
				Namespace: testNamespace,
			},
		}
	})

	When("the CFRoute is being created", func() {
		When("on the happy path", func() {
			BeforeEach(func() {
				reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
			})

			It("returns an empty result and does not return error, also updates cfRoute status", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).NotTo(HaveOccurred())

				Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
				_, routeObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
				updatedCFRoute, ok := routeObj.(*networkingv1alpha1.CFRoute)
				Expect(ok).To(BeTrue())
				expectCFRouteValidStatus(updatedCFRoute.Status, true, "Valid CFRoute", "Valid", "Valid CFRoute")
			})

			It("creates an FQDN HTTPProxy, a route HTTPProxy, and a Service", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(3), "Client.Create call count mismatch")

				createdObjectMatches := func(matcher gomegaTypes.GomegaMatcher) gomegaTypes.GomegaMatcher {
					return MatchElementsWithIndex(IndexIdentity, IgnoreExtras, Elements{
						"1": matcher,
					})
				}

				Expect(fakeClient.Invocations()["Create"]).To(ConsistOf(
					createdObjectMatches(BeAssignableToTypeOf(new(contourv1.HTTPProxy))),
					createdObjectMatches(BeAssignableToTypeOf(new(contourv1.HTTPProxy))),
					createdObjectMatches(BeAssignableToTypeOf(new(v1.Service))),
				))
			})
		})

		When("on the unhappy path", func() {
			When("fetch CFRoute returns an error", func() {
				BeforeEach(func() {
					getRouteError = errors.New("failed to get CFRoute")
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError("failed to get CFRoute"))
				})
			})

			When("fetch CFRoute returns a NotFoundError", func() {
				BeforeEach(func() {
					getRouteError = apierrors.NewNotFound(schema.GroupResource{}, testRouteGUID)
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("should NOT return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("fetch CFDomain returns an error", func() {
				BeforeEach(func() {
					getDomainError = errors.New("failed to get CFDomain")
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns an error and updates the CurrentStatus and Status Conditions", func() {
					Expect(reconcileErr).To(HaveOccurred())
					Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
					_, routeObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
					updatedCFRoute, ok := routeObj.(*networkingv1alpha1.CFRoute)
					Expect(ok).To(BeTrue())
					expectCFRouteValidStatus(updatedCFRoute.Status, false)
				})
			})

			When("fetch CFDomain returns a NotFoundError", func() {
				BeforeEach(func() {
					getDomainError = apierrors.NewNotFound(schema.GroupResource{}, testDomainGUID)

					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns an error and updates the CurrentStatus and Status Conditions", func() {
					Expect(reconcileErr).To(HaveOccurred())
					Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
					_, routeObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
					updatedCFRoute, ok := routeObj.(*networkingv1alpha1.CFRoute)
					Expect(ok).To(BeTrue())
					expectCFRouteValidStatus(updatedCFRoute.Status, false)
				})
			})

			When("list HTTPProxies returns an error", func() {
				BeforeEach(func() {
					listHTTPProxiesError = errors.New("failed to list HTTPProxies")
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError("failed to list HTTPProxies"))
				})
			})

			When("list HTTPProxies returns multiple proxies with the same FQDN", func() {
				BeforeEach(func() {
					httpProxyList = &contourv1.HTTPProxyList{
						Items: []contourv1.HTTPProxy{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "proxy-1",
									Namespace: testNamespace,
								},
								Spec: contourv1.HTTPProxySpec{
									VirtualHost: &contourv1.VirtualHost{
										Fqdn: testFQDN,
									},
								},
							},
							{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "proxy-2",
									Namespace: testNamespace,
								},
								Spec: contourv1.HTTPProxySpec{
									VirtualHost: &contourv1.VirtualHost{
										Fqdn: testFQDN,
									},
								},
							},
						},
					}
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError("found multiple HTTPProxy with FQDN " + testFQDN))
				})
			})

			When("list HTTPProxies a proxy with the same FQDN in a different namespace", func() {
				BeforeEach(func() {
					httpProxyList = &contourv1.HTTPProxyList{
						Items: []contourv1.HTTPProxy{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "proxy-1",
									Namespace: "other-ns",
								},
								Spec: contourv1.HTTPProxySpec{
									VirtualHost: &contourv1.VirtualHost{
										Fqdn: testFQDN,
									},
								},
							},
						},
					}
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError("found existing HTTPProxy with FQDN " + testFQDN + " in another space"))
				})
			})

			When("adding the finalizer to the CFRoute returns an error", func() {
				BeforeEach(func() {
					cfRoute.ObjectMeta.Finalizers = []string{}
					patchCFRouteError = errors.New("failed to patch CFRoute")
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns the error and sets an \"invalid\" status on the cfRoute", func() {
					Expect(reconcileErr).To(MatchError("failed to patch CFRoute"))

					Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
					_, routeObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
					updatedCFRoute, ok := routeObj.(*networkingv1alpha1.CFRoute)
					Expect(ok).To(BeTrue())
					expectCFRouteValidStatus(updatedCFRoute.Status, false)
				})
			})

			When("setting the CFRoute status fields returns an error", func() {
				BeforeEach(func() {
					updateCFRouteStatusError = errors.New("failed to update CFRoute status")
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns the error", func() {
					Expect(reconcileErr).To(MatchError("failed to update CFRoute status"))
				})
			})

			When("creating the FQDN HTTPProxy returns an error", func() {
				BeforeEach(func() {
					createFQDNHTTPProxyError = errors.New("failed to create FQDN HTTPProxy")
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns the error and sets an \"invalid\" status on the cfRoute", func() {
					Expect(reconcileErr).To(MatchError("failed to create FQDN HTTPProxy"))

					Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
					_, routeObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
					updatedCFRoute, ok := routeObj.(*networkingv1alpha1.CFRoute)
					Expect(ok).To(BeTrue())
					expectCFRouteValidStatus(updatedCFRoute.Status, false)
				})
			})

			When("creating the Route HTTPProxy returns an error", func() {
				BeforeEach(func() {
					createRouteHTTPProxyError = errors.New("failed to create route HTTPProxy")
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns the error and sets an \"invalid\" status on the cfRoute", func() {
					Expect(reconcileErr).To(MatchError("failed to create route HTTPProxy"))

					Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
					_, routeObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
					updatedCFRoute, ok := routeObj.(*networkingv1alpha1.CFRoute)
					Expect(ok).To(BeTrue())
					expectCFRouteValidStatus(updatedCFRoute.Status, false)
				})
			})

			When("creating the Service returns an error", func() {
				BeforeEach(func() {
					createServiceError = errors.New("failed to create service")
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns the error and sets an \"invalid\" status on the cfRoute", func() {
					Expect(reconcileErr).To(MatchError("service reconciliation failed for CFRoute/" + testRouteGUID + " destinations"))

					Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
					_, routeObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
					updatedCFRoute, ok := routeObj.(*networkingv1alpha1.CFRoute)
					Expect(ok).To(BeTrue())
					expectCFRouteValidStatus(updatedCFRoute.Status, false)
				})
			})
		})
	})

	When("the CFRoute is being deleted", func() {
		BeforeEach(func() {
			cfRoute.ObjectMeta.DeletionTimestamp = &metav1.Time{
				Time: time.Now(),
			}

			httpProxyList = &contourv1.HTTPProxyList{
				Items: []contourv1.HTTPProxy{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testFQDN,
							Namespace: testNamespace,
						},
						Spec: contourv1.HTTPProxySpec{
							VirtualHost: &contourv1.VirtualHost{
								Fqdn: testFQDN,
							},
							Includes: []contourv1.Include{
								{
									Name:      testRouteGUID,
									Namespace: testNamespace,
								},
							},
						},
					},
				},
			}

			getHTTPProxyError = nil

			fqdnHTTPProxy = &contourv1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testFQDN,
					Namespace: testNamespace,
				},
				Spec: contourv1.HTTPProxySpec{
					VirtualHost: &contourv1.VirtualHost{
						Fqdn: testFQDN,
					},
					Includes: []contourv1.Include{
						{
							Name:      testRouteGUID,
							Namespace: testNamespace,
						},
					},
				},
			}
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
			})

			It("returns an empty result and does not return error", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).NotTo(HaveOccurred())
			})

			It("updates the FQDN HTTPProxy to remove the route HTTPProxy from the includes list", func() {
				Expect(fakeClient.PatchCallCount()).To(Equal(1), "Client.Patch call count mismatch")

				_, requestObject, _, _ := fakeClient.PatchArgsForCall(0)
				requestHTTPProxy, ok := requestObject.(*contourv1.HTTPProxy)
				Expect(ok).To(BeTrue(), "Cast to contourv1.HTTPProxy failed")
				Expect(requestHTTPProxy.Name).To(Equal(testFQDN))
			})

			It("removes the finalizer from the CFRoute", func() {
				Expect(fakeClient.UpdateCallCount()).To(Equal(1), "Client.Update call count mismatch")

				_, requestObject, _ := fakeClient.UpdateArgsForCall(0)
				requestRoute, ok := requestObject.(*networkingv1alpha1.CFRoute)
				Expect(ok).To(BeTrue(), "Cast to networkingv1alpha1.CFRoute failed")
				Expect(requestRoute.ObjectMeta.Finalizers).To(HaveLen(0), "CFRoute finalizer count mismatch")
			})

			It("does not attempt to create any resources", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(0), "Client.Create call count mismatch")
			})
		})

		When("on the unhappy path", func() {
			When("the FQDN HTTPProxy was never created", func() {
				BeforeEach(func() {
					httpProxyList = &contourv1.HTTPProxyList{}
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns an empty result and does not return error", func() {
					Expect(reconcileResult).To(Equal(ctrl.Result{}))
					Expect(reconcileErr).NotTo(HaveOccurred())
				})

				It("removes the finalizer from the CFRoute", func() {
					Expect(fakeClient.UpdateCallCount()).To(Equal(1), "Client.Update call count mismatch")

					_, requestObject, _ := fakeClient.UpdateArgsForCall(0)
					requestRoute, ok := requestObject.(*networkingv1alpha1.CFRoute)
					Expect(ok).To(BeTrue(), "Cast to networkingv1alpha1.CFRoute failed")
					Expect(requestRoute.ObjectMeta.Finalizers).To(HaveLen(0), "CFRoute finalizer count mismatch")
				})

				It("does not attempt to create any resources", func() {
					Expect(fakeClient.CreateCallCount()).To(Equal(0), "Client.Create call count mismatch")
				})
			})

			When("patching the FQDN HTTPProxy to remove the include fails", func() {
				BeforeEach(func() {
					patchHTTPProxyError = errors.New("failed to patch FQDN HTTPProxy")
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns the error", func() {
					Expect(reconcileErr).To(MatchError("failed to patch FQDN HTTPProxy"))
				})
			})

			When("removing the finalizer from the CFRoute fails", func() {
				BeforeEach(func() {
					updateCFRouteError = errors.New("failed to update CFRoute")
					_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns the error", func() {
					Expect(reconcileErr).To(MatchError("failed to update CFRoute"))
				})
			})
		})
	})

	When("the CFRoute is being updated", func() {
		BeforeEach(func() {
			httpProxyList = &contourv1.HTTPProxyList{
				Items: []contourv1.HTTPProxy{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testFQDN,
							Namespace: testNamespace,
						},
						Spec: contourv1.HTTPProxySpec{
							VirtualHost: &contourv1.VirtualHost{
								Fqdn: testFQDN,
							},
							Includes: []contourv1.Include{
								{
									Name:      testRouteGUID,
									Namespace: testNamespace,
								},
							},
						},
					},
				},
			}

			getHTTPProxyError = nil

			fqdnHTTPProxy = &contourv1.HTTPProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testFQDN,
					Namespace: testNamespace,
				},
				Spec: contourv1.HTTPProxySpec{
					VirtualHost: &contourv1.VirtualHost{
						Fqdn: testFQDN,
					},
					Includes: []contourv1.Include{
						{
							Name:      testRouteGUID,
							Namespace: testNamespace,
						},
					},
				},
			}

			serviceList = &v1.ServiceList{
				Items: []v1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testServiceGUID,
							Namespace: testNamespace,
							Labels: map[string]string{
								routeGUIDLabelKey: testRouteGUID,
							},
						},
						Spec: v1.ServiceSpec{},
					},
				},
			}
		})

		When("a destination on CFRoute is removed", func() {
			BeforeEach(func() {
				cfRoute.Spec.Destinations = []networkingv1alpha1.Destination{}
			})

			When("on the happy path", func() {
				BeforeEach(func() {
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("returns an empty result and does not return error", func() {
					Expect(reconcileResult).To(Equal(ctrl.Result{}))
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("on the unhappy path", func() {
				When("listing Services fails", func() {
					BeforeEach(func() {
						listServicesError = errors.New("failed to list Services")
						_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
					})

					It("returns an error", func() {
						Expect(reconcileErr).To(MatchError("failed to list Services"))
					})

					It("doesn't delete any services", func() {
						Expect(fakeClient.DeleteCallCount()).To(Equal(0))
					})
				})

				When("deleting a service fails", func() {
					BeforeEach(func() {
						deleteServiceErr = errors.New("failed to delete Service")
						_, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
					})

					It("returns an error and sets an \"invalid\" status on the cfRoute", func() {
						Expect(reconcileErr).To(MatchError("failed to delete Service"))
					})
				})
			})
		})
	})
})
