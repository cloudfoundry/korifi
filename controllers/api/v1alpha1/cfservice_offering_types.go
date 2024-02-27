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
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CFServiceOfferingSpec defines the desired state of CFServiceOffering
type CFServiceOfferingSpec struct {
	Id                   string      `json:"id"`
	OfferingName         string      `json:"offeringName"`
	Description          string      `json:"description,omitempty"`
	Available            bool        `json:"available"`
	Tags                 []string    `json:"tags,omitempty"`
	Requires             []string    `json:"required,omitempty"`
	CreationTimestamp    metav1.Time `json:"creationTimestamp"`
	UpdatedTimestamp     metav1.Time `json:"updatedTimestamp"`
	Shareable            bool        `json:"shareable,omitempty"`
	DocumentationUrl     string      `json:"documentationUrl,omitempty"`
	Bindable             bool        `json:"bindable"`
	PlanUpdateable       bool        `json:"plan_updateable"`
	InstancesRetrievable bool        `json:"instances_retrievable"`
	BindingsRetrievable  bool        `json:"bindings_retrievable"`
	AllowContextUpdates  bool        `json:"allow_context_updates"`
	CatalogId            string      `json:"catalog_id"`

	Metadata *Metadata `json:"metadata,omitempty"`
}

// CFServiceOfferingStatus defines the observed state of CFServiceOffering
type CFServiceOfferingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Offering",type=string,JSONPath=`.spec.offeringName`
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

func init() {
	SchemeBuilder.Register(&CFServiceOffering{}, &CFServiceOfferingList{})
}
