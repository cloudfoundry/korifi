package controllers_test

import (
	"context"
	"errors"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/statefulset-runner/fake"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	testAppWorkloadGUID = "test-appworkload-guid"
	testNamespace       = "test-ns"
)

var _ = Describe("AppWorkload Reconcile", func() {
	var (
		reconciler             *k8s.PatchingReconciler[korifiv1alpha1.AppWorkload, *korifiv1alpha1.AppWorkload]
		reconcileResult        ctrl.Result
		reconcileErr           error
		ctx                    context.Context
		req                    ctrl.Request
		appWorkload            *korifiv1alpha1.AppWorkload
		statefulSet            *v1.StatefulSet
		fakeWorkloadToStSet    *fake.WorkloadToStatefulsetConverter
		fakePDB                *fake.PDB
		getAppWorkloadError    error
		getStatefulSetError    error
		createStatefulSetError error
	)

	BeforeEach(func() {
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		appWorkload = createAppWorkload("some-namespace", "guid_1234")
		statefulSet = &v1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "converted-stset",
				Namespace: testNamespace,
			},
		}

		fakeWorkloadToStSet = new(fake.WorkloadToStatefulsetConverter)
		fakeWorkloadToStSet.ConvertReturns(statefulSet, nil)

		fakePDB = new(fake.PDB)

		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      testAppWorkloadGUID,
				Namespace: testNamespace,
			},
		}

		getAppWorkloadError = nil
		getStatefulSetError = apierrors.NewNotFound(schema.GroupResource{
			Group:    "v1",
			Resource: "StatefulSet",
		}, "some-resource")
		createStatefulSetError = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.AppWorkload:
				appWorkload.DeepCopyInto(obj)
				return getAppWorkloadError
			case *v1.StatefulSet:
				if getStatefulSetError == nil {
					statefulSet.DeepCopyInto(obj)
				}
				return getStatefulSetError
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		fakeClient.CreateStub = func(ctx context.Context, obj client.Object, option ...client.CreateOption) error {
			switch obj.(type) {
			case *v1.StatefulSet:
				return createStatefulSetError
			default:
				panic("TestClient Create provided an unexpected object type")
			}
		}

		reconciler = controllers.NewAppWorkloadReconciler(fakeClient, scheme.Scheme, fakeWorkloadToStSet, fakePDB, zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = reconciler.Reconcile(ctx, req)
	})

	When("the appworkload is being created", func() {
		It("returns an empty result and does not return error", func() {
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
			Expect(reconcileErr).NotTo(HaveOccurred())
		})

		It("converts the app workload to a statefulset", func() {
			Expect(fakeWorkloadToStSet.ConvertCallCount()).To(Equal(1))
			actualWorkload := fakeWorkloadToStSet.ConvertArgsForCall(0)
			Expect(actualWorkload).To(Equal(appWorkload))
		})

		When("coverting the app workload to statefulset fails", func() {
			BeforeEach(func() {
				fakeWorkloadToStSet.ConvertReturns(nil, errors.New("convert-error"))
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("convert-error"))
			})
		})

		It("creates a StatefulSet", func() {
			Expect(fakeClient.CreateCallCount()).To(Equal(1), "Client.Create call count mismatch")
			_, obj, _ := fakeClient.CreateArgsForCall(0)
			Expect(obj).To(BeAssignableToTypeOf(new(v1.StatefulSet)))
		})

		When("creating the StatefulSet fails", func() {
			BeforeEach(func() {
				createStatefulSetError = errors.New("big sad")
			})

			It("returns an error", func() {
				Expect(reconcileErr).To(MatchError("big sad"))
			})
		})

		When("reconciler name on the AppWorkload is not statefulset-runner", func() {
			BeforeEach(func() {
				appWorkload.Spec.RunnerName = "MyCustomReconciler"
			})

			It("does not create/patch statefulset", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(0), "Client.Create call count mismatch")
			})
		})
	})

	When("the appworkload is being deleted", func() {
		BeforeEach(func() {
			getAppWorkloadError = apierrors.NewNotFound(schema.GroupResource{
				Group:    "v1alpha1",
				Resource: "AppWorkload",
			}, "some-resource")
		})

		It("returns an empty result and does not return error", func() {
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
			Expect(reconcileErr).NotTo(HaveOccurred())
		})
	})

	When("the appworkload is being updated", func() {
		BeforeEach(func() {
			getStatefulSetError = nil

			desiredStSet := statefulSet.DeepCopy()
			desiredStSet.Spec.Replicas = tools.PtrTo(int32(2))
			fakeWorkloadToStSet.ConvertReturns(desiredStSet, nil)
		})

		It("scales instances", func() {
			Expect(fakeClient.PatchCallCount()).To(BeNumerically(">", 1))
			_, updatedObject, _, _ := fakeClient.PatchArgsForCall(0)
			updatedStSet, ok := updatedObject.(*v1.StatefulSet)
			Expect(ok).To(BeTrue())
			Expect(updatedStSet.Spec.Replicas).To(Equal(tools.PtrTo(int32(2))))
		})

		When("updating the pod disruption budget fails", func() {
			BeforeEach(func() {
				fakePDB.UpdateReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(reconcileErr).To(MatchError("boom"))
			})
		})
	})
})

func expectedValFrom(fieldPath string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: "",
			FieldPath:  fieldPath,
		},
	}
}
