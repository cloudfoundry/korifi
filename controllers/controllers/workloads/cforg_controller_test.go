package workloads_test

import (
	"context"
	"errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("CFOrgReconciler", func() {
	var (
		fakeClient       *fake.Client
		fakeStatusWriter *fake.StatusWriter

		cfOrgGUID                 string
		packageRegistrySecretName string

		cfOrg                 *korifiv1alpha1.CFOrg
		namespace             *v1.Namespace
		packageRegistrySecret *v1.Secret

		cfOrgError         error
		cfOrgPatchError    error
		secretError        error
		secretCreateError  error
		secretPatchError   error
		namespaceGetError  error
		namespaceCreateErr error
		namespacePatchErr  error
		namespaceDeleteErr error

		cfOrgReconciler *CFOrgReconciler
		ctx             context.Context
		req             ctrl.Request

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		fakeClient = new(fake.Client)

		cfOrgGUID = PrefixedGUID("cf-org")
		packageRegistrySecretName = "test-package-registry-secret"

		cfOrg = BuildCFOrgObject(cfOrgGUID, defaultNamespace)
		namespace = BuildNamespaceObject(cfOrgGUID)
		packageRegistrySecret = BuildDockerRegistrySecret(packageRegistrySecretName, defaultNamespace)

		cfOrgError = nil
		namespaceGetError = nil
		secretError = nil
		secretCreateError = nil
		secretPatchError = nil
		reconcileErr = nil
		namespaceCreateErr = nil
		namespacePatchErr = nil
		cfOrgPatchError = nil
		namespaceDeleteErr = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFOrg:
				cfOrg.DeepCopyInto(obj)
				return cfOrgError
			case *v1.Namespace:
				namespace.DeepCopyInto(obj)
				return namespaceGetError
			case *v1.Secret:
				packageRegistrySecret.DeepCopyInto(obj)
				return secretError
			default:
				panic("TestClient Get provided a weird obj")
			}
		}

		fakeClient.CreateStub = func(ctx context.Context, obj client.Object, option ...client.CreateOption) error {
			switch obj.(type) {
			case *v1.Namespace:
				return namespaceCreateErr
			case *v1.Secret:
				return secretCreateError
			default:
				panic("TestClient Create provided an unexpected object type")
			}
		}

		fakeClient.PatchStub = func(ctx context.Context, obj client.Object, patch client.Patch, option ...client.PatchOption) error {
			switch obj.(type) {
			case *korifiv1alpha1.CFOrg:
				return cfOrgPatchError
			case *v1.Namespace:
				return namespacePatchErr
			case *v1.Secret:
				return secretPatchError
			default:
				panic("TestClient Patch provided an unexpected object type")
			}
		}

		fakeClient.DeleteStub = func(ctx context.Context, obj client.Object, option ...client.DeleteOption) error {
			switch obj.(type) {
			case *v1.Namespace:
				return namespaceDeleteErr
			default:
				panic("TestClient Delete provided an unexpected object type")
			}
		}

		// Configure mock status update to succeed
		fakeStatusWriter = &fake.StatusWriter{}
		fakeClient.StatusReturns(fakeStatusWriter)

		// configure a CFOrgReconciler with the client
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfOrgReconciler = NewCFOrgReconciler(
			fakeClient,
			scheme.Scheme,
			zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
			packageRegistrySecretName,
		)
		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: defaultNamespace,
				Name:      cfOrgGUID,
			},
		}
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = cfOrgReconciler.Reconcile(ctx, req)
	})

	When("Create", func() {
		BeforeEach(func() {
			namespaceGetError = k8serrors.NewNotFound(schema.GroupResource{}, "CFOrg")
		})

		It("validates the condition on the CFOrg is set to Unknown", func() {
			Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
			_, updatedCFOrg, _ := fakeStatusWriter.UpdateArgsForCall(0)
			castCFOrg, ok := updatedCFOrg.(*korifiv1alpha1.CFOrg)
			Expect(ok).To(BeTrue(), "Cast to korifiv1alpha1.CFOrg failed")
			Expect(meta.IsStatusConditionPresentAndEqual(castCFOrg.Status.Conditions, StatusConditionReady, metav1.ConditionUnknown)).To(BeTrue(), "Status Condition "+StatusConditionReady+" was not True as expected")
			Expect(castCFOrg.Status.GUID).To(Equal(""))
		})

		When("fetch CFOrg returns an error", func() {
			BeforeEach(func() {
				cfOrgError = errors.New("get CFOrg failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("get CFOrg failed"))
			})
		})

		When("update CFOrg status to unknown returns an error", func() {
			BeforeEach(func() {
				fakeStatusWriter.UpdateReturns(errors.New("update CFOrg status failed"))
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("update CFOrg status failed"))
			})
		})

		When("adding finalizer to CFOrg fails", func() {
			BeforeEach(func() {
				cfOrgPatchError = errors.New("adding finalizer failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("adding finalizer failed"))
			})
		})

		When("create namespace returns an error ", func() {
			BeforeEach(func() {
				namespaceCreateErr = errors.New("create namespace failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("create namespace failed"))
			})
		})

		When("fetch secret returns an error ", func() {
			BeforeEach(func() {
				namespaceGetError = nil
				secretError = errors.New("fetch secret failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("fetch secret failed"))
			})
		})

	})

	When("a CFOrg is being deleted", func() {
		BeforeEach(func() {
			cfOrg.ObjectMeta.DeletionTimestamp = &metav1.Time{
				Time: time.Now(),
			}
		})
		It("returns an empty result and does not return error", func() {
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
			Expect(reconcileErr).NotTo(HaveOccurred())
		})

		When("deleting namespace returns an error", func() {
			BeforeEach(func() {
				namespaceDeleteErr = errors.New("delete namespace failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("delete namespace failed"))
			})
		})
	})
})
