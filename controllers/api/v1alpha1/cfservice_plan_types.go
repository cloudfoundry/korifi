/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ServicePlanCost struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Unit     string `json:"unit"`
}

type ServicePlanMaintenanceInfo struct {
	Version     string `json:"version"`
	Description string `json:"description"`
}

type ServicePlanFeatures struct {
	Plan_updateable bool `json:"plan_updateable"`
	Bindable        bool `json:"bindable"`
}

type ServicePlanBrokerCatalog struct {
	Id string `json:"Id"`
	// +kubebuilder:validation:Optional
	Metadata *runtime.RawExtension `json:"metadata"`
	// +kubebuilder:validation:Optional
	Maximum_polling_duration *int32              `json:"maximum_polling_duration"`
	Features                 ServicePlanFeatures `json:"features"`
}

type ServicePlanRelationships struct {
	Service_offering ToOneRelationship `json:"service_offering"`
	// +kubebuilder:validation:Optional
	Space *ToOneRelationship `json:"space"`
}

type InputParameterSchema struct {
	Parameters runtime.RawExtension `json:"parameters"`
}

type ServiceInstanceSchema struct {
	Create InputParameterSchema `json:"create"`
	Update InputParameterSchema `json:"update"`
}

type ServiceBindingSchema struct {
	Create InputParameterSchema `json:"create"`
}

type ServicePlanSchemas struct {
	Service_instance ServiceInstanceSchema `json:"service_instance"`
	Service_binding  ServiceBindingSchema  `json:"service_binding"`
}

type BrokerServicePlan struct {
	Name string `json:"name"`
	Free bool   `json:"free"`
	// +kubebuilder:validation:Optional
	Description      *string                    `json:"description,omitempty"`
	Maintenance_info ServicePlanMaintenanceInfo `json:"maintenance_info"`
	Broker_catalog   ServicePlanBrokerCatalog   `json:"broker_catalog"`
	// +kubebuilder:validation:Optional
	Schemas       *ServicePlanSchemas      `json:"schemas"`
	Relationships ServicePlanRelationships `json:"relationships"`
}

type ServicePlan struct {
	BrokerServicePlan `json:",inline"`
	// +kubebuilder:validation:Optional
	Costs           []ServicePlanCost `json:"costs"`
	Available       bool              `json:"available"`
	Visibility_type string            `json:"visibility_type"`
}

type ServicePlanResource struct {
	ServicePlan
	CFResource
}

// CFServicePlanSpec defines the desired state of CFServicePlan
type CFServicePlanSpec struct {
	ServicePlan `json:",inline"`
}

// CFServicePlanStatus defines the observed state of CFServicePlan
type CFServicePlanStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Plan",type=string,JSONPath=`.spec.name`
//+kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.spec.available`
//+kubebuilder:printcolumn:name="Free",type=string,JSONPath=`.spec.free`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// CFServicePlan is the Schema for the cfserviceplans API
type CFServicePlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFServicePlanSpec   `json:"spec,omitempty"`
	Status CFServicePlanStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFServicePlanList contains a list of CFServicePlan
type CFServicePlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFServicePlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFServicePlan{}, &CFServicePlanList{})
}
