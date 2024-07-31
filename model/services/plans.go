package services

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:generate=true
type ServicePlan struct {
	Name          string                   `json:"name"`
	Free          bool                     `json:"free"`
	Description   string                   `json:"description,omitempty"`
	BrokerCatalog ServicePlanBrokerCatalog `json:"broker_catalog"`
	Schemas       ServicePlanSchemas       `json:"schemas"`
}

// +kubebuilder:object:generate=true
type ServicePlanBrokerCatalog struct {
	ID string `json:"id"`
	// +kubebuilder:validation:Optional
	Metadata *runtime.RawExtension `json:"metadata,omitempty"`
	// +kubebuilder:validation:Optional
	Features ServicePlanFeatures `json:"features"`
}

// +kubebuilder:object:generate=true
type InputParameterSchema struct {
	// +kubebuilder:validation:Optional
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// +kubebuilder:object:generate=true
type ServiceInstanceSchema struct {
	Create InputParameterSchema `json:"create"`
	Update InputParameterSchema `json:"update"`
}

// +kubebuilder:object:generate=true
type ServiceBindingSchema struct {
	Create InputParameterSchema `json:"create"`
}

// +kubebuilder:object:generate=true
type ServicePlanSchemas struct {
	ServiceInstance ServiceInstanceSchema `json:"service_instance"`
	ServiceBinding  ServiceBindingSchema  `json:"service_binding"`
}

type ServicePlanFeatures struct {
	PlanUpdateable bool `json:"plan_updateable"`
	Bindable       bool `json:"bindable"`
}
