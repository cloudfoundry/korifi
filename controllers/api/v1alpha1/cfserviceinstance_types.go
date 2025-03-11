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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	UserProvidedType = "user-provided"
	ManagedType      = "managed"

	CFServiceInstanceFinalizerName = "cfServiceInstance.korifi.cloudfoundry.org"

	ProvisioningFailedCondition   = "ProvisioningFailed"
	DeprovisioningFailedCondition = "DeprovisioningFailed"
)

// CFServiceInstanceSpec defines the desired state of CFServiceInstance
type CFServiceInstanceSpec struct {
	// The mutable, user-friendly name of the service instance. Unlike metadata.name, the user can change this field
	DisplayName string `json:"displayName"`

	// Name of a secret containing the service credentials. The Secret must be in the same namespace
	SecretName string `json:"secretName"`

	// Type of the Service Instance. Must be `user-provided` or `managed`
	Type InstanceType `json:"type"`

	// Service label to use when adding this instance to VCAP_SERVICES. If not
	// set, the service instance Type would be used. For managed services the
	// value is defaulted to the offering name
	// +optional
	ServiceLabel *string `json:"serviceLabel,omitempty"`

	// Tags are used by apps to identify service instances
	Tags []string `json:"tags,omitempty"`

	PlanGUID string `json:"planGuid"`

	Parameters corev1.LocalObjectReference `json:"parameters,omitempty"`
}

// InstanceType defines the type of the Service Instance
// +kubebuilder:validation:Enum=user-provided;managed
type InstanceType string

// CFServiceInstanceStatus defines the observed state of CFServiceInstance
type CFServiceInstanceStatus struct {
	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration captures the latest generation of the CFServiceInstance that has been reconciled
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// A reference to the service instance secret containing the credentials
	// (derived from spec.secretName).
	//+kubebuilder:validation:Optional
	Credentials corev1.LocalObjectReference `json:"credentials"`

	// ObservedGeneration captures the latest version of the spec.secretName that has been reconciled
	// This will ensure that interested contollers are notified on instance credentials change
	//+kubebuilder:validation:Optional
	CredentialsObservedVersion string `json:"credentialsObservedVersion,omitempty"`

	//+kubebuilder:validation:Optional
	LastOperation LastOperation `json:"lastOperation"`

	// The service instance maintenance info. Only makes seense for managed service instances
	//+kubebuilder:validation:Optional
	MaintenanceInfo MaintenanceInfo `json:"maintenanceInfo"`

	// True if there is an upgrade available for for the service instance (i.e. the plan has a new version). Only makes seense for managed service instances
	//+kubebuilder:validation:Optional
	UpgradeAvailable bool `json:"upgradeAvailable"`
}

type LastOperation struct {
	// +kubebuilder:validation:Enum=create;update;delete
	Type string `json:"type"`
	// +kubebuilder:validation:Enum=initial;in progress;succeeded;failed
	State string `json:"state"`

	//+kubebuilder:validation:Optional
	Description string `json:"description"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFServiceInstance is the Schema for the cfserviceinstances API
type CFServiceInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CFServiceInstanceSpec `json:"spec,omitempty"`

	Status CFServiceInstanceStatus `json:"status,omitempty"`
}

func (si CFServiceInstance) UniqueName() string {
	return si.Spec.DisplayName
}

func (si CFServiceInstance) UniqueValidationErrorMessage() string {
	// Note: the cf cli expects the specific text 'The service instance name is taken'
	return fmt.Sprintf("The service instance name is taken: %s", si.Spec.DisplayName)
}

func (si *CFServiceInstance) StatusConditions() *[]metav1.Condition {
	return &si.Status.Conditions
}

//+kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFServiceInstanceList contains a list of CFServiceInstance
type CFServiceInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFServiceInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFServiceInstance{}, &CFServiceInstanceList{})
}
