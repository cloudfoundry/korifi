package broker

type CatalogResponse struct {
	Services []Service `json:"services"`
}
type Service struct {
	Name                 string         `json:"name"`
	ID                   string         `json:"id"`
	Description          string         `json:"description"`
	Bindable             bool           `json:"bindable"`
	InstancesRetrievable bool           `json:"instances_retrievable,omitempty"`
	BindingsRetrievable  bool           `json:"bindings_retrievable,omitempty"`
	PlanUpdateable       bool           `json:"plan_updateable,omitempty"`
	Tags                 []string       `json:"tags,omitempty"`
	Metadata             map[string]any `json:"metadata,omitempty"`
	Plans                []Plan         `json:"plans"`
}
type Plan struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Free        *bool    `json:"free,omitempty"`
	Schemas     *Schemas `json:"schemas,omitempty"`
}
type Schemas struct {
	ServiceInstance *ServiceInstanceSchema `json:"service_instance,omitempty"`
	ServiceBinding  *ServiceBindingSchema  `json:"service_binding,omitempty"`
}
type ServiceInstanceSchema struct {
	Create *InputParametersSchema `json:"create,omitempty"`
	Update *InputParametersSchema `json:"update,omitempty"`
}
type ServiceBindingSchema struct {
	Create *InputParametersSchema `json:"create,omitempty"`
}
type InputParametersSchema struct {
	Parameters map[string]any `json:"parameters,omitempty"`
}

type ProvisionRequest struct {
	ServiceID         string         `json:"service_id"`
	PlanID            string         `json:"plan_id"`
	Context           map[string]any `json:"context,omitempty"`
	Parameters        map[string]any `json:"parameters,omitempty"`
	AcceptsIncomplete bool           `json:"-"`
}
type ProvisionResponse struct {
	DashboardURL string `json:"dashboard_url,omitempty"`
	Operation    string `json:"operation,omitempty"`
}
type GetInstanceResponse struct {
	ServiceID  string         `json:"service_id"`
	PlanID     string         `json:"plan_id"`
	Parameters map[string]any `json:"parameters,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}
type UpdateRequest struct {
	ServiceID         string         `json:"service_id"`
	PlanID            string         `json:"plan_id,omitempty"`
	Context           map[string]any `json:"context,omitempty"`
	Parameters        map[string]any `json:"parameters,omitempty"`
	AcceptsIncomplete bool           `json:"-"`
}
type UpdateResponse struct {
	Operation string `json:"operation,omitempty"`
}
type DeprovisionResponse struct {
	Operation string `json:"operation,omitempty"`
}
type BindRequest struct {
	ServiceID    string         `json:"service_id"`
	PlanID       string         `json:"plan_id"`
	AppGUID      string         `json:"app_guid,omitempty"`
	BindResource map[string]any `json:"bind_resource,omitempty"`
	Parameters   map[string]any `json:"parameters,omitempty"`
}
type BindResponse struct {
	Credentials     map[string]any `json:"credentials,omitempty"`
	SyslogDrainURL  string         `json:"syslog_drain_url,omitempty"`
	RouteServiceURL string         `json:"route_service_url,omitempty"`
	VolumeMounts    []any          `json:"volume_mounts,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	ExpiresAt       string         `json:"expires_at,omitempty"`
}
type LastOperationResponse struct {
	State       string `json:"state"`
	Description string `json:"description,omitempty"`
	RetryAfter  int    `json:"retry_after,omitempty"`
}
type ErrorResponse struct {
	Error       string `json:"error,omitempty"`
	Description string `json:"description,omitempty"`
}
