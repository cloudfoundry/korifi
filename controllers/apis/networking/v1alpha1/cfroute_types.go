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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Destination defines a target for a CFRoute, does not carry meaning outside of a CF context
type Destination struct {
	// GUID is required to support CF V3 Destination endpoints
	GUID string `json:"guid"`
	// Port is optional, defaults to ProcessModel::DEFAULT_HTTP_PORT
	Port int `json:"port,omitempty"`
	// App ref is required, part of the identity of a running process to which traffic may be routed
	// We use a ref because the app must exist
	AppRef v1.LocalObjectReference `json:"appRef"`
	// Process type is required, part of the identity of a running process to which traffic may be routed
	// We use process type instead of processRef because a process of the type may not exist at time of destination creation
	ProcessType string `json:"processType"`
}

// Protocol defines the transport protocol of the route
// +kubebuilder:validation:Enum=http;tcp
type Protocol string

// CFRouteSpec defines the desired state of CFRoute
type CFRouteSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Host is optional, defaults to empty. For cf push default route, uses the name of the app.
	Host string `json:"host,omitempty"`
	// Path is optional, defaults to empty.
	Path string `json:"path,omitempty"`
	// Protocol is optional, defaults to http. Dependent on allow-listed protocols on domain.
	Protocol Protocol `json:"protocol,omitempty"`
	// Domain ref is required, provides base domain name and allowed protocol info.
	DomainRef v1.LocalObjectReference `json:"domainRef"`
	// Destinations are optional, a route can exist independently of being mapped to apps.
	Destinations []Destination `json:"destinations,omitempty"`
}

// CFRouteStatus defines the observed state of CFRoute
type CFRouteStatus struct {
	// FQDN captures the fully-qualified domain name for the route
	FQDN string `json:"fqdn,omitempty"`

	// URI captures the URI (FQDN + path) for the route
	URI string `json:"uri,omitempty"`

	// Conditions capture the current status of the route
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="URI",type=string,JSONPath=`.status.uri`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// CFRoute is the Schema for the cfroutes API
type CFRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CFRouteSpec   `json:"spec,omitempty"`
	Status CFRouteStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CFRouteList contains a list of CFRoute
type CFRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFRoute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFRoute{}, &CFRouteList{})
}
