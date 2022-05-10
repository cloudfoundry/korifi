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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CFSpaceSpec defines the desired state of CFSpace
type CFSpaceSpec struct {
	// DisplayName of the space displayed to the user
	// +kubebuilder:validation:Pattern="^[-\\w]+$"
	DisplayName string `json:"displayName"`
}

// CFSpaceStatus defines the observed state of CFSpace
type CFSpaceStatus struct {
	// Conditions capture the current status of the CFSpace
	Conditions []metav1.Condition `json:"conditions"`

	GUID string `json:"guid"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CFSpace is the Schema for the cfspaces API
type CFSpace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFSpaceSpec   `json:"spec,omitempty"`
	Status CFSpaceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFSpaceList contains a list of CFSpace
type CFSpaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFSpace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFSpace{}, &CFSpaceList{})
}
