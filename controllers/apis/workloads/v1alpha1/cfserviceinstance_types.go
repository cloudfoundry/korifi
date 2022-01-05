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

// CFServiceInstanceSpec defines the desired state of CFServiceInstance
type CFServiceInstanceSpec struct {
	// Specifies the Name of the Secret containing service credentials
	SecretName string `json:"secret_name"`
	// Specifies the type of service for this instance (currently only "user-provided")
	Type string `json:"type"`
}

// CFServiceInstanceStatus defines the observed state of CFServiceInstance
type CFServiceInstanceStatus struct {
	// Specifies the Name of the Secret containing service credentials
	Binding v1.LocalObjectReference `json:"binding"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CFServiceInstance is the Schema for the cfprocesses API
type CFServiceInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFServiceInstanceSpec   `json:"spec,omitempty"`
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
