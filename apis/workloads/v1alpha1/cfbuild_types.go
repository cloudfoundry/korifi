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

// CFBuildSpec defines the desired state of CFBuild
type CFBuildSpec struct {
	// Specifies the CFPackage associated with this build
	PackageRef v1.LocalObjectReference `json:"packageRef"`
	// Specifies the CFApp associated with this build
	AppRef v1.LocalObjectReference `json:"appRef"`
	// Specifies the buildpacks and stack for the build
	Lifecycle Lifecycle `json:"lifecycle"`
}

// CFBuildStatus defines the observed state of CFBuild
type CFBuildStatus struct {
	DropletRef v1.LocalObjectReference `json:"dropletRef,omitempty"`
	// Conditions capture the current status of the Build
	Conditions []metav1.Condition `json:"conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CFBuild is the Schema for the cfbuilds API
type CFBuild struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFBuildSpec   `json:"spec,omitempty"`
	Status CFBuildStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFBuildList contains a list of CFBuild
type CFBuildList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFBuild `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFBuild{}, &CFBuildList{})
}
