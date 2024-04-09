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

type OrgQuotaRelationships struct {
	// +kubebuilder:validation:Optional
	Organizations ToManyRelationship `json:"organizations"`
}

type DomainQuotas struct {
	// +kubebuilder:validation:Optional
	TotalDomains int64 `json:"total_domains"`
}

type DomainQuotasPatch struct {
	TotalDomains *int64 `json:"total_domains"`
}

func (dq *DomainQuotas) Patch(p DomainQuotasPatch) {
	if p.TotalDomains != nil {
		dq.TotalDomains = *p.TotalDomains
	}
}

type OrgQuota struct {
	Name string `json:"name"`
	// +kubebuilder:validation:Optional
	Apps *AppQuotas `json:"apps"`
	// +kubebuilder:validation:Optional
	Services *ServiceQuotas `json:"services"`
	// +kubebuilder:validation:Optional
	Routes *RouteQuotas `json:"routes"`
	// +kubebuilder:validation:Optional
	Domains *DomainQuotas `json:"domains"`
}

type OrgQuotaPatch struct {
	Name     *string             `json:"name"`
	Apps     *AppQuotasPatch     `json:"apps"`
	Services *ServiceQuotasPatch `json:"services"`
	Routes   *RouteQuotasPatch   `json:"routes"`
	Domains  *DomainQuotasPatch  `json:"domains"`
}

func (oq *OrgQuota) Patch(p OrgQuotaPatch) {
	if p.Name != nil {
		oq.Name = *p.Name
	}
	if p.Apps != nil {
		if oq.Apps == nil {
			oq.Apps = &AppQuotas{}
		}
		oq.Apps.Patch(*p.Apps)
	}
	if p.Services != nil {
		if oq.Services == nil {
			oq.Services = &ServiceQuotas{}
		}
		oq.Services.Patch(*p.Services)
	}
	if p.Routes != nil {
		if oq.Routes == nil {
			oq.Routes = &RouteQuotas{}
		}
		oq.Routes.Patch(*p.Routes)
	}
	if p.Domains != nil {
		if oq.Domains == nil {
			oq.Domains = &DomainQuotas{}
		}
		oq.Domains.Patch(*p.Domains)
	}
}

type OrgQuotaSpec struct {
	OrgQuota `json:",inline"`
}

// OrgQuotaResource
type OrgQuotaResource struct {
	OrgQuota      `json:",inline"`
	CFResource    `json:",inline"`
	Relationships OrgQuotaRelationships `json:"relationships"`
}

//+kubebuilder:object:root=true
//+kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.Name`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`

// CFOrgQuota is the Schema for the cforgs API
type CFOrgQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OrgQuotaSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// CFOrgList contains a list of CFOrg
type CFOrgQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CFOrgQuota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CFOrgQuota{}, &CFOrgQuotaList{})
}
