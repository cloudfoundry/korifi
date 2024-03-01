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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SpaceQuotaRelationships struct {
	// +kubebuilder:validation:Optional
	Organization ToOneRelationship `json:"organization"`
	// +kubebuilder:validation:Optional
	Spaces ToManyRelationship `json:"spaces"`
}

// CFOrgQuotaSpec defines the desired state of CFOrgQuota
type SpaceQuota struct {
	Name string `json:"name"`
	// +kubebuilder:validation:Optional
	Apps *AppQuotas `json:"apps"`
	// +kubebuilder:validation:Optional
	Services *ServiceQuotas `json:"services"`
	// +kubebuilder:validation:Optional
	Routes *RouteQuotas `json:"routes"`
	// +kubebuilder:validation:Optional
	Relationships SpaceQuotaRelationships `json:"relationships"`
}

// CFOrgQuotaSpec defines the desired state of CFOrgQuota
type SpaceQuotaPatch struct {
	Name     *string             `json:"name"`
	Apps     *AppQuotasPatch     `json:"apps"`
	Services *ServiceQuotasPatch `json:"services"`
	Routes   *RouteQuotasPatch   `json:"routes"`
}

func (sq *SpaceQuota) Patch(p SpaceQuotaPatch) {
	if p.Name != nil {
		sq.Name = *p.Name
	}
	if p.Apps != nil {
		if sq.Apps == nil {
			sq.Apps = &AppQuotas{}
		}
		sq.Apps.Patch(*p.Apps)
	}
	if p.Services != nil {
		if sq.Services == nil {
			sq.Services = &ServiceQuotas{}
		}
		sq.Services.Patch(*p.Services)
	}
	if p.Routes != nil {
		if sq.Routes == nil {
			sq.Routes = &RouteQuotas{}
		}
		sq.Routes.Patch(*p.Routes)
	}
}

// OrgQuotaResource
type SpaceQuotaResource struct {
	SpaceQuota `json:",inline"`
	CFResource `json:",inline"`
}

type SpaceQuotaSpec struct {
	SpaceQuota `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.Name`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// CFSpaceQuota is the Schema for the cfspacequota API
type CFSpaceQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec SpaceQuotaSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// CFOrgList contains a list of CFOrg
type CFSpaceQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFSpaceQuota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFSpaceQuota{}, &CFSpaceQuotaList{})
}
