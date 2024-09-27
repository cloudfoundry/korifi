package osbapi

import "code.cloudfoundry.org/korifi/model/services"

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

type InstanceProvisionPayload struct {
	InstanceID string
	InstanceProvisionRequest
}

type InstanceProvisionRequest struct {
	ServiceId  string         `json:"service_id"`
	PlanID     string         `json:"plan_id"`
	SpaceGUID  string         `json:"space_guid"`
	OrgGUID    string         `json:"organization_guid"`
	Parameters map[string]any `json:"parameters"`
}

type GetLastOperationPayload struct {
	ID string
	GetLastOperationRequest
}

type GetLastOperationRequest struct {
	ServiceId string `json:"service_id"`
	PlanID    string `json:"plan_id"`
	Operation string `json:"operation"`
}

type InstanceDeprovisionPayload struct {
	ID string
	InstanceDeprovisionRequest
}

type InstanceDeprovisionRequest struct {
	ServiceId string `json:"service_id"`
	PlanID    string `json:"plan_id"`
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
}

type ServiceInstanceOperationResponse struct {
	Operation string `json:"operation"`
	Complete  bool
}

type LastOperationResponse struct {
	State       string `json:"state"`
	Description string `json:"description"`
}
