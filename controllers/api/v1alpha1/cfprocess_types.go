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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ProcessTypeWeb = "web"
)

// CFProcessSpec defines the desired state of CFProcess
type CFProcessSpec struct {
	// A reference to the CFApp that owns this CFProcess. The CFApp must be in the same namespace.
	AppRef v1.LocalObjectReference `json:"appRef"`

	// The name of the process within the CFApp (e.g. "web")
	ProcessType string `json:"processType"`

	// Command string used to run this process on the app image. This is analogous to command in k8s and ENTRYPOINT in Docker
	Command string `json:"command,omitempty"`

	// The default command for this process as defined by the build. This field is ignored when the Command field is set
	DetectedCommand string `json:"detectedCommand,omitempty"`

	// Used to build the Liveness and Readiness Probes for the process' AppWorkload.
	HealthCheck HealthCheck `json:"healthCheck"`

	// The desired number of replicas to deploy
	DesiredInstances *int32 `json:"desiredInstances,omitempty"`

	// The memory limit in MiB
	MemoryMB int64 `json:"memoryMB"`

	// The disk limit in MiB
	DiskQuotaMB int64 `json:"diskQuotaMB"`

	// The ports to expose
	// Deprecated: No longer used
	// +kubebuilder:validation:Optional
	Ports []int32 `json:"ports,omitempty"`
}

type HealthCheck struct {
	// The type of Health Check the App process will use
	// Valid values are "http", "port", and "process".
	// For processType "web", the default type is "port". For all other processes, the default is "process".
	Type HealthCheckType `json:"type"`

	// The input parameters for the liveness and readiness probes in kubernetes
	Data HealthCheckData `json:"data"`
}

// HealthCheckType used to ensure illegal HealthCheckTypes are not passed
// +kubebuilder:validation:Enum=http;port;process;""
type HealthCheckType string

// HealthCheckData used to pass through input parameters to liveness probe
type HealthCheckData struct {
	// The http endpoint to use with "http" healthchecks
	HTTPEndpoint string `json:"httpEndpoint,omitempty"`

	InvocationTimeoutSeconds int32 `json:"invocationTimeoutSeconds"`
	TimeoutSeconds           int32 `json:"timeoutSeconds"`
}

// CFProcessStatus defines the observed state of CFProcess
type CFProcessStatus struct {
	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration captures the latest generation of the CFProcess that has been reconciled
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	//+kubebuilder:validation:Optional
	ActualInstances int32 `json:"actualInstances"`

	//+kubebuilder:validation:Optional
	InstancesStatus map[string]InstanceStatus `json:"instancesStatus"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFProcess is the Schema for the cfprocesses API
type CFProcess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFProcessSpec   `json:"spec,omitempty"`
	Status CFProcessStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFProcessList contains a list of CFProcess
type CFProcessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFProcess `json:"items"`
}

func (p *CFProcess) StatusConditions() *[]metav1.Condition {
	return &p.Status.Conditions
}

func init() {
	SchemeBuilder.Register(&CFProcess{}, &CFProcessList{})
}
