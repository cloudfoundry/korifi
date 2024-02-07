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

type CFServiceBrokerSpec struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	SecretName string `json:"secretName"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.Name`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
type CFServiceBroker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CFServiceBrokerSpec `json:"spec,omitempty"`
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
