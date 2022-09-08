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

const (
	OrgNameLabel = "cloudfoundry.org/org-name"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CFOrgSpec defines the desired state of CFOrg
type CFOrgSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// DisplayName that is displayed to users
	// +kubebuilder:validation:Pattern="^[-\\w]+$"
	DisplayName string `json:"displayName"`
}

// CFOrgStatus defines the observed state of CFOrg
type CFOrgStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []metav1.Condition `json:"conditions"`

	// +optional
	GUID string `json:"guid"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// CFOrg is the Schema for the cforgs API
type CFOrg struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFOrgSpec   `json:"spec,omitempty"`
	Status CFOrgStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFOrgList contains a list of CFOrg
type CFOrgList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFOrg `json:"items"`
}

func (o *CFOrg) StatusConditions() *[]metav1.Condition {
	return &o.Status.Conditions
}

func init() {
	SchemeBuilder.Register(&CFOrg{}, &CFOrgList{})
}
