package networking_test

import (
	"context"
	"errors"
	"testing"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/networking"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/networking/fake"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/networking/testutils"

	. "github.com/onsi/gomega"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestRouteReconciler(t *testing.T) {
	spec.Run(t, "CFRouteReconciler", testCFRouteReconciler, spec.Report(report.Terminal{}))

}

func testCFRouteReconciler(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

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
	it.Before(func() {
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
		g.Expect(networkingv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		g.Expect(contourv1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfRouteReconciler = &CFRouteReconciler{
			Client: fakeClient,
			Scheme: scheme.Scheme,
			Log:    zap.New(zap.WriteTo(it.Out()), zap.UseDevMode(true)),
		}

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: defaultNamespace,
				Name:      cfRouteGUID,
			},
		}

		ctx = context.Background()
	})

	when("CFRoute status conditions are unknown and", func() {
		when("on the happy path", func() {
			it.Before(func() {
				reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
			})

			it("does not return an error", func() {
				g.Expect(reconcileErr).NotTo(HaveOccurred())
			})

			it("returns an empty result", func() {
				g.Expect(reconcileResult).To(Equal(ctrl.Result{}))
			})

			it("should check that the CFRoute, CFDomain and HTTPProxy resources exist", func() {
				g.Expect(fakeClient.GetCallCount()).To(Equal(3), "fakeClient Get was not called three times")

				// First request checks for the existence of the CFRoute
				_, namespacedName, _ := fakeClient.GetArgsForCall(0)
				g.Expect(namespacedName.Name).To(Equal(cfRouteGUID))

				// Second request checks for the existence of the CFDomain
				_, namespacedName, _ = fakeClient.GetArgsForCall(1)
				g.Expect(namespacedName.Name).To(Equal(cfDomainGUID))

				// Third request checks for the existence of the HTTPProxy
				_, namespacedName, _ = fakeClient.GetArgsForCall(2)
				g.Expect(namespacedName.Name).To(Equal(cfRouteGUID))
			})

			it("should create an HTTPProxy with the same GUID as the CF Route", func() {
				g.Expect(fakeClient.CreateCallCount()).To(Equal(1), "fakeClient Create was not called once")
				_, contourHTTPProxyObj, _ := fakeClient.CreateArgsForCall(0)
				contourHTTPProxy := contourHTTPProxyObj.(*contourv1.HTTPProxy)
				g.Expect(contourHTTPProxy.Name).To(Equal(cfRouteGUID))
				g.Expect(contourHTTPProxy.Spec.VirtualHost.Fqdn).To(Equal(cfRouteFQDN))
			})

			it("should set the ProxyCreated status to true", func() {
				g.Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1), "fakeStatusWriter Update was not called once")
				_, cfRouteObj, _ := fakeStatusWriter.UpdateArgsForCall(0)
				cfRoute := cfRouteObj.(*networkingv1alpha1.CFRoute)
				g.Expect(cfRoute.Status.Conditions).NotTo(BeEmpty())
			})
		})

		when("on unhappy path", func() {
			when("fetch CFRoute returns a NotFoundError", func() {
				it.Before(func() {
					getCFRouteError = apierrors.NewNotFound(schema.GroupResource{}, getCFRoute.Name)
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				it("should NOT return any error", func() {
					g.Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			when("fetch CFRoute returns an unknown error", func() {
				it.Before(func() {
					getCFRouteError = errors.New("failed to get route")
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				it("should return an error", func() {
					g.Expect(reconcileErr).To(MatchError("failed to get route"))
				})
			})

			when("fetch CFDomain returns an error", func() {
				it.Before(func() {
					getCFDomainError = errors.New("failed to get domain")
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				it("should return an error", func() {
					g.Expect(reconcileErr).To(MatchError("failed to get domain"))
				})
			})

			when("fetch HTTPProxy returns an error", func() {
				it.Before(func() {
					getContourHTTPProxyError = errors.New("failed to get httpproxy")
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				it("should return an error", func() {
					g.Expect(reconcileErr).To(MatchError("failed to get httpproxy"))
				})
			})

			when("create Contour HTTPProxy returns an unknown error", func() {
				it.Before(func() {
					fakeClient.CreateReturns(errors.New("failed to create httpproxy"))
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				it("should return an error", func() {
					g.Expect(reconcileErr).To(MatchError("failed to create httpproxy"))
				})
			})

			when("update status conditions returns an error", func() {
				it.Before(func() {
					fakeStatusWriter.UpdateReturns(errors.New("failed to write status conditions"))
					reconcileResult, reconcileErr = cfRouteReconciler.Reconcile(ctx, req)
				})

				it("should return an error", func() {
					g.Expect(reconcileErr).To(MatchError("failed to write status conditions"))
				})
			})
		})
	})
}
