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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DeploymentStatusValueActive    string = "ACTIVE"
	DeploymentStatusValueFinalized string = "FINALIZED"
)

// CFAppSpec defines the desired state of CFApp
type CFAppSpec struct {
	// The mutable, user-friendly name of the app. Unlike metadata.name, the user can change this field.
	// This is more restrictive than CC's app model- to make default route validation errors less likely
	// +kubebuilder:validation:Pattern="^[-\\w]+$"
	DisplayName string `json:"displayName"`

	// The user-requested state of the CFApp. The currently-applied state of the CFApp is in status.ObservedDesiredState.
	// Allowed values are "STARTED", and "STOPPED".
	// +kubebuilder:validation:Enum=STOPPED;STARTED
	DesiredState AppState `json:"desiredState"`

	// Specifies how to build images for the app
	Lifecycle Lifecycle `json:"lifecycle"`

	// The name of a Secret in the same namespace, which contains the environment variables to be set on every one of its running containers (via AppWorkload)
	EnvSecretName string `json:"envSecretName,omitempty"`

	// A reference to the CFBuild currently assigned to the app. The CFBuild must be in the same namespace.
	CurrentDropletRef corev1.LocalObjectReference `json:"currentDropletRef,omitempty"`
}

// AppState defines the desired state of CFApp.
type AppState string

// CFAppStatus defines the observed state of CFApp
type CFAppStatus struct {
	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Deprecated: No longer used
	//+kubebuilder:validation:Optional
	ObservedDesiredState AppState `json:"observedDesiredState"`

	// VCAPServicesSecretName contains the name of the CFApp's VCAP_SERVICES Secret, which should exist in the same namespace
	//+kubebuilder:validation:Optional
	VCAPServicesSecretName string `json:"vcapServicesSecretName"`

	// VCAPApplicationSecretName contains the name of the CFApp's VCAP_APPLICATION Secret, which should exist in the same namespace
	//+kubebuilder:validation:Optional
	VCAPApplicationSecretName string `json:"vcapApplicationSecretName"`

	// ObservedGeneration captures the latest generation of the CFApp that has been reconciled
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	//+kubebuilder:validation:Optional
	ActualState AppState `json:"actualState"`

	// Reference to service credentials secrets to be projected onto the app workload
	// They are in the [servicebinding.io](https://servicebinding.io/spec/core/1.1.0/) format
	//+kubebuilder:validation:Optional
	ServiceBindings []ServiceBinding `json:"serviceBindings,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
//+kubebuilder:printcolumn:name="Created At",type="string",JSONPath=`.metadata.labels.korifi\.cloudfoundry\.org/created_at`
//+kubebuilder:printcolumn:name="Updated At",type="string",JSONPath=`.metadata.labels.korifi\.cloudfoundry\.org/updated_at`
//+kubebuilder:printcolumn:name="State",type=string,JSONPath=`.spec.desiredState`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFApp is the Schema for the cfapps API
type CFApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFAppSpec   `json:"spec,omitempty"`
	Status CFAppStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFAppList contains a list of CFApp
type CFAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFApp{}, &CFAppList{})
}

func (a *CFApp) StatusConditions() *[]metav1.Condition {
	return &a.Status.Conditions
}

func (a CFApp) UniqueName() string {
	return strings.ToLower(a.Spec.DisplayName)
}

func (a CFApp) UniqueValidationErrorMessage() string {
	return fmt.Sprintf("App with the name '%s' already exists.", a.Spec.DisplayName)
}
