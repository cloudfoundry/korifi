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

// CFDropletSpec defines the desired state of CFDroplet
type CFDropletSpec struct {
	// Specifies the Lifecycle type of the droplet
	Type LifecycleType `json:"type"`

	// Specifies the App associated with this Droplet
	AppRef v1.LocalObjectReference `json:"appRef"`

	// Specifies the Build associated with this Droplet
	BuildRef v1.LocalObjectReference `json:"buildRef"`

	// Specifies the Container registry image, and secrets to access
	Registry Registry `json:"registry"`

	// Specifies the process types and associated start commands for the Droplet
	ProcessTypes []ProcessType `json:"processTypes"`

	// Specifies the exposed ports for the application
	Ports []int32 `json:"ports"`
}

type ProcessType struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// CFDropletStatus defines the observed state of CFDroplet
type CFDropletStatus struct {
	// Conditions capture the current status of the Droplet
	Conditions []metav1.Condition `json:"conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CFDroplet is the Schema for the cfdroplets API
type CFDroplet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFDropletSpec   `json:"spec,omitempty"`
	Status CFDropletStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFDropletList contains a list of CFDroplet
type CFDropletList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFDroplet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFDroplet{}, &CFDropletList{})
}
