package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// CFServiceOfferingSpec defines the desired state of CFServiceOffering
type CFServiceOfferingSpec struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Requires    []string `json:"requires,omitempty"`
	// +kubebuilder:validation:Optional
	DocumentationURL *string              `json:"documentationUrl"`
	BrokerCatalog    ServiceBrokerCatalog `json:"brokerCatalog"`
}

type ServiceBrokerCatalog struct {
	ID string `json:"id"`
	// +kubebuilder:validation:Optional
	Metadata *runtime.RawExtension `json:"metadata"`
	Features BrokerCatalogFeatures `json:"features"`
}

type BrokerCatalogFeatures struct {
	PlanUpdateable       bool `json:"planUpdateable"`
	Bindable             bool `json:"bindable"`
	InstancesRetrievable bool `json:"instancesRetrievable"`
	BindingsRetrievable  bool `json:"bindingsRetrievable"`
	AllowContextUpdates  bool `json:"allowContextUpdates"`
}

//+kubebuilder:object:root=true
//+kubebuilder:printcolumn:name="Offering",type=string,JSONPath=`.spec.name`
//+kubebuilder:printcolumn:name="Description",type=string,JSONPath=`.spec.description`
//+kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.spec.available`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFServiceOffering is the Schema for the cfserviceofferings API
type CFServiceOffering struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CFServiceOfferingSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFServiceOfferingList contains a list of CFServiceOffering
type CFServiceOfferingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFServiceOffering `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFServiceOffering{}, &CFServiceOfferingList{})
}
