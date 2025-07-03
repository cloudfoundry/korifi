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
	CFSpaceFinalizerName = "cfSpace.korifi.cloudfoundry.org"

	CFSpaceDisplayNameKey = "korifi.cloudfoundry.org/space-name"
)

// CFSpaceSpec defines the desired state of CFSpace
type CFSpaceSpec struct {
	// The mutable, user-friendly name of the space. Unlike metadata.name, the user can change this field
	// +kubebuilder:validation:Pattern="^[[:alnum:][:punct:][:print:]]+$"
	DisplayName string `json:"displayName"`
}

// CFSpaceStatus defines the observed state of CFSpace
type CFSpaceStatus struct {
	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	GUID string `json:"guid"`

	// ObservedGeneration captures the latest generation of the CFSpace that has been reconciled
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Created At",type="string",JSONPath=`.metadata.labels.korifi\.cloudfoundry\.org/created_at`
//+kubebuilder:printcolumn:name="Updated At",type="string",JSONPath=`.metadata.labels.korifi\.cloudfoundry\.org/updated_at`
//+kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFSpace is the Schema for the cfspaces API
type CFSpace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFSpaceSpec   `json:"spec,omitempty"`
	Status CFSpaceStatus `json:"status,omitempty"`
}

func (s CFSpace) UniqueValidationErrorMessage() string {
	// Note: the cf cli expects the specific text `Name must be unique per organization` in the error and ignores the error if it matches it.
	return fmt.Sprintf("Space '%s' already exists. Name must be unique per organization.", s.Spec.DisplayName)
}

func (s CFSpace) UniqueName() string {
	return strings.ToLower(s.Spec.DisplayName)
}

//+kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFSpaceList contains a list of CFSpace
type CFSpaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFSpace `json:"items"`
}

func (s *CFSpace) StatusConditions() *[]metav1.Condition {
	return &s.Status.Conditions
}

func (s *CFSpace) GetStatus() status.NamespaceStatus {
	return &s.Status
}

func (s *CFSpaceStatus) GetConditions() *[]metav1.Condition {
	return &s.Conditions
}

func (s *CFSpaceStatus) SetGUID(guid string) {
	s.GUID = guid
}

func (s *CFSpaceStatus) SetObservedGeneration(generation int64) {
	s.ObservedGeneration = generation
}

func init() {
	SchemeBuilder.Register(&CFSpace{}, &CFSpaceList{})
}
