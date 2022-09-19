/*
Copyright 2022.

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

// BuildWorkloadSpec defines the desired state of BuildWorkload
type BuildWorkloadSpec struct {
	// this is a reference to the object requesting the build
	BuildRef RequiredLocalObjectReference `json:"buildRef"`

	// Source contains the details for the source image(bits)
	Source PackageSource `json:"source,omitempty"`

	Buildpacks []string `json:"buildpacks,omitempty"`

	Env []v1.EnvVar `json:"env,omitempty"`

	Services []v1.ObjectReference `json:"services,omitempty"`

	BuilderName string `json:"builderName"`
}

// BuildWorkloadStatus defines the observed state of BuildWorkload
type BuildWorkloadStatus struct {
	// Conditions capture the current status of the observed generation of the BuildWorkload
	Conditions []metav1.Condition `json:"conditions"`

	Droplet *BuildDropletStatus `json:"droplet,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// BuildWorkload is the Schema for the buildworkloads API
type BuildWorkload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuildWorkloadSpec   `json:"spec,omitempty"`
	Status BuildWorkloadStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BuildWorkloadList contains a list of BuildWorkload
type BuildWorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BuildWorkload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BuildWorkload{}, &BuildWorkloadList{})
}
