package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type CFServicePlanSpec struct {
	Name            string                   `json:"name"`
	Free            bool                     `json:"free"`
	Description     string                   `json:"description,omitempty"`
	BrokerCatalog   ServicePlanBrokerCatalog `json:"brokerCatalog"`
	Schemas         ServicePlanSchemas       `json:"schemas"`
	MaintenanceInfo MaintenanceInfo          `json:"maintenanceInfo"`
	Visibility      ServicePlanVisibility    `json:"visibility"`
}

type ServicePlanBrokerCatalog struct {
	ID string `json:"id"`
	// +kubebuilder:validation:Optional
	Metadata *runtime.RawExtension `json:"metadata,omitempty"`
	// +kubebuilder:validation:Optional
	Features ServicePlanFeatures `json:"features"`
}

type InputParameterSchema struct {
	// +kubebuilder:validation:Optional
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

type ServiceInstanceSchema struct {
	Create InputParameterSchema `json:"create"`
	Update InputParameterSchema `json:"update"`
}

type ServiceBindingSchema struct {
	Create InputParameterSchema `json:"create"`
}

type ServicePlanSchemas struct {
	ServiceInstance ServiceInstanceSchema `json:"serviceInstance"`
	ServiceBinding  ServiceBindingSchema  `json:"serviceBinding"`
}

type ServicePlanFeatures struct {
	PlanUpdateable bool `json:"planUpdateable"`
	Bindable       bool `json:"bindable"`
}

type VisibilityOrganization struct {
	GUID string `json:"guid"`
	Name string `json:"name"`
}

type MaintenanceInfo struct {
	Version string `json:"version"`
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
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CFServicePlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CFServicePlanSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CFServicePlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFServicePlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFServicePlan{}, &CFServicePlanList{})
}
