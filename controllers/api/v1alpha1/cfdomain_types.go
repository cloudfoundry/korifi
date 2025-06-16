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
	CFDomainFinalizerName = "cfDomain.korifi.cloudfoundry.org"
)

// CFDomainSpec defines the desired state of CFDomain
type CFDomainSpec struct {
	// The domain name. It is required and must conform to RFC 1035
	Name string `json:"name"`
}

// CFDomainStatus defines the observed state of CFDomain
type CFDomainStatus struct {
	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration captures the latest generation of the CFDomain that has been reconciled
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced
//+kubebuilder:printcolumn:name="Domain Name",type=string,JSONPath=`.spec.name`
//+kubebuilder:printcolumn:name="Created At",type="string",JSONPath=`.metadata.labels.korifi\.cloudfoundry\.org/created_at`
//+kubebuilder:printcolumn:name="Updated At",type="string",JSONPath=`.metadata.labels.korifi\.cloudfoundry\.org/updated_at`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFDomain is the Schema for the cfdomains API
type CFDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFDomainSpec   `json:"spec,omitempty"`
	Status CFDomainStatus `json:"status,omitempty"`
}

func (d *CFDomain) StatusConditions() *[]metav1.Condition {
	return &d.Status.Conditions
}

//+kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFDomainList contains a list of CFDomain
type CFDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFDomain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFDomain{}, &CFDomainList{})
}
