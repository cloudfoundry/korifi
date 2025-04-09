package v1alpha1

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ProtocolTCP = "tcp"
	ProtocolUDP = "udp"
	ProtocolALL = "all"
)

type SecurityGroupRule struct {
	Protocol    string `json:"protocol"`
	Destination string `json:"destination"`
	Ports       string `json:"ports,omitempty"`
	Type        int    `json:"type,omitempty"`
	Code        int    `json:"code,omitempty"`
	Description string `json:"description,omitempty"`
	Log         bool   `json:"log,omitempty"`
}

type SecurityGroupWorkloads struct {
	Running bool `json:"running"`
	Staging bool `json:"staging"`
}

type CFSecurityGroupSpec struct {
	DisplayName string              `json:"displayName"`
	Rules       []SecurityGroupRule `json:"rules"`
	// //+kubebuilder:validation:Optional
	Spaces map[string]SecurityGroupWorkloads `json:"spaces,omitempty"`
	// //+kubebuilder:validation:Optional
	GloballyEnabled SecurityGroupWorkloads `json:"globallyEnabled,omitempty"`
}

type CFSecurityGroupStatus struct {
	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:subresource:status
//+kubebuilder:object:root=true
//+kubebuilder:printcolumn:name="DisplayName",type=string,JSONPath=`.spec.displayName`
//+k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CFSecurityGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CFSecurityGroupSpec `json:"spec,omitempty"`

	Status CFSecurityGroupStatus `json:"status,omitempty"`
}

func (s CFSecurityGroup) UniqueName() string {
	return strings.ToLower(s.Spec.DisplayName)
}

func (b CFSecurityGroup) UniqueValidationErrorMessage() string {
	return fmt.Sprintf("Security group with name '%s' already exists.", b.Spec.DisplayName)
}

func (g *CFSecurityGroup) StatusConditions() *[]metav1.Condition {
	return &g.Status.Conditions
}

//+kubebuilder:object:root=true
//+k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CFSecurityGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFSecurityGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFSecurityGroup{}, &CFSecurityGroupList{})
}
