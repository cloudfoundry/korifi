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

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ProcessTypeWeb    = "web"
	processNamePrefix = "cf-proc"
)

// CFProcessSpec defines the desired state of CFProcess
type CFProcessSpec struct {
	// Specifies the App that owns this process
	AppRef v1.LocalObjectReference `json:"appRef"`

	// Specifies the name of the process in the App
	ProcessType string `json:"processType"`

	// Specifies the Command(k8s) ENTRYPOINT(Docker) of the Process
	Command string `json:"command,omitempty"`

	// Specifies the Liveness Probe (k8s) details of the Process
	HealthCheck HealthCheck `json:"healthCheck"`

	// Specifies the desired number of Process replicas to deploy
	DesiredInstances int `json:"desiredInstances"`

	// Specifies the Process memory limit in MiB
	MemoryMB int64 `json:"memoryMB"`

	// Specifies the Process disk limit in MiB
	DiskQuotaMB int64 `json:"diskQuotaMB"`

	// Specifies the Process ports to expose
	Ports []int32 `json:"ports"`
}

type HealthCheck struct {
	// Specifies the type of Health Check the App process will use
	// Valid values are:
	// "http": http health check
	// "port": TCP health check
	// "process" (default): checks if process for start command is still alive
	Type HealthCheckType `json:"type"`

	// Specifies the input parameters for the liveness probe/health check in kubernetes
	Data HealthCheckData `json:"data"`
}

// HealthCheckType used to ensure illegal HealthCheckTypes are not passed
// +kubebuilder:validation:Enum=http;port;process
type HealthCheckType string

// HealthCheckData used to pass through input parameters to liveness probe
type HealthCheckData struct {
	// HTTPEndpoint is only used by an "http" liveness probe
	HTTPEndpoint string `json:"httpEndpoint,omitempty"`

	InvocationTimeoutSeconds int64 `json:"invocationTimeoutSeconds"`
	TimeoutSeconds           int64 `json:"timeoutSeconds"`
}

// CFProcessStatus defines the observed state of CFProcess
type CFProcessStatus struct {
	// RunningInstances captures the actual number of Process replicas
	RunningInstances int `json:"runningInstances"`
	// Conditions capture the current status of the Process
	Conditions []metav1.Condition `json:"conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CFProcess is the Schema for the cfprocesses API
type CFProcess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFProcessSpec   `json:"spec,omitempty"`
	Status CFProcessStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFProcessList contains a list of CFProcess
type CFProcessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFProcess `json:"items"`
}

func (r *CFProcess) SetStableName(appGUID string) {
	r.Name = strings.Join([]string{processNamePrefix, appGUID, r.Spec.ProcessType}, "-")
	if r.Labels == nil {
		r.Labels = map[string]string{}
	}
	r.Labels[CFProcessGUIDLabelKey] = r.Name
}

func init() {
	SchemeBuilder.Register(&CFProcess{}, &CFProcessList{})
}
