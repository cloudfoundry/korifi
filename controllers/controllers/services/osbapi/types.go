package osbapi

import (
	"code.cloudfoundry.org/korifi/model/services"
)

type Broker struct {
	URL      string
	Username string
	Password string
}

type Catalog struct {
	Services []Service `json:"services"`
}

type Service struct {
	services.BrokerCatalogFeatures `json:",inline"`
	ID                             string         `json:"id"`
	Name                           string         `json:"name"`
	Description                    string         `json:"description"`
	Tags                           []string       `json:"tags"`
	Requires                       []string       `json:"requires"`
	Metadata                       map[string]any `json:"metadata"`
	DashboardClient                struct {
		Id          string `json:"id"`
		Secret      string `json:"secret"`
		RedirectUri string `json:"redirect_url"`
	} `json:"dashboard_client"`
	Plans []Plan `json:"plans"`
}

type Plan struct {
	ID               string                      `json:"id"`
	Name             string                      `json:"name"`
	Description      string                      `json:"description"`
	Metadata         map[string]any              `json:"metadata"`
	Free             bool                        `json:"free"`
	Bindable         bool                        `json:"bindable"`
	BindingRotatable bool                        `json:"binding_rotatable"`
	PlanUpdateable   bool                        `json:"plan_updateable"`
	Schemas          services.ServicePlanSchemas `json:"schemas"`
	MaintenanceInfo  services.MaintenanceInfo    `json:"maintenance_info"`
}

type ProvisionPayload struct {
	InstanceID string
	ProvisionRequest
}

type ProvisionRequest struct {
	ServiceId  string         `json:"service_id"`
	PlanID     string         `json:"plan_id"`
	SpaceGUID  string         `json:"space_guid"`
	OrgGUID    string         `json:"organization_guid"`
	Parameters map[string]any `json:"parameters"`
}

type ProvisionResponse struct {
	IsAsync   bool
	Operation string `json:"operation,omitempty"`
}

type GetBindingRequest struct {
	InstanceID string
	BindingID  string
	ServiceId  string
	PlanID     string
}

type GetBindingResponse struct {
	Credentials map[string]any `json:"credentials"`
}

type GetInstanceLastOperationRequest struct {
	InstanceID string
	GetLastOperationRequestParameters
}

type GetBindingLastOperationRequest struct {
	InstanceID string
	BindingID  string
	GetLastOperationRequestParameters
}

type GetLastOperationRequestParameters struct {
	ServiceId string
	PlanID    string
	Operation string
}
type DeprovisionPayload struct {
	ID string
	DeprovisionRequestParamaters
}

type DeprovisionRequestParamaters struct {
	ServiceId string `json:"service_id"`
	PlanID    string `json:"plan_id"`
}

type BindRequest struct {
	ServiceId    string         `json:"service_id"`
	PlanID       string         `json:"plan_id"`
	AppGUID      string         `json:"app_guid"`
	BindResource BindResource   `json:"bind_resource"`
	Parameters   map[string]any `json:"parameters"`
}

type BindPayload struct {
	BindingID  string
	InstanceID string
	BindRequest
}

type BindResponse struct {
	Credentials map[string]any `json:"credentials"`
	Operation   string         `json:"operation"`
	IsAsync     bool
}

type BindResource struct {
	AppGUID string `json:"app_guid"`
}

type UnbindPayload struct {
	BindingID  string
	InstanceID string
	UnbindRequestParameters
}

type UnbindRequestParameters struct {
	ServiceId string
	PlanID    string
}

type UnbindResponse struct {
	IsAsync   bool
	Operation string `json:"operation,omitempty"`
}

func (r UnbindResponse) IsComplete() bool {
	return r.Operation == ""
}

type LastOperationResponse struct {
	State       LastOperationResponseState `json:"state"`
	Description string                     `json:"description"`
}

type LastOperationResponseState string

func (s LastOperationResponseState) Value() string {
	if s == "succeeded" || s == "failed" {
		return string(s)
	}

	return "in progress"
}
