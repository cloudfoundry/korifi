package v1

import (
	corev1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=task
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=.status.execution_status,type=string,name=State
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Task describes a short-lived job running alongside an LRP
type Task struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TaskSpec   `json:"spec"`
	Status TaskStatus `json:"status"`
}

type TaskSpec struct {
	// +kubebuilder:validation:Required
	GUID string `json:"GUID"`
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	Image           string           `json:"image"`
	PrivateRegistry *PrivateRegistry `json:"privateRegistry,omitempty"`
	// deprecated: Env is deprecated. Use Environment instead
	Env         map[string]string `json:"env,omitempty"`
	Environment []corev1.EnvVar   `json:"environment,omitempty"`
	// +kubebuilder:validation:Required
	Command   []string `json:"command,omitempty"`
	AppName   string   `json:"appName"`
	AppGUID   string   `json:"appGUID"`
	OrgName   string   `json:"orgName"`
	OrgGUID   string   `json:"orgGUID"`
	SpaceName string   `json:"spaceName"`
	SpaceGUID string   `json:"spaceGUID"`
	MemoryMB  int64    `json:"memoryMB"`
	DiskMB    int64    `json:"diskMB"`
	// +kubebuilder:validation:Format:=uint8
	CPUWeight uint8 `json:"cpuWeight"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type TaskList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []Task `json:"items"`
}

type ExecutionStatus string

const (
	TaskStarting  ExecutionStatus = "starting"
	TaskRunning   ExecutionStatus = "running"
	TaskSucceeded ExecutionStatus = "succeeded"
	TaskFailed    ExecutionStatus = "failed"
)

type TaskStatus struct {
	StartTime *meta_v1.Time `json:"start_time"`
	EndTime   *meta_v1.Time `json:"end_time"`
	// +kubebuilder:validation:Enum=starting;running;succeeded;failed
	// +kubebuilder:default=starting
	ExecutionStatus ExecutionStatus `json:"execution_status"`
}
