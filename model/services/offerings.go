package services

import "k8s.io/apimachinery/pkg/runtime"

// +kubebuilder:object:generate=true
type ServiceOffering struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Requires    []string `json:"required,omitempty"`
	// +kubebuilder:validation:Optional
	DocumentationURL *string              `json:"documentation_url"`
	BrokerCatalog    ServiceBrokerCatalog `json:"broker_catalog"`
}

// +kubebuilder:object:generate=true
type ServiceBrokerCatalog struct {
	ID string `json:"id"`
	// +kubebuilder:validation:Optional
	Metadata *runtime.RawExtension `json:"metadata"`
	Features BrokerCatalogFeatures `json:"features"`
}

type BrokerCatalogFeatures struct {
	PlanUpdateable       bool `json:"plan_updateable"`
	Bindable             bool `json:"bindable"`
	InstancesRetrievable bool `json:"instances_retrievable"`
	BindingsRetrievable  bool `json:"bindings_retrievable"`
	AllowContextUpdates  bool `json:"allow_context_updates"`
}
