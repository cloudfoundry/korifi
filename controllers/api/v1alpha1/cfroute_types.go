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
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Deprecated. Used for removing leftover finalizers
	CFRouteFinalizerName = "cfRoute.korifi.cloudfoundry.org"

	DestinationAppGUIDLabelPrefix = "korifi.cloudfoundry.org/destination-app-guid-"
	CFRouteIsUnmappedLabelKey     = "korifi.cloudfoundry.org/unmapped"
)

// Destination defines a target for a CFRoute, does not carry meaning outside of a CF context
type Destination struct {
	// A unique identifier for this route destination. Required to support CF V3 Destination endpoints
	GUID string `json:"guid"`
	// The port to use for the destination. Port is optional, and defaults to
	// either the droplet port, or 8080 if no ports are available in the
	// droplet
	//+kubebuilder:validation:Optional
	Port *int32 `json:"port,omitempty"`
	// A required reference to the CFApp that will receive traffic. The CFApp must be in the same namespace
	AppRef v1.LocalObjectReference `json:"appRef"`
	// The process type on the CFApp app which will receive traffic
	ProcessType string `json:"processType"`
	// Protocol is optional, when set must be "http1"
	// +kubebuilder:validation:Enum=http1
	//+kubebuilder:validation:Optional
	Protocol *string `json:"protocol,omitempty"`
}

// Protocol defines the transport protocol of the route
// +kubebuilder:validation:Enum=http;tcp
type Protocol string

// CFRouteSpec defines the desired state of CFRoute
type CFRouteSpec struct {
	// The subdomain of the route within the domain. Host is optional and defaults to empty.
	// When the host is empty, then the name of the app will be used
	Host string `json:"host,omitempty"`
	// Path is optional, defaults to empty
	Path string `json:"path,omitempty"`
	// Protocol is optional and defaults to http. Currently only http is supported
	Protocol Protocol `json:"protocol,omitempty"`
	// A reference to the CFDomain this CFRoute is assigned to, including name and namespace
	DomainRef v1.ObjectReference `json:"domainRef"`
	// Destinations are optional. A route can exist without any destinations, independently of any CFApps
	Destinations []Destination `json:"destinations,omitempty"`
}

// CFRouteStatus defines the observed state of CFRoute
type CFRouteStatus struct {
	// The fully-qualified domain name for the route
	FQDN string `json:"fqdn,omitempty"`

	// The URI (FQDN + path) for the route
	URI string `json:"uri,omitempty"`

	// The observed state of the destinations. This is mainly used to record the target port of the underlying service
	Destinations []Destination `json:"destinations,omitempty"`

	//+kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration captures the latest generation of the CFRoute that has been reconciled
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="URI",type=string,JSONPath=`.status.uri`
//+kubebuilder:printcolumn:name="Created At",type="string",JSONPath=`.metadata.labels.korifi\.cloudfoundry\.org/created_at`
//+kubebuilder:printcolumn:name="Updated At",type="string",JSONPath=`.metadata.labels.korifi\.cloudfoundry\.org/updated_at`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFRoute is the Schema for the cfroutes API
type CFRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFRouteSpec   `json:"spec,omitempty"`
	Status CFRouteStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CFRouteList contains a list of CFRoute
type CFRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFRoute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFRoute{}, &CFRouteList{})
}

func (r CFRoute) UniqueName() string {
	return strings.Join([]string{strings.ToLower(r.Spec.Host), r.Spec.DomainRef.Namespace, r.Spec.DomainRef.Name, r.Spec.Path}, "::")
}

func (r CFRoute) UniqueValidationErrorMessage() string {
	pathDetails := ""

	if r.Spec.Path != "" {
		pathDetails = fmt.Sprintf(" and path '%s'", r.Spec.Path)
	}

	return fmt.Sprintf("Route already exists with host '%s'%s for domain '%s'.", r.Spec.Host, pathDetails, r.Status.FQDN)
}

func (r *CFRoute) StatusConditions() *[]metav1.Condition {
	return &r.Status.Conditions
}
