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

const (
	CFPackageFinalizerName = "korifi.cloudfoundry.org/cfPackageController"
)

// CFPackageSpec defines the desired state of CFPackage
type CFPackageSpec struct {
	// The package type. Allowed values are "bits" and "docker".
	Type PackageType `json:"type"`

	// Reference the CFApp that owns this package. The CFApp must be in the same namespace.
	AppRef v1.LocalObjectReference `json:"appRef"`

	// Contains the details for the source image (e.g. its bits)
	Source PackageSource `json:"source,omitempty"`
}

// PackageType used to enum the inputs to package.type
// +kubebuilder:validation:Enum=bits;docker
type PackageType string

type PackageSource struct {
	// registry (i.e an OCI image in a registry that contains application source)
	Registry Registry `json:"registry"`
}

// CFPackageStatus defines the observed state of CFPackage
type CFPackageStatus struct {
	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration captures the latest generation of the CFPackage that has been reconciled
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="AppGUID",type=string,JSONPath=`.spec.appRef.name`
//+kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=='Ready')].status`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// CFPackage is the Schema for the cfpackages API
type CFPackage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFPackageSpec   `json:"spec,omitempty"`
	Status CFPackageStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFPackageList contains a list of CFPackage
type CFPackageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFPackage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFPackage{}, &CFPackageList{})
}

func (p CFPackage) StatusConditions() []metav1.Condition {
	return p.Status.Conditions
}
