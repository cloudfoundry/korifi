// +kubebuilder:validation:Optional
package v1

import (
	corev1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LRP describes an Long Running Process

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=lrp
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=.spec.instances,type=integer,name=Replicas
// +kubebuilder:printcolumn:JSONPath=.status.replicas,type=integer,name=Ready
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type LRP struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LRPSpec   `json:"spec"`
	Status LRPStatus `json:"status"`
}

type LRPSpec struct {
	// +kubebuilder:validation:Required
	GUID        string `json:"GUID"`
	Version     string `json:"version"`
	ProcessType string `json:"processType"`
	AppName     string `json:"appName"`
	AppGUID     string `json:"appGUID"`
	OrgName     string `json:"orgName"`
	OrgGUID     string `json:"orgGUID"`
	SpaceName   string `json:"spaceName"`
	SpaceGUID   string `json:"spaceGUID"`
	// +kubebuilder:validation:Required
	Image           string           `json:"image"`
	Command         []string         `json:"command,omitempty"`
	Sidecars        []Sidecar        `json:"sidecars,omitempty"`
	PrivateRegistry *PrivateRegistry `json:"privateRegistry,omitempty"`
	// deprecated: Env is deprecated. Use Environment instead
	Env         map[string]string `json:"env,omitempty"`
	Environment []corev1.EnvVar   `json:"environment,omitempty"`
	Health      Healthcheck       `json:"health"`
	Ports       []int32           `json:"ports,omitempty"`
	// +kubebuilder:default:=1
	Instances int   `json:"instances"`
	MemoryMB  int64 `json:"memoryMB"`
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Required
	DiskMB int64 `json:"diskMB"`
	// +kubebuilder:validation:Format:=uint8
	CPUWeight              uint8             `json:"cpuWeight"`
	VolumeMounts           []VolumeMount     `json:"volumeMounts,omitempty"`
	UserDefinedAnnotations map[string]string `json:"userDefinedAnnotations,omitempty"`
}

type LRPStatus struct {
	Replicas int32 `json:"replicas"`
}

type Route struct {
	Hostname string `json:"hostname"`
	Port     int32  `json:"port"`
}

type Sidecar struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	Command  []string          `json:"command"`
	MemoryMB int64             `json:"memoryMB"`
	Env      map[string]string `json:"env,omitempty"`
}

type PrivateRegistry struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type VolumeMount struct {
	MountPath string `json:"mountPath"`
	ClaimName string `json:"claimName"`
}

type Healthcheck struct {
	Type     string `json:"type"`
	Port     int32  `json:"port"`
	Endpoint string `json:"endpoint"`
	// +kubebuilder:validation:Format:=uint8
	TimeoutMs uint `json:"timeoutMs"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type LRPList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []LRP `json:"items"`
}
