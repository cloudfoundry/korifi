/*
Copyright 2022.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AppWorkloadSpec defines the desired state of AppWorkload
type AppWorkloadSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	GUID        string `json:"GUID"`
	Version     string `json:"version"`
	AppGUID     string `json:"appGUID"`
	ProcessType string `json:"processType"`
	Image       string `json:"image"`

	Command        []string        `json:"command,omitempty"`
	Env            []corev1.EnvVar `json:"env,omitempty"`
	StartupProbe   *corev1.Probe   `json:"startupProbe,omitempty"`
	LivenessProbe  *corev1.Probe   `json:"livenessProbe,omitempty"`
	ReadinessProbe *corev1.Probe   `json:"readinessProbe,omitempty"`
	Ports          []int32         `json:"ports,omitempty"`

	// +kubebuilder:default:=1
	Instances int32 `json:"instances"`

	// The name of the runner that should reconcile this AppWorkload resource and execute running its instances
	// +kubebuilder:validation:Required
	RunnerName string `json:"runnerName"`

	// +kubebuilder:validation:Optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// AppWorkloadStatus defines the observed state of AppWorkload
type AppWorkloadStatus struct {
	// Conditions capture the current status of the observed generation of the AppWorkload
	Conditions []metav1.Condition `json:"conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AppWorkload is the Schema for the appworkloads API
type AppWorkload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppWorkloadSpec   `json:"spec,omitempty"`
	Status AppWorkloadStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AppWorkloadList contains a list of AppWorkload
type AppWorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppWorkload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AppWorkload{}, &AppWorkloadList{})
}
