package k8s_test

import (
	"context"
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"code.cloudfoundry.org/korifi/controllers/fake"
	"code.cloudfoundry.org/korifi/tools/k8s"
)

type fakeObjectReconciler struct {
	reconcileResourceError     error
	reconcileResourceCallCount int
	reconcileResourceObj       *korifiv1alpha1.CFOrg
}

func (f *fakeObjectReconciler) ReconcileResource(ctx context.Context, obj *korifiv1alpha1.CFOrg) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("fake reconciler reconciling")

	f.reconcileResourceCallCount++
	f.reconcileResourceObj = obj

	obj.Spec.DisplayName = "reconciled-display-name"
	obj.Status.GUID = "hello"

	return ctrl.Result{
		RequeueAfter: 1,
	}, f.reconcileResourceError
}

func (f *fakeObjectReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return nil
}

var _ = Describe("Reconcile", func() {
	var (
		ctx                context.Context
		fakeClient         *fake.Client
		fakeStatusWriter   *fake.StatusWriter
		patchingReconciler *k8s.PatchingReconciler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg]
		objectReconciler   *fakeObjectReconciler
		org                *korifiv1alpha1.CFOrg
		result             ctrl.Result
		err                error
	)

	BeforeEach(func() {
		objectReconciler = new(fakeObjectReconciler)
		fakeClient = new(fake.Client)
		fakeStatusWriter = new(fake.StatusWriter)
		fakeClient.StatusReturns(fakeStatusWriter)

		ctx = context.Background()
		org = &korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: uuid.NewString(),
				Name:      uuid.NewString(),
			},
		}

		fakeClient.PatchStub = func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) error {
			o, ok := obj.(*korifiv1alpha1.CFOrg)
			Expect(ok).To(BeTrue())
			o.Status = korifiv1alpha1.CFOrgStatus{}
			return nil
		}

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			o, ok := obj.(*korifiv1alpha1.CFOrg)
			Expect(ok).To(BeTrue())
			*o = *org

			return nil
		}

		patchingReconciler = k8s.NewPatchingReconciler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg](ctrl.Log, fakeClient, objectReconciler)
	})

	JustBeforeEach(func() {
		result, err = patchingReconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: org.Namespace,
				Name:      org.Name,
			},
		})
	})

	It("fetches the object", func() {
		Expect(fakeClient.GetCallCount()).To(Equal(1))
		_, namespacedName, obj, _ := fakeClient.GetArgsForCall(0)
		Expect(namespacedName.Namespace).To(Equal(org.Namespace))
		Expect(namespacedName.Name).To(Equal(org.Name))
		Expect(obj).To(BeAssignableToTypeOf(&korifiv1alpha1.CFOrg{}))
	})

	When("the object does not exist", func() {
		BeforeEach(func() {
			fakeClient.GetReturns(apierrors.NewNotFound(schema.GroupResource{}, "cforg"))
		})

		It("does not call the object reconciler and succeeds", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(objectReconciler.reconcileResourceCallCount).To(Equal(0))
		})
	})

	When("the getting the object fails", func() {
		BeforeEach(func() {
			fakeClient.GetReturns(errors.New("get-error"))
		})

		It("fails without calling the reconciler", func() {
			Expect(err).To(MatchError(ContainSubstring("get-error")))
			Expect(objectReconciler.reconcileResourceCallCount).To(Equal(0))
		})
	})

	It("calls the object reconciler", func() {
		Expect(objectReconciler.reconcileResourceCallCount).To(Equal(1))
		Expect(objectReconciler.reconcileResourceObj.Namespace).To(Equal(org.Namespace))
		Expect(objectReconciler.reconcileResourceObj.Name).To(Equal(org.Name))
	})

	It("patches the object via the k8s client", func() {
		Expect(fakeClient.PatchCallCount()).To(Equal(1))
		_, updatedObject, _, _ := fakeClient.PatchArgsForCall(0)
		updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
		Expect(ok).To(BeTrue())
		Expect(updatedOrg.Spec.DisplayName).To(Equal("reconciled-display-name"))
	})

	It("sets the Ready condition to true", func() {
		Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
		_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
		updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
		Expect(ok).To(BeTrue())
		Expect(updatedOrg.Status.Conditions).To(ContainElement(SatisfyAll(
			HasType(Equal(korifiv1alpha1.StatusConditionReady)),
			HasStatus(Equal(metav1.ConditionTrue)),
		)))
	})

	When("patching the object fails", func() {
		BeforeEach(func() {
			fakeClient.PatchReturns(errors.New("patch-object-error"))
		})

		It("returns the error", func() {
			Expect(err).To(MatchError(errors.New("patch-object-error")))
		})
	})

	It("patches the object status via the k8s client", func() {
		Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
		_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
		updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
		Expect(ok).To(BeTrue())
		Expect(updatedOrg.Status.GUID).To(Equal("hello"))
	})

	When("patching the object status fails", func() {
		BeforeEach(func() {
			fakeStatusWriter.PatchReturns(errors.New("patch-status-error"))
		})

		It("returns the error", func() {
			Expect(err).To(MatchError(errors.New("patch-status-error")))
		})
	})

	It("succeeds and returns the result from the object reconciler", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{RequeueAfter: 1}))
	})

	When("the object reconciliation fails", func() {
		BeforeEach(func() {
			objectReconciler.reconcileResourceError = errors.New("reconcile-error")
		})

		It("returns the error", func() {
			Expect(err).To(MatchError("reconcile-error"))
		})

		It("updates the object and its status nevertheless", func() {
			Expect(fakeClient.PatchCallCount()).To(Equal(1))
			Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
		})

		It("sets the ready condition to false with unknown reason", func() {
			Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
			_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
			updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
			Expect(ok).To(BeTrue())

			Expect(updatedOrg.Status.Conditions).To(ContainElement(SatisfyAll(
				HasType(Equal(korifiv1alpha1.StatusConditionReady)),
				HasStatus(Equal(metav1.ConditionFalse)),
				HasReason(Equal("Unknown")),
			)))
		})

		When("the object reconciliation fails with NotReady error", func() {
			BeforeEach(func() {
				objectReconciler.reconcileResourceError = k8s.NewNotReadyError()
			})

			It("sets the ready condition to false with unknown reason", func() {
				Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
				_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
				Expect(ok).To(BeTrue())

				Expect(updatedOrg.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
					HasReason(Equal("Unknown")),
					HasMessage(BeEmpty()),
				)))
			})

			When("reason is specified", func() {
				BeforeEach(func() {
					objectReconciler.reconcileResourceError = k8s.NewNotReadyError().WithReason("TestReason")
				})

				It("sets the ready condition to false with the reason specified", func() {
					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
					Expect(ok).To(BeTrue())

					Expect(updatedOrg.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("TestReason")),
					)))
				})
			})

			When("message is specified", func() {
				BeforeEach(func() {
					objectReconciler.reconcileResourceError = k8s.NewNotReadyError().WithMessage("TestMessage")
				})

				It("sets the ready condition to false with the message specified", func() {
					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
					Expect(ok).To(BeTrue())

					Expect(updatedOrg.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasMessage(Equal("TestMessage")),
					)))
				})
			})

			When("cause is specified", func() {
				BeforeEach(func() {
					objectReconciler.reconcileResourceError = k8s.NewNotReadyError().WithCause(errors.New("test-err"))
				})

				It("sets the ready condition to false with the error as message", func() {
					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
					Expect(ok).To(BeTrue())

					Expect(updatedOrg.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasMessage(Equal("test-err")),
					)))
				})
			})

			When("cause and message are specified", func() {
				BeforeEach(func() {
					objectReconciler.reconcileResourceError = k8s.NewNotReadyError().
						WithCause(errors.New("test-err")).
						WithMessage("TestMessage")
				})

				It("combines the cause error and the message", func() {
					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
					Expect(ok).To(BeTrue())

					Expect(updatedOrg.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasMessage(Equal("TestMessage: test-err")),
					)))
				})
			})

			When("requeue is specified", func() {
				BeforeEach(func() {
					objectReconciler.reconcileResourceError = k8s.NewNotReadyError().WithRequeue()
				})

				It("sets the ready condition to false", func() {
					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
					Expect(ok).To(BeTrue())

					Expect(updatedOrg.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
					)))
				})

				It("requeues the reconcile event", func() {
					Expect(result).To(Equal(ctrl.Result{Requeue: true}))
					Expect(err).NotTo(HaveOccurred())
				})
			})

			When("requeueAfter is specified", func() {
				BeforeEach(func() {
					objectReconciler.reconcileResourceError = k8s.NewNotReadyError().WithRequeueAfter(time.Minute)
				})

				It("sets the ready condition to false", func() {
					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
					Expect(ok).To(BeTrue())

					Expect(updatedOrg.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
					)))
				})

				It("requeues the reconcile event", func() {
					Expect(result).To(Equal(ctrl.Result{RequeueAfter: time.Minute}))
					Expect(err).NotTo(HaveOccurred())
				})
			})

			When("no requeue is specified", func() {
				BeforeEach(func() {
					objectReconciler.reconcileResourceError = k8s.NewNotReadyError().WithNoRequeue()
				})

				It("sets the ready condition to false", func() {
					Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
					_, updatedObject, _, _ := fakeStatusWriter.PatchArgsForCall(0)
					updatedOrg, ok := updatedObject.(*korifiv1alpha1.CFOrg)
					Expect(ok).To(BeTrue())

					Expect(updatedOrg.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
					)))
				})

				It("does not requeue the reconcile event", func() {
					Expect(result).To(Equal(ctrl.Result{}))
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})
	})

	Describe("logging", func() {
		var logOutput *gbytes.Buffer

		BeforeEach(func() {
			logOutput = gbytes.NewBuffer()
			GinkgoWriter.TeeTo(logOutput)
			logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
		})

		It("captures logs from object reconciler", func() {
			Eventually(logOutput).Should(SatisfyAll(
				gbytes.Say("fake reconciler reconciling"),
				gbytes.Say(`"namespace":`),
				gbytes.Say(`"name":`),
				gbytes.Say(`"logID":`),
			))
		})
	})
})
