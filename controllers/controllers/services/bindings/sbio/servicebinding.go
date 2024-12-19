package sbio

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ToSBServiceBinding(cfServiceBinding *korifiv1alpha1.CFServiceBinding, bindingType string) *servicebindingv1beta1.ServiceBinding {
	return &servicebindingv1beta1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cf-binding-%s", cfServiceBinding.Name),
			Namespace: cfServiceBinding.Namespace,
			Labels: map[string]string{
				korifiv1alpha1.ServiceBindingGUIDLabel:           cfServiceBinding.Name,
				korifiv1alpha1.CFAppGUIDLabelKey:                 cfServiceBinding.Spec.AppRef.Name,
				korifiv1alpha1.ServiceCredentialBindingTypeLabel: "app",
			},
		},
		Spec: servicebindingv1beta1.ServiceBindingSpec{
			Name: getSBServiceBindingName(cfServiceBinding),
			Type: bindingType,
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
				Name:       cfServiceBinding.Name,
			},
		},
	}
}

func getSBServiceBindingName(cfServiceBinding *korifiv1alpha1.CFServiceBinding) string {
	if cfServiceBinding.Spec.DisplayName != nil {
		return *cfServiceBinding.Spec.DisplayName
	}

	return cfServiceBinding.Status.Binding.Name
}

func IsSbServiceBindingReady(sbServiceBinding *servicebindingv1beta1.ServiceBinding) bool {
	if sbServiceBinding.Generation != sbServiceBinding.Status.ObservedGeneration {
		return false
	}

	return meta.IsStatusConditionTrue(sbServiceBinding.Status.Conditions, korifiv1alpha1.StatusConditionReady)
}
