package workloads_test

import (
	"context"
	"errors"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("CFOrgReconciler", func() {
	var (
		fakeClient       *fake.Client
		fakeStatusWriter *fake.StatusWriter

		cfOrgGUID string

		cfOrg              *workloadsv1alpha1.CFOrg
		subNamespaceAnchor *v1alpha2.SubnamespaceAnchor
		namespace          *v1.Namespace

		subNamespaceAnchorState v1alpha2.SubnamespaceAnchorState
		cfOrgError              error
		subNamespaceAnchorError error
		createError             error
		namespaceError          error

		cfOrgReconciler *CFOrgReconciler
		ctx             context.Context
		req             ctrl.Request

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		fakeClient = new(fake.Client)

		cfOrgGUID = PrefixedGUID("cf-org")

		cfOrg = BuildCFOrgObject(cfOrgGUID, defaultNamespace)
		subNamespaceAnchor = BuildSubNamespaceAnchorObject(cfOrgGUID, defaultNamespace)
		namespace = BuildNamespaceObject(cfOrgGUID)

		subNamespaceAnchorState = v1alpha2.Ok
		cfOrgError = nil
		subNamespaceAnchorError = nil
		namespaceError = nil
		createError = nil
		reconcileErr = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *workloadsv1alpha1.CFOrg:
				cfOrg.DeepCopyInto(obj)
				return cfOrgError
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

		// configure a CFOrgReconciler with the client
		Expect(workloadsv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfOrgReconciler = NewCFOrgReconciler(
			fakeClient,
			scheme.Scheme,
			zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
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
			subNamespaceAnchorError = k8serrors.NewNotFound(schema.GroupResource{}, "CFOrg")
		})

		It("creates a subnamespace anchor and returns an empty result and does not return error", func() {
			Expect(reconcileResult).To(Equal(ctrl.Result{RequeueAfter: 100 * time.Millisecond}))
			Expect(reconcileErr).NotTo(HaveOccurred())
		})

		It("validates the inputs to Get", func() {
			Expect(fakeClient.GetCallCount()).To(Equal(2))
			_, namespacedName, _ := fakeClient.GetArgsForCall(0)
			Expect(namespacedName.Namespace).To(Equal(defaultNamespace))
			Expect(namespacedName.Name).To(Equal(cfOrgGUID))
			_, namespacedName, _ = fakeClient.GetArgsForCall(1)
			Expect(namespacedName.Namespace).To(Equal(defaultNamespace))
			Expect(namespacedName.Name).To(Equal(cfOrgGUID))
		})

		It("creates the subnamespace anchor", func() {
			Expect(fakeClient.CreateCallCount()).To(Equal(1))
			_, createArg, _ := fakeClient.CreateArgsForCall(0)
			castAnchor, ok := createArg.(*v1alpha2.SubnamespaceAnchor)
			Expect(ok).To(BeTrue(), "Cast to v1alpha2.SubnamespaceAnchor failed")
			Expect(castAnchor.ObjectMeta).To(Equal(metav1.ObjectMeta{
				Name:      cfOrgGUID,
				Namespace: defaultNamespace,
				Labels: map[string]string{
					OrgNameLabel: cfOrg.Spec.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: APIVersion,
						Kind:       Kind,
						Name:       cfOrg.Name,
						UID:        cfOrg.GetUID(),
					},
				},
			}))
			Expect(castAnchor.Status).To(Equal(v1alpha2.SubnamespaceAnchorStatus{
				State: v1alpha2.Ok,
			}))
		})

		It("does not set allowCascadingDeletion on the subnamespace anchor", func() {
			Expect(fakeClient.PatchCallCount()).To(Equal(0))
		})

		It("validates the condition on the CFOrg is set to Unknown", func() {
			Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
			_, updatedCFOrg, _ := fakeStatusWriter.UpdateArgsForCall(0)
			castCFOrg, ok := updatedCFOrg.(*workloadsv1alpha1.CFOrg)
			Expect(ok).To(BeTrue(), "Cast to workloadsv1alpha1.CFOrg failed")
			Expect(meta.IsStatusConditionPresentAndEqual(castCFOrg.Status.Conditions, StatusConditionReady, metav1.ConditionUnknown)).To(BeTrue(), "Status Condition "+StatusConditionReady+" was not True as expected")
			Expect(castCFOrg.Status.GUID).To(Equal(""))
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

			It("validates the inputs to Get", func() {
				Expect(fakeClient.GetCallCount()).To(Equal(3))
				_, namespacedName, _ := fakeClient.GetArgsForCall(2)
				Expect(namespacedName.Name).To(Equal(cfOrgGUID))
			})

			It("does not create the subsnamespace anchor", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(0))
			})

			It("sets allowCascadingDeletion on the subnamespace anchor", func() {
				Expect(fakeClient.PatchCallCount()).To(Equal(1))
				_, patchedHierarchy, _, _ := fakeClient.PatchArgsForCall(0)
				castHierarchy, ok := patchedHierarchy.(*v1alpha2.HierarchyConfiguration)
				Expect(ok).To(BeTrue(), "Cast to v1alpha2.HierarchyConfiguration failed")
				Expect(castHierarchy.Spec.AllowCascadingDeletion).To(BeTrue())
			})

			It("validates the condition on the CFOrg is set to Ready", func() {
				Expect(fakeStatusWriter.UpdateCallCount()).To(Equal(1))
				_, updatedCFOrg, _ := fakeStatusWriter.UpdateArgsForCall(0)
				castCFOrg, ok := updatedCFOrg.(*workloadsv1alpha1.CFOrg)
				Expect(ok).To(BeTrue(), "Cast to workloadsv1alpha1.CFOrg failed")
				Expect(meta.IsStatusConditionTrue(castCFOrg.Status.Conditions, StatusConditionReady)).To(BeTrue(), "Status Condition "+StatusConditionReady+" was not True as expected")
				Expect(castCFOrg.Status.GUID).To(Equal(namespace.Name))
			})
		})

		When("fetch CFOrg returns an error", func() {
			BeforeEach(func() {
				cfOrgError = errors.New("get CFOrg failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("get CFOrg failed"))
			})
		})

		When("fetch Subnamespace anchor returns an error other than not found", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = errors.New("get subnamespace anchor failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("get subnamespace anchor failed"))
			})
		})

		When("create Subnamespace anchor returns an error ", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = k8serrors.NewNotFound(schema.GroupResource{}, "CFOrg")
				createError = errors.New("create subnamespace anchor failed")
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("create subnamespace anchor failed"))
			})
		})

		When("update CFOrg status to unknown returns an error", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = k8serrors.NewNotFound(schema.GroupResource{}, "CFOrg")
				fakeStatusWriter.UpdateReturns(errors.New("update CFOrg status failed"))
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("update CFOrg status failed"))
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

		When("set cascading delete returns an error ", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Ok
				fakeClient.PatchReturns(errors.New("set cascading delete failed"))
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError(ContainSubstring("set cascading delete failed")))
			})
		})

		When("update CFOrg status to ready returns an error", func() {
			BeforeEach(func() {
				subNamespaceAnchorError = nil
				subNamespaceAnchor.Status.State = v1alpha2.Ok
				fakeStatusWriter.UpdateReturns(errors.New("update CFOrg status failed"))
			})

			It("should return an error", func() {
				Expect(reconcileErr).To(MatchError("update CFOrg status failed"))
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
	})
})
