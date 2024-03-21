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

type BrokerCatalogFeatures struct {
	Plan_updateable       bool `json:"plan_updateable"`
	Bindable              bool `json:"bindable"`
	Instances_retrievable bool `json:"instances_retrievable"`
	Bindings_retrievable  bool `json:"bindings_retrievable"`
	Allow_context_updates bool `json:"allow_context_updates"`
}

type ServiceBrokerCatalog struct {
	Id       string                `json:"id"`
	Metadata *runtime.RawExtension `json:"metadata"`
	Features BrokerCatalogFeatures `json:"features"`
}

type ServiceOfferingRelationships struct {
	Service_broker ToOneRelationship `json:"service_broker"`
}

func (rel *ServiceOfferingRelationships) Create(plan *CFServiceOffering) {
	rel.Service_broker = ToOneRelationship{
		Data: Relationship{
			GUID: plan.Labels[RelServiceBrokerLabel],
		},
	}
}

type BrokerServiceOffering struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Requires    []string `json:"required,omitempty"`
	Shareable   bool     `json:"shareable"`
	// +kubebuilder:validation:Optional
	Documentation_url *string              `json:"documentationUrl"`
	Broker_catalog    ServiceBrokerCatalog `json:"broker_catalog"`
}

type ServiceOffering struct {
	BrokerServiceOffering `json:",inline"`
	Available             bool `json:"available"`
}

type ServiceOfferingResource struct {
	ServiceOffering
	CFResource
	Relationships ServiceOfferingRelationships
}

// CFServiceOfferingSpec defines the desired state of CFServiceOffering
type CFServiceOfferingSpec struct {
	ServiceOffering `json:",inline"`
	// +kubebuilder:validation:Optional
	Metadata *Metadata `json:"metadata"`
}

// CFServiceOfferingStatus defines the observed state of CFServiceOffering
type CFServiceOfferingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Offering",type=string,JSONPath=`.spec.name`
//+kubebuilder:printcolumn:name="Description",type=string,JSONPath=`.spec.description`
//+kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.spec.available`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// CFServiceOffering is the Schema for the cfserviceofferings API
type CFServiceOffering struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFServiceOfferingSpec   `json:"spec,omitempty"`
	Status CFServiceOfferingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFServiceOfferingList contains a list of CFServiceOffering
type CFServiceOfferingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFServiceOffering `json:"items"`
}

type ServicePlanVisibilityResource struct {
	Type string `json:"type"`
}

func init() {
	SchemeBuilder.Register(&CFServiceOffering{}, &CFServiceOfferingList{})
}
