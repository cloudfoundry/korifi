package v1alpha1

import (
	"strings"

	"code.cloudfoundry.org/korifi/model/services"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	UsernameCredentialsKey = "username"
	PasswordCredentialsKey = "password"
)

type CFServiceBrokerSpec struct {
	services.ServiceBroker `json:",inline"`
	Credentials            corev1.LocalObjectReference `json:"credentials"`
}

type CFServiceBrokerStatus struct {
	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration captures the latest generation of the CFServiceBroker that has been reconciled
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ObservedGeneration captures the latest version of the spec.Credentials.Name secret that has been reconciled
	// This will ensure that interested contollers are notified on broker credentials change
	//+kubebuilder:validation:Optional
	CredentialsObservedVersion string `json:"credentialsObservedVersion,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Broker Name",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
type CFServiceBroker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFServiceBrokerSpec   `json:"spec,omitempty"`
	Status CFServiceBrokerStatus `json:"status,omitempty"`
}

func (b CFServiceBroker) UniqueName() string {
	return strings.ToLower(b.Spec.Name)
}

func (b CFServiceBroker) UniqueValidationErrorMessage() string {
	return "Name must be unique"
}

func (b *CFServiceBroker) StatusConditions() *[]metav1.Condition {
	return &b.Status.Conditions
}

// +kubebuilder:object:root=true
type CFServiceBrokerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFServiceBroker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFServiceBroker{}, &CFServiceBrokerList{})
}
