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

// CFPackageSpec defines the desired state of CFPackage
type CFPackageSpec struct {
	// Type specifies the package type
	// Valid values are:
	// "bits": package to upload source code
	Type PackageType `json:"type"`

	// AppRef reference to the CFApp that owns this package
	AppRef ResourceReference `json:"appRef"`

	// Source contains the details for the source image(bits)
	Source PackageSource `json:"source,omitempty"`
}

// PackageType used to enum the inputs to package.type
// +kubebuilder:validation:Enum=bits
type PackageType string

type PackageSource struct {
	// registry ( Source code is an OCI image in a registry that contains application source)
	Registry Registry `json:"registry"`
}

// CFPackageStatus defines the observed state of CFPackage
type CFPackageStatus struct {
	// Conditions capture the current status of the Package
	Conditions []metav1.Condition `json:"conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
