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
	"fmt"
	"strings"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CFOrgFinalizerName = "cfOrg.korifi.cloudfoundry.org"

	CFOrgDisplayNameKey = "korifi.cloudfoundry.org/org-name"
)

// CFOrgSpec defines the desired state of CFOrg
type CFOrgSpec struct {
	// The mutable, user-friendly name of the CFOrg. Unlike metadata.name, the user can change this field.
	// +kubebuilder:validation:Pattern="^[[:alnum:][:punct:][:print:]]+$"
	DisplayName string `json:"displayName"`
}

// CFOrgStatus defines the observed state of CFOrg
type CFOrgStatus struct {
	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	GUID string `json:"guid"`

	// ObservedGeneration captures the latest generation of the CFOrg that has been reconciled
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFOrg is the Schema for the cforgs API
type CFOrg struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFOrgSpec   `json:"spec,omitempty"`
	Status CFOrgStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFOrgList contains a list of CFOrg
type CFOrgList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFOrg `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFOrg{}, &CFOrgList{})
}

func (o CFOrg) UniqueName() string {
	return strings.ToLower(o.Spec.DisplayName)
}

func (o CFOrg) UniqueValidationErrorMessage() string {
	// Note: the cf cli expects the specific text `Organization '.*' already exists.` in the error and ignores the error if it matches it.
	return fmt.Sprintf("Organization '%s' already exists.", o.Spec.DisplayName)
}

func (o *CFOrg) GetStatus() status.NamespaceStatus {
	return &o.Status
}

func (o *CFOrg) StatusConditions() *[]metav1.Condition {
	return &o.Status.Conditions
}

func (s *CFOrgStatus) GetConditions() *[]metav1.Condition {
	return &s.Conditions
}

func (s *CFOrgStatus) SetGUID(guid string) {
	s.GUID = guid
}

func (s *CFOrgStatus) SetObservedGeneration(generation int64) {
	s.ObservedGeneration = generation
}
