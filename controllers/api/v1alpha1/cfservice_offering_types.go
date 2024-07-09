package v1alpha1

import (
	"code.cloudfoundry.org/korifi/model/services"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CFServiceOfferingSpec defines the desired state of CFServiceOffering
type CFServiceOfferingSpec struct {
	services.ServiceOffering `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:printcolumn:name="Offering",type=string,JSONPath=`.spec.name`
//+kubebuilder:printcolumn:name="Description",type=string,JSONPath=`.spec.description`
//+kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.spec.available`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// CFServiceOffering is the Schema for the cfserviceofferings API
type CFServiceOffering struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CFServiceOfferingSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// CFServiceOfferingList contains a list of CFServiceOffering
type CFServiceOfferingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFServiceOffering `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFServiceOffering{}, &CFServiceOfferingList{})
}
