package workloads_test

import (
	"context"
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("CFSpace Reconciler", func() {
	const (
		packageRegistrySecretName = "test-package-registry-secret"
		rootNamespace             = "root-namespace"
	)
	var (
		fakeClient       *fake.Client
		fakeStatusWriter *fake.StatusWriter

		cfSpaceGUID string

		cfSpace            *korifiv1alpha1.CFSpace
		namespace          *corev1.Namespace
		serviceAccountList *corev1.ServiceAccountList

		cfSpaceError                       error
		cfSpacePatchError                  error
		createSubnamespaceAnchorCallCount  int
		namespaceError                     error
		createNamespaceErr                 error
		patchNamespaceErr                  error
		deleteNamespaceErr                 error
		secretErr                          error
		getKpackServiceAccountError        error
		createKpackServiceAccountError     error
		createKpackServiceAccountCallCount int

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
		namespace = BuildNamespaceObject(cfSpaceGUID)
		serviceAccountList = &corev1.ServiceAccountList{}

		cfSpaceError = nil
		cfSpacePatchError = nil
		namespaceError = nil
		createNamespaceErr = nil
		patchNamespaceErr = nil
		deleteNamespaceErr = nil
		secretErr = nil
		createSubnamespaceAnchorCallCount = 0
		reconcileErr = nil

		createKpackServiceAccountError = nil
		createKpackServiceAccountCallCount = 0

		fakeClient.GetStub = func(_ context.Context, nn types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFSpace:
				cfSpace.DeepCopyInto(obj)
				return cfSpaceError
			case *corev1.Namespace:
				namespace.DeepCopyInto(obj)
				return namespaceError
			case *corev1.Secret:
				return secretErr
			case *corev1.ServiceAccount:
				return getKpackServiceAccountError
			default:
				panic("TestClient Get provided a weird obj")
			}
		}

		fakeClient.CreateStub = func(ctx context.Context, obj client.Object, option ...client.CreateOption) error {
			switch obj.(type) {
			case *corev1.Namespace:
				createSubnamespaceAnchorCallCount++
				return createNamespaceErr
			case *corev1.ServiceAccount:
				createKpackServiceAccountCallCount++
				return createKpackServiceAccountError
			default:
				panic("TestClient Create provided an unexpected object type")
			}
		}

		fakeClient.PatchStub = func(ctx context.Context, obj client.Object, patch client.Patch, option ...client.PatchOption) error {
			switch obj.(type) {
			case *korifiv1alpha1.CFSpace:
				return cfSpacePatchError
			case *corev1.Namespace:
				return patchNamespaceErr
			default:
				panic("TestClient Patch provided an unexpected object type")
			}
		}

		fakeClient.DeleteStub = func(ctx context.Context, obj client.Object, option ...client.DeleteOption) error {
			switch obj.(type) {
			case *corev1.Namespace:
				return deleteNamespaceErr
			default:
				panic("TestClient Delete provided an unexpected object type")
			}
		}

		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, _ ...client.ListOption) error {
			switch list := list.(type) {
			case *corev1.ServiceAccountList:
				serviceAccountList.DeepCopyInto(list)
				return nil
			case *rbacv1.RoleBindingList:
				return nil
			default:
				panic("TestClient List provided an unexpected object type")
			}
		}

		// Configure mock status update to succeed
		fakeStatusWriter = &fake.StatusWriter{}
		fakeClient.StatusReturns(fakeStatusWriter)

		// configure a CFSpaceReconciler with the client
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfSpaceReconciler = NewCFSpaceReconciler(
			fakeClient,
			scheme.Scheme,
			zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
			packageRegistrySecretName,
			rootNamespace,
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
			namespaceError = k8serrors.NewNotFound(schema.GroupResource{}, "CFSpace")
		})

		It("validates the condition on the CFSpace is set to Unknown", func() {
			Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
			_, updatedCFSpace, _ := fakeStatusWriter.UpdateArgsForCall(0)
			castCFSpace, ok := updatedCFSpace.(*korifiv1alpha1.CFSpace)
			Expect(ok).To(BeTrue(), "Cast to v1alpha1.CFOrg failed")
			Expect(meta.IsStatusConditionPresentAndEqual(castCFSpace.Status.Conditions, StatusConditionReady, metav1.ConditionUnknown)).To(BeTrue(), "Status Condition "+StatusConditionReady+" was not True as expected")
			Expect(castCFSpace.Status.GUID).To(Equal(""))
		})

		When("fetching the CFSpace errors", func() {
			BeforeEach(func() {
				cfSpaceError = errors.New("get CFSpace failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("get CFSpace failed"))
			})
		})

		When("update CFSpace status to unknown returns an error", func() {
			BeforeEach(func() {
				fakeStatusWriter.UpdateReturns(errors.New("update CFSpace status failed"))
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("update CFSpace status failed"))
			})
		})

		When("adding finalizer to CFSpace fails", func() {
			BeforeEach(func() {
				cfSpacePatchError = errors.New("adding finalizer failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("adding finalizer failed"))
			})
		})

		When("creating the namespace returns error", func() {
			BeforeEach(func() {
				createNamespaceErr = errors.New("create namespace failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("create namespace failed"))
			})
		})

		When("fetch secret returns an error ", func() {
			BeforeEach(func() {
				namespaceError = nil
				secretErr = errors.New("fetch secret failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("fetch secret failed"))
			})
		})

		When("creating the kpack service account errors", func() {
			BeforeEach(func() {
				namespaceError = nil
				getKpackServiceAccountError = k8serrors.NewNotFound(schema.GroupResource{}, "serviceaccount")
				createKpackServiceAccountError = errors.New("boom")
				serviceAccountList = &corev1.ServiceAccountList{
					Items: []corev1.ServiceAccount{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "kpack-service-account",
								Namespace: rootNamespace,
								Annotations: map[string]string{
									korifiv1alpha1.PropagateServiceAccountAnnotation: "true",
								},
							},
						},
					},
				}
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("boom"))
			})
		})

		When("the kpack service account already exists", func() {
			BeforeEach(func() {
				namespaceError = nil
			})

			It("should not fail", func() {
				Expect(reconcileErr).To(Not(HaveOccurred()))
			})
		})

		It("returns an empty result and does not return error", func() {
			Expect(reconcileResult).To(Equal(ctrl.Result{RequeueAfter: 100 * time.Millisecond}))
			Expect(reconcileErr).NotTo(HaveOccurred())
		})
	})

	When("a CFSpace is being deleted", func() {
		BeforeEach(func() {
			cfSpace.ObjectMeta.DeletionTimestamp = &metav1.Time{
				Time: time.Now(),
			}
		})
		It("returns an empty result and does not return error", func() {
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
			Expect(reconcileErr).NotTo(HaveOccurred())
		})

		When("deleting namespace returns an error", func() {
			BeforeEach(func() {
				deleteNamespaceErr = errors.New("delete namespace failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("delete namespace failed"))
			})
		})
	})
})
