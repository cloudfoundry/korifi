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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BuilderInfoSpec defines the desired state of BuilderInfo
type BuilderInfoSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// BuilderInfoStatus defines the observed state of BuilderInfo
type BuilderInfoStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Stacks     []BuilderInfoStatusStack     `json:"stacks"`
	Buildpacks []BuilderInfoStatusBuildpack `json:"buildpacks"`
	Conditions []metav1.Condition           `json:"conditions"`
}

type BuilderInfoStatusStack struct {
	Name              string      `json:"name"`
	Description       string      `json:"description"`
	CreationTimestamp metav1.Time `json:"creationTimestamp"`
	UpdatedTimestamp  metav1.Time `json:"updatedTimestamp"`
}

type BuilderInfoStatusBuildpack struct {
	Name              string      `json:"name"`
	Version           string      `json:"version"`
	Stack             string      `json:"stack"`
	CreationTimestamp metav1.Time `json:"creationTimestamp"`
	UpdatedTimestamp  metav1.Time `json:"updatedTimestamp"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=builderinfos
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=`.status.conditions[?(@.type=='Ready')].status`

// BuilderInfo is the Schema for the builderinfos API
type BuilderInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuilderInfoSpec   `json:"spec,omitempty"`
	Status BuilderInfoStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BuilderInfoList contains a list of BuilderInfo
type BuilderInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BuilderInfo `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BuilderInfo{}, &BuilderInfoList{})
}
