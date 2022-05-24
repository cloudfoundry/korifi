package workloads_test

import (
	"context"
	"errors"
	"time"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/fake"

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
	const (
		packageRegistrySecretName = "test-package-registry-secret"
	)
	var (
		fakeClient       *fake.Client
		fakeStatusWriter *fake.StatusWriter

		cfSpaceGUID string

		cfSpace            *v1alpha1.CFSpace
		subNamespaceAnchor *v1alpha2.SubnamespaceAnchor
		namespace          *v1.Namespace

		subNamespaceAnchorState             v1alpha2.SubnamespaceAnchorState
		cfSpaceError                        error
		subNamespaceAnchorError             error
		createSubnamespaceAnchorError       error
		namespaceError                      error
		getEiriniServiceAccountError        error
		createEiriniServiceAccountError     error
		createEiriniServiceAccountCallCount int
		getKpackServiceAccountError         error
		createKpackServiceAccountError      error
		createKpackServiceAccountCallCount  int

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
		createSubnamespaceAnchorError = nil
		reconcileErr = nil

		getEiriniServiceAccountError = nil
		createEiriniServiceAccountError = nil
		createEiriniServiceAccountCallCount = 0
		getKpackServiceAccountError = nil
		createKpackServiceAccountError = nil
		createKpackServiceAccountCallCount = 0

		fakeClient.GetStub = func(_ context.Context, nn types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *v1alpha1.CFSpace:
				cfSpace.DeepCopyInto(obj)
				return cfSpaceError
			case *v1alpha2.SubnamespaceAnchor:
				subNamespaceAnchor.DeepCopyInto(obj)
				return subNamespaceAnchorError
			case *v1.Namespace:
				namespace.DeepCopyInto(obj)
				return namespaceError
			case *v1.ServiceAccount:
				if nn.Name == "eirini" {
					return getEiriniServiceAccountError
				} else {
					return getKpackServiceAccountError
				}
			default:
				panic("TestClient Get provided a weird obj")
			}
		}

		fakeClient.CreateStub = func(ctx context.Context, obj client.Object, option ...client.CreateOption) error {
			switch obj := obj.(type) {
			case *v1alpha2.SubnamespaceAnchor:
				obj.Status.State = subNamespaceAnchorState
				return createSubnamespaceAnchorError
			case *v1.ServiceAccount:
				if obj.Name == "eirini" {
					createEiriniServiceAccountCallCount++
					return createEiriniServiceAccountError
				} else {
					createKpackServiceAccountCallCount++
					return createKpackServiceAccountError
				}
			default:
				panic("TestClient Create provided an unexpected object type")
			}
		}

		// Configure mock status update to succeed
		fakeStatusWriter = &fake.StatusWriter{}
		fakeClient.StatusReturns(fakeStatusWriter)

		// configure a CFSpaceReconciler with the client
		Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfSpaceReconciler = NewCFSpaceReconciler(
			fakeClient,
			scheme.Scheme,
			zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
			packageRegistrySecretName,
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

		When("fetching the CFSpace errors", func() {
			BeforeEach(func() {
				cfSpaceError = errors.New("get CFSpace failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("get CFSpace failed"))
			})
		})

		When("fetching the subnamespace anchor returns an error other than not found", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = errors.New("get subnamespace anchor failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("get subnamespace anchor failed"))
			})
		})

		When("creating the subnamespace anchor errors", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = k8serrors.NewNotFound(schema.GroupResource{}, "CFSpace")
				createSubnamespaceAnchorError = errors.New("create subnamespace anchor failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("create subnamespace anchor failed"))
			})
		})

		When("updating the CFSpace status to 'Unknown' errors", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = k8serrors.NewNotFound(schema.GroupResource{}, "CFSpace")
				fakeStatusWriter.UpdateReturns(errors.New("update CFSpace status failed"))
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("update CFSpace status failed"))
			})
		})

		When("the subnamespace anchor status never becomes Ok", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Missing
			})

			It("should requeue", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{RequeueAfter: 100 * time.Millisecond}))
				Expect(reconcileErr).NotTo(HaveOccurred())
			})
		})

		When("fetching the namespace errors", func() {
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

		When("updating the CFSpace status to 'Ready' errors", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Ok
				fakeStatusWriter.UpdateReturns(errors.New("update CFSpace status failed"))
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("update CFSpace status failed"))
			})
		})

		When("creating the kpack service account errors", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Ok
				getKpackServiceAccountError = errors.New("not found")
				createKpackServiceAccountError = errors.New("boom")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("boom"))
			})
		})

		When("creating the eirini service account errors", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Ok
				getEiriniServiceAccountError = errors.New("not found")
				createEiriniServiceAccountError = errors.New("boom")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("boom"))
			})
		})

		When("the kpack service account already exists", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Ok
			})

			It("should doesn't try to create it", func() {
				Expect(createKpackServiceAccountCallCount).To(Equal(0))
			})
		})

		When("the eirini service account already exists", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Ok
			})

			It("should doesn't try to create it", func() {
				Expect(createEiriniServiceAccountCallCount).To(Equal(0))
			})
		})
	})
})
