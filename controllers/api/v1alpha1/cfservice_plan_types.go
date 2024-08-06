package v1alpha1

import (
	"code.cloudfoundry.org/korifi/model/services"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CFServicePlanSpec struct {
	services.ServicePlan `json:",inline"`
	Visibility           ServicePlanVisibility `json:"visibility"`
}

const (
	AdminServicePlanVisibilityType        = "admin"
	PublicServicePlanVisibilityType       = "public"
	OrganizationServicePlanVisibilityType = "organization"
)

type ServicePlanVisibility struct {
	// +kubebuilder:validation:Enum=admin;public;organization
	Type string `json:"type"`
	// +kubebuilder:validation:Optional
	Organizations []string `json:"organizations,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Plan",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.spec.available`
// +kubebuilder:printcolumn:name="Free",type=string,JSONPath=`.spec.free`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
type CFServicePlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CFServicePlanSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type CFServicePlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFServicePlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFServicePlan{}, &CFServicePlanList{})
}
