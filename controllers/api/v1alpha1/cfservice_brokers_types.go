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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ServiceBrokerRelationships struct {
	Space *ToOneRelationship `json:"space"`
}

type ServiceBroker struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	// +kubebuilder:validation:Optional
	Metadata *Metadata `json:"metadata"`
	// +kubebuilder:validation:Optional
	Relationships *ServiceBrokerRelationships `json:"relationships"`
}

type ServiceBrokerPatch struct {
	Name     *string        `json:"name"`
	URL      *string        `json:"url"`
	Metadata *MetadataPatch `json:"metadata"`
}

func (sb *ServiceBroker) Patch(p ServiceBrokerPatch) {
	if p.Name != nil {
		sb.Name = *p.Name
	}
	if p.URL != nil {
		sb.URL = *p.URL
	}
	if p.Metadata != nil {
		if sb.Metadata == nil {
			sb.Metadata = &Metadata{}
		}
		sb.Metadata.Patch(*p.Metadata)
	}
}

type ServiceBrokerResource struct {
	ServiceBroker
	CFResource
}

type CFServiceBrokerSpec struct {
	ServiceBroker `json:",inline"`
	SecretName    string `json:"secretName"`
}

type CFServiceBrokerStatus struct {
	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.Name`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
type CFServiceBroker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFServiceBrokerSpec   `json:"spec,omitempty"`
	Status CFServiceBrokerStatus `json:"status,omitempty"`
}

func (b CFServiceBroker) UniqueName() string {
	return strings.ToLower(b.Spec.Name)
}

func (b CFServiceBroker) UniqueValidationErrorMessage() string {
	return "Name must be unique"
}

// +kubebuilder:object:root=true
// CFServiceBrokerList contains a list of CFServiceInstance
type CFServiceBrokerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFServiceBroker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFServiceBroker{}, &CFServiceBrokerList{})
}