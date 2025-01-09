package sbio_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/bindings/sbio"
	"code.cloudfoundry.org/korifi/tools"
)

var _ = Describe("SBIO", func() {
	Describe("ToSBServiceBinding", func() {
		var (
			cfServiceBinding *korifiv1alpha1.CFServiceBinding
			bindingName      string
			sbioBinding      *servicebindingv1beta1.ServiceBinding
		)

		BeforeEach(func() {
			bindingName = "cf-binding"

			cfServiceBinding = &korifiv1alpha1.CFServiceBinding{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: uuid.NewString(),
					Finalizers: []string{
						korifiv1alpha1.CFServiceBindingFinalizerName,
					},
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "ServiceInstance",
						Name:       uuid.NewString(),
						APIVersion: "korifi.cloudfoundry.org/v1alpha1",
					},
					AppRef: corev1.LocalObjectReference{
						Name: uuid.NewString(),
					},
				},
				Status: korifiv1alpha1.CFServiceBindingStatus{
					Binding: corev1.LocalObjectReference{
						Name: bindingName,
					},
				},
			}
		})

		JustBeforeEach(func() {
			sbioBinding = sbio.ToSBServiceBinding(cfServiceBinding, korifiv1alpha1.ManagedType)
		})

		It("should transform CFServiceBinding to SBerviceBinding correctly", func() {
			Expect(sbioBinding).To(Equal(&servicebindingv1beta1.ServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cf-binding-cf-binding",
					Namespace: cfServiceBinding.Namespace,
					Labels: map[string]string{
						korifiv1alpha1.ServiceBindingGUIDLabel: bindingName,
						korifiv1alpha1.CFAppGUIDLabelKey:       cfServiceBinding.Spec.AppRef.Name,
						korifiv1alpha1.ServiceBindingTypeLabel: "app",
					},
				},
				Spec: servicebindingv1beta1.ServiceBindingSpec{
					Name: bindingName,
					Type: korifiv1alpha1.ManagedType,
					Workload: servicebindingv1beta1.ServiceBindingWorkloadReference{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								korifiv1alpha1.CFAppGUIDLabelKey: cfServiceBinding.Spec.AppRef.Name,
							},
						},
					},
					Service: servicebindingv1beta1.ServiceBindingServiceReference{
						APIVersion: "korifi.cloudfoundry.org/v1alpha1",
						Kind:       "CFServiceBinding",
						Name:       bindingName,
					},
				},
			}))
		})

		When("the binding display name is set", func() {
			BeforeEach(func() {
				cfServiceBinding.Spec.DisplayName = tools.PtrTo("binding-display-name")
			})

			It("sets it in the servicebinding spec name", func() {
				Expect(sbioBinding.Spec.Name).To(Equal("binding-display-name"))
			})
		})
	})

	Describe("IsSBBindingReady", func() {
		var (
			sbioBinding *servicebindingv1beta1.ServiceBinding
			ready       bool
		)

		BeforeEach(func() {
			sbioBinding = &servicebindingv1beta1.ServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 4,
				},
				Status: servicebindingv1beta1.ServiceBindingStatus{
					ObservedGeneration: 4,
				},
			}
			meta.SetStatusCondition(&sbioBinding.Status.Conditions, metav1.Condition{
				Type:   korifiv1alpha1.StatusConditionReady,
				Status: metav1.ConditionTrue,
			})
		})

		JustBeforeEach(func() {
			ready = sbio.IsSbServiceBindingReady(sbioBinding)
		})

		It("returns true", func() {
			Expect(ready).To(BeTrue())
		})

		When("the ready condition is false", func() {
			BeforeEach(func() {
				meta.SetStatusCondition(&sbioBinding.Status.Conditions, metav1.Condition{
					Type:   korifiv1alpha1.StatusConditionReady,
					Status: metav1.ConditionFalse,
				})
			})

			It("returns false", func() {
				Expect(ready).To(BeFalse())
			})
		})

		When("sbiobinding status is outdated", func() {
			BeforeEach(func() {
				sbioBinding = &servicebindingv1beta1.ServiceBinding{
					Status: servicebindingv1beta1.ServiceBindingStatus{
						ObservedGeneration: 3,
					},
				}
			})

			It("returns false", func() {
				Expect(ready).To(BeFalse())
			})
		})
	})
})
