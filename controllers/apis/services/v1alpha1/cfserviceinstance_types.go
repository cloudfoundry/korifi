/*
Copyright 2021.

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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	UserProvidedType = "user-provided"
)

// CFServiceInstanceSpec defines the desired state of CFServiceInstance
type CFServiceInstanceSpec struct {
	// DisplayName defines the name of the Service Instance
	DisplayName string `json:"displayName"`

	// Name of a secret containing the service credentials
	SecretName string `json:"secretName"`

	// Type of the Service Instance. Must be `user-provided`
	Type InstanceType `json:"type"`

	// Tags are used by apps to identify service instances
	Tags []string `json:"tags,omitempty"`
}

// InstanceType defines the type of the Service Instance
// +kubebuilder:validation:Enum=user-provided
type InstanceType string

// CFServiceInstanceStatus defines the observed state of CFServiceInstance
type CFServiceInstanceStatus struct {
	// A reference to the Secret containing the credentials (same as spec.secretName).
	// This is required to conform to the Kubernetes Service Bindings spec
	Binding v1.LocalObjectReference `json:"binding"`

	// Conditions capture the current status of the CFServiceInstance
	Conditions []metav1.Condition `json:"conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CFServiceInstance is the Schema for the cfserviceinstances API
type CFServiceInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CFServiceInstanceSpec `json:"spec,omitempty"`

	Status CFServiceInstanceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFServiceInstanceList contains a list of CFServiceInstance
type CFServiceInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFServiceInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFServiceInstance{}, &CFServiceInstanceList{})
}
