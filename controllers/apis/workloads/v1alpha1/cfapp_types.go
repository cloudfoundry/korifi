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

// CFAppSpec defines the desired state of CFApp
type CFAppSpec struct {
	// Name defines the name of the app
	// +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"
	Name string `json:"name"`

	// Specifies the current state of the CFApp
	// Allowed values are:
	// "STARTED": App is started
	// "STOPPED": App is stopped
	DesiredState DesiredState `json:"desiredState"`

	// Lifecycle specifies how to build droplets
	Lifecycle Lifecycle `json:"lifecycle"`

	// Name of a secret containing a map of multiple environment variables passed to every CFProcess of the app
	EnvSecretName string `json:"envSecretName,omitempty"`

	// CurrentDropletRef provides reference to the droplet currently assigned (active) for the app
	CurrentDropletRef v1.LocalObjectReference `json:"currentDropletRef,omitempty"`
}

// DesiredState defines the desired state of CFApp.
// +kubebuilder:validation:Enum=STOPPED;STARTED
type DesiredState string

// CFAppStatus defines the observed state of CFApp
type CFAppStatus struct {
	// Conditions capture the current status of the App
	Conditions []metav1.Condition `json:"conditions"`

	ObservedDesiredState DesiredState `json:"observedDesiredState"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CFApp is the Schema for the cfapps API
type CFApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFAppSpec   `json:"spec,omitempty"`
	Status CFAppStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFAppList contains a list of CFApp
type CFAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFApp{}, &CFAppList{})
}
