package networking_test

import (
	"context"
	"errors"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/networking"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/networking/fake"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/networking/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("CFRouteReconciler Unit Tests", func() {
	const (
		defaultNamespace          = "default"
		proxyCreatedConditionType = "ProxyCreated"
	)

	var (
		fakeClient       *fake.CFClient
		fakeStatusWriter *fake.StatusWriter

		cfDomainGUID string
		cfDomainName string
		cfRouteHost  string
		cfRouteGUID  string
		cfRouteFQDN  string

		getCFDomain              *networkingv1alpha1.CFDomain
		getCFDomainError         error
		getCFRoute               *networkingv1alpha1.CFRoute
		getCFRouteError          error
		getContourHTTPProxyError error

		cfRouteReconciler *CFRouteReconciler
		req               ctrl.Request
		ctx               context.Context

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	// Set up happy path
	BeforeEach(func() {
		fakeClient = new(fake.CFClient)

		cfRouteHost = "apps"
		cfDomainName = "example.org"
		cfRouteGUID = "cf-route-guid"
		cfRouteFQDN = "apps.example.org"

		getCFDomain = InitializeCFDomain(cfDomainGUID, cfDomainName)
		getCFDomainError = nil
		getCFRoute = InitializeCFRoute(cfRouteGUID, defaultNamespace, cfDomainGUID, cfRouteHost)
		getCFRouteError = nil
		getContourHTTPProxyError = apierrors.NewNotFound(schema.GroupResource{}, getCFRoute.Name)

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			// cast obj to find its kind
			switch typedObj := obj.(type) {
			case *networkingv1alpha1.CFRoute:
				getCFRoute.DeepCopyInto(typedObj)
				return getCFRouteError
			case *networkingv1alpha1.CFDomain:
				getCFDomain.DeepCopyInto(typedObj)
				return getCFDomainError
			case *contourv1.HTTPProxy:
				return getContourHTTPProxyError
			default:
				panic("test Client Get provided an unexpected obj")
			}
		}

		// Configure mock status update to succeed
		fakeStatusWriter = &fake.StatusWriter{}
		fakeClient.StatusReturns(fakeStatusWriter)

		// Configure a CFRouteReconciler with the client
		Expect(networkingv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(contourv1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfRouteReconciler = &CFRouteReconciler{
			Client: fakeClient,
			Scheme: scheme.Scheme,
			Log:    zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
		}

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: defaultNamespace,
				Name:      cfRouteGUID,
			},
		}

		ctx = context.Background()
	})

	When("CFRoute status conditions are unknown and", func() {
		When("on the happy path", func() {
			BeforeEach(func() {
				reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
			})

			It("does not return an error", func() {
				Expect(reconcileErr).NotTo(HaveOccurred())
			})

			It("returns an empty result", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
			})

			It("should check that the CFRoute, CFDomain and HTTPProxy resources exist", func() {
				Expect(fakeClient.GetCallCount()).To(Equal(3), "fakeClient Get was not called three times")

				// First request checks for the existence of the CFRoute
				_, namespacedName, _ := fakeClient.GetArgsForCall(0)
				Expect(namespacedName.Name).To(Equal(cfRouteGUID))

				// Second request checks for the existence of the CFDomain
				_, namespacedName, _ = fakeClient.GetArgsForCall(1)
				Expect(namespacedName.Name).To(Equal(cfDomainGUID))

				// Third request checks for the existence of the HTTPProxy
				_, namespacedName, _ = fakeClient.GetArgsForCall(2)
				Expect(namespacedName.Name).To(Equal(cfRouteGUID))
			})

			It("should create an HTTPProxy with the same GUID as the CF Route", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(1), "fakeClient Create was not called once")
				_, contourHTTPProxyObj, _ := fakeClient.CreateArgsForCall(0)
				contourHTTPProxy := contourHTTPProxyObj.(*contourv1.HTTPProxy)
				Expect(contourHTTPProxy.Name).To(Equal(cfRouteGUID))
				Expect(contourHTTPProxy.Spec.VirtualHost.Fqdn).To(Equal(cfRouteFQDN))
			})

			It("should set the ProxyCreated status to true", func() {
				Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1), "fakeStatusWriter Update was not called once")
				_, cfRouteObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
				cfRoute := cfRouteObj.(*networkingv1alpha1.CFRoute)
				Expect(cfRoute.Status.Conditions).NotTo(BeEmpty())
			})
		})

		When("on unhappy path", func() {
			When("fetch CFRoute returns a NotFoundError", func() {
				BeforeEach(func() {
					getCFRouteError = apierrors.NewNotFound(schema.GroupResource{}, getCFRoute.Name)
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("should NOT return any error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("fetch CFRoute returns an unknown error", func() {
				BeforeEach(func() {
					getCFRouteError = errors.New("failed to get route")
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError("failed to get route"))
				})
			})

			When("fetch CFDomain returns an error", func() {
				BeforeEach(func() {
					getCFDomainError = errors.New("failed to get domain")
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError("failed to get domain"))
				})
			})

			When("fetch HTTPProxy returns an error", func() {
				BeforeEach(func() {
					getContourHTTPProxyError = errors.New("failed to get httpproxy")
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError("failed to get httpproxy"))
				})
			})

			When("create Contour HTTPProxy returns an unknown error", func() {
				BeforeEach(func() {
					fakeClient.CreateReturns(errors.New("failed to create httpproxy"))
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError("failed to create httpproxy"))
				})
			})

			When("update status conditions returns an error", func() {
				BeforeEach(func() {
					fakeStatusWriter.UpdateReturns(errors.New("failed to write status conditions"))
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(MatchError("failed to write status conditions"))
				})
			})
		})
	})
})
