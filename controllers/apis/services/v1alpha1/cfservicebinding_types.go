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

// CFServiceBindingSpec defines the desired state of CFServiceBinding
type CFServiceBindingSpec struct {
	// Name defines the name of the Service Binding
	Name string `json:"name"`

	// Specifies the Service this binding uses
	Service v1.ObjectReference `json:"service"`

	// Name of a secret containing the service credentials
	SecretName string `json:"secretName"`

	// Specifies the App that owns this process
	AppRef v1.LocalObjectReference `json:"appRef"`
}

// CFServiceBindingStatus defines the observed state of CFServiceBinding
type CFServiceBindingStatus struct {
	// A reference to the Secret containing the credentials (same as spec.secretName).
	// This is required to conform to the Kubernetes Service Bindings spec
	Binding v1.LocalObjectReference `json:"binding"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CFServiceBinding is the Schema for the cfservicebindings API
type CFServiceBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFServiceBindingSpec   `json:"spec,omitempty"`
	Status CFServiceBindingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFServiceBindingList contains a list of CFServiceBinding
type CFServiceBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFServiceBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFServiceBinding{}, &CFServiceBindingList{})
}
