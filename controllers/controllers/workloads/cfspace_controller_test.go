package workloads_test

import (
	"context"
	"errors"
	"time"

	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("CFSpace Reconciler", func() {
	var (
		fakeClient       *fake.Client
		fakeStatusWriter *fake.StatusWriter

		cfSpaceGUID string

		cfSpace            *workloadsv1alpha1.CFSpace
		subNamespaceAnchor *v1alpha2.SubnamespaceAnchor
		namespace          *v1.Namespace

		subNamespaceAnchorState v1alpha2.SubnamespaceAnchorState
		cfSpaceError            error
		subNamespaceAnchorError error
		createError             error
		namespaceError          error

		cfSpaceReconciler *CFSpaceReconciler
		ctx               context.Context
		req               ctrl.Request

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		fakeClient = new(fake.Client)

		cfSpaceGUID = PrefixedGUID("cf-space")

		cfSpace = BuildCFSpaceObject(cfSpaceGUID, defaultNamespace)
		subNamespaceAnchor = BuildSubNamespaceAnchorObject(cfSpaceGUID, defaultNamespace)
		namespace = BuildNamespaceObject(cfSpaceGUID)

		subNamespaceAnchorState = v1alpha2.Ok
		cfSpaceError = nil
		subNamespaceAnchorError = nil
		namespaceError = nil
		createError = nil
		reconcileErr = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *workloadsv1alpha1.CFSpace:
				cfSpace.DeepCopyInto(obj)
				return cfSpaceError
			case *v1alpha2.SubnamespaceAnchor:
				subNamespaceAnchor.DeepCopyInto(obj)
				return subNamespaceAnchorError
			case *v1.Namespace:
				namespace.DeepCopyInto(obj)
				return namespaceError
			default:
				panic("TestClient Get provided a weird obj")
			}
		}

		fakeClient.CreateStub = func(ctx context.Context, obj client.Object, option ...client.CreateOption) error {
			switch obj := obj.(type) {
			case *v1alpha2.SubnamespaceAnchor:
				obj.Status.State = subNamespaceAnchorState
				return createError
			default:
				panic("TestClient Create provided an unexpected object type")
			}
		}

		// Configure mock status update to succeed
		fakeStatusWriter = &fake.StatusWriter{}
		fakeClient.StatusReturns(fakeStatusWriter)

		// configure a CFSpaceReconciler with the client
		Expect(workloadsv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfSpaceReconciler = NewCFSpaceReconciler(
			fakeClient,
			scheme.Scheme,
			zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
		)
		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: defaultNamespace,
				Name:      cfSpaceGUID,
			},
		}
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = cfSpaceReconciler.Reconcile(ctx, req)
	})

	When("Create", func() {
		BeforeEach(func() {
			subNamespaceAnchorError = k8serrors.NewNotFound(schema.GroupResource{}, "CFSpace")
		})

		It("returns an empty result and does not return error", func() {
			Expect(reconcileResult).To(Equal(ctrl.Result{RequeueAfter: 100 * time.Millisecond}))
			Expect(reconcileErr).NotTo(HaveOccurred())
		})

		When("a subnamespace anchor exists", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Ok
			})

			It("returns an empty result and does not return error", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).NotTo(HaveOccurred())
			})

			It("does not create the subsnamespace anchor", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(0))
			})
		})

		When("fetch CFSpace returns an error", func() {
			BeforeEach(func() {
				cfSpaceError = errors.New("get CFSpace failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("get CFSpace failed"))
			})
		})

		When("fetch subnamespace anchor returns an error other than not found", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = errors.New("get subnamespace anchor failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("get subnamespace anchor failed"))
			})
		})

		When("create subnamespace anchor returns an error ", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = k8serrors.NewNotFound(schema.GroupResource{}, "CFSpace")
				createError = errors.New("create subnamespace anchor failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("create subnamespace anchor failed"))
			})
		})

		When("update CFSpace status to unknown returns an error", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = k8serrors.NewNotFound(schema.GroupResource{}, "CFSpace")
				fakeStatusWriter.UpdateReturns(errors.New("update CFSpace status failed"))
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("update CFSpace status failed"))
			})
		})

		When("Subnamespace anchor status never goes to ok", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Missing
			})

			It("should requeue", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{RequeueAfter: 100 * time.Millisecond}))
				Expect(reconcileErr).NotTo(HaveOccurred())
			})
		})

		When("fetch namespace returns an error ", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Ok
				namespaceError = errors.New("get namespace failed")
			})

			It("should requeue", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{RequeueAfter: 100 * time.Millisecond}))
				Expect(reconcileErr).NotTo(HaveOccurred())
			})
		})

		When("update CFSpace status to ready returns an error", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Ok
				fakeStatusWriter.UpdateReturns(errors.New("update CFSpace status failed"))
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("update CFSpace status failed"))
			})
		})
	})
})
