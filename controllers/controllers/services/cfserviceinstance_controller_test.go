package services_test

import (
	"context"
	"errors"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/services"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFServiceInstance.Reconcile", func() {
	var (
		cfServiceInstance               *korifiv1alpha1.CFServiceInstance
		cfServiceInstanceSecret         *corev1.Secret
		getCFServiceInstanceSecretError error

		cfServiceInstanceReconciler *k8s.PatchingReconciler[korifiv1alpha1.CFServiceInstance, *korifiv1alpha1.CFServiceInstance]
		ctx                         context.Context
		req                         ctrl.Request

		reconcileErr error
	)

	BeforeEach(func() {
		getCFServiceInstanceSecretError = nil

		cfServiceInstance = new(korifiv1alpha1.CFServiceInstance)
		cfServiceInstanceSecret = new(corev1.Secret)

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFServiceInstance:
				cfServiceInstance.DeepCopyInto(obj)
				return nil
			case *corev1.Secret:
				cfServiceInstanceSecret.DeepCopyInto(obj)
				return getCFServiceInstanceSecretError
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

		cfServiceInstanceReconciler = NewCFServiceInstanceReconciler(
			fakeClient,
			scheme.Scheme,
			zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
		)
		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "make-this-a-guid",
				Namespace: "make-this-a-guid-too",
			},
		}
	})

	JustBeforeEach(func() {
		_, reconcileErr = cfServiceInstanceReconciler.Reconcile(ctx, req)
	})

	When("the API errors fetching the secret", func() {
		BeforeEach(func() {
			getCFServiceInstanceSecretError = errors.New("some random error")
		})

		It("errors, and updates status", func() {
			Expect(reconcileErr).To(HaveOccurred())

			Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
			_, serviceInstanceObj, _, _ := fakeStatusWriter.PatchArgsForCall(0)
			updatedCFServiceInstance, ok := serviceInstanceObj.(*korifiv1alpha1.CFServiceInstance)
			Expect(ok).To(BeTrue())

			Expect(updatedCFServiceInstance.Status.Binding).To(BeZero())
			Expect(updatedCFServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Type":    Equal("BindingSecretAvailable"),
				"Status":  Equal(metav1.ConditionFalse),
				"Reason":  Equal("UnknownError"),
				"Message": Equal("Error occurred while fetching secret: " + getCFServiceInstanceSecretError.Error()),
			})))
		})
	})
})
