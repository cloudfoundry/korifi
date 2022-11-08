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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TaskInitializedConditionType = "Initialized"
	TaskStartedConditionType     = "Started"
	TaskSucceededConditionType   = "Succeeded"
	TaskFailedConditionType      = "Failed"
	TaskCanceledConditionType    = "Canceled"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CFTaskSpec defines the desired state of CFTask
type CFTaskSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The command used to start the task process
	Command string `json:"command,omitempty"`
	// A reference to the CFApp containing the code or script for this CFTask
	AppRef corev1.LocalObjectReference `json:"appRef,omitempty"`
	// A boolean describing whether the CFTask has been canceled
	// +optional
	Canceled bool `json:"canceled"`
}

// CFTaskStatus defines the observed state of CFTask
type CFTaskStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +optional
	Conditions []metav1.Condition `json:"conditions"`

	// +optional
	SequenceID int64 `json:"sequenceId"`
	// +optional
	MemoryMB int64 `json:"memoryMB"`
	// +optional
	DiskQuotaMB int64 `json:"diskQuotaMB"`
	// +optional
	DropletRef corev1.LocalObjectReference `json:"dropletRef"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CFTask is the Schema for the cftasks API
type CFTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFTaskSpec   `json:"spec,omitempty"`
	Status CFTaskStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFTaskList contains a list of CFTask
type CFTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFTask `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFTask{}, &CFTaskList{})
}

func (t CFTask) StatusConditions() []metav1.Condition {
	return t.Status.Conditions
}
