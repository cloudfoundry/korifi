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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	UserProvidedType = "user-provided"
)

// CFServiceInstanceSpec defines the desired state of CFServiceInstance
type CFServiceInstanceSpec struct {
	// The mutable, user-friendly name of the service instance. Unlike metadata.name, the user can change this field
	DisplayName string `json:"displayName"`

	// Name of a secret containing the service credentials. The Secret must be in the same namespace
	SecretName string `json:"secretName"`

	// Type of the Service Instance. Must be `user-provided`
	Type InstanceType `json:"type"`

	// Service label to use when adding this instance to VCAP_Services
	// Defaults to `user-provided` when this field is not set
	// +optional
	ServiceLabel *string `json:"serviceLabel,omitempty"`

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
	// +optional
	Binding corev1.LocalObjectReference `json:"binding"`

	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration captures the latest generation of the CFServiceInstance that has been reconciled
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// CFServiceInstance is the Schema for the cfserviceinstances API
type CFServiceInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CFServiceInstanceSpec `json:"spec,omitempty"`

	Status CFServiceInstanceStatus `json:"status,omitempty"`
}

func (si CFServiceInstance) UniqueName() string {
	return si.Spec.DisplayName
}

func (si CFServiceInstance) UniqueValidationErrorMessage() string {
	// Note: the cf cli expects the specific text 'The service instance name is taken'
	return fmt.Sprintf("The service instance name is taken: %s", si.Spec.DisplayName)
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
