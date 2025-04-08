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

// AppWorkloadSpec defines the desired state of AppWorkload
type AppWorkloadSpec struct {
	// +kubebuilder:validation:Required
	GUID        string `json:"GUID"`
	Version     string `json:"version"`
	AppGUID     string `json:"appGUID"`
	ProcessType string `json:"processType"`
	Image       string `json:"image"`

	// An optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec.
	// If specified, these secrets will be passed to individual puller implementations for them to use.
	// More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	// +kubebuilder:validation:Optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

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

	// Reference to service credentials secrets to be projected onto the app workload
	// They are in the [servicebinding.io](https://servicebinding.io/spec/core/1.1.0/) format
	Services []ServiceBinding `json:"services,omitempty"`
}

// AppWorkloadStatus defines the observed state of AppWorkload
type AppWorkloadStatus struct {
	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration captures the latest generation of the AppWorkload that has been reconciled
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	//+kubebuilder:validation:Optional
	ActualInstances int32 `json:"actualInstances"`

	//+kubebuilder:validation:Optional
	InstancesStatus map[string]InstanceStatus `json:"instancesStatus"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AppWorkload is the Schema for the appworkloads API
type AppWorkload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppWorkloadSpec   `json:"spec,omitempty"`
	Status AppWorkloadStatus `json:"status,omitempty"`
}

func (w *AppWorkload) StatusConditions() *[]metav1.Condition {
	return &w.Status.Conditions
}

//+kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AppWorkloadList contains a list of AppWorkload
type AppWorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppWorkload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AppWorkload{}, &AppWorkloadList{})
}
