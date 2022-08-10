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
	GUID             string                        `json:"GUID"`
	Version          string                        `json:"version"`
	AppGUID          string                        `json:"appGUID"`
	ProcessType      string                        `json:"processType"`
	Image            string                        `json:"image"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets"`
	Command          []string                      `json:"command,omitempty"`
	Env              []corev1.EnvVar               `json:"env,omitempty"`
	Health           Healthcheck                   `json:"health"`
	Ports            []int32                       `json:"ports,omitempty"`
	// +kubebuilder:default:=1
	Instances int32 `json:"instances"`
	MemoryMiB int64 `json:"memoryMiB"`
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Required
	DiskMiB       int64 `json:"diskMiB"`
	CPUMillicores int64 `json:"cpuMillicores"`
	// +kubebuilder:validation:Required
	ReconcilerName string `json:"reconcilerName"`
}

type Healthcheck struct {
	Type     string `json:"type"`
	Port     int32  `json:"port"`
	Endpoint string `json:"endpoint"`
	// +kubebuilder:validation:Format:=uint8
	TimeoutMs uint `json:"timeoutMs"`
}

// AppWorkloadStatus defines the observed state of AppWorkload
type AppWorkloadStatus struct {
	ReadyReplicas int32 `json:"readyReplicas"`
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
