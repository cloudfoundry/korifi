package osbapi

type Catalog struct {
	Services []Service `json:"services"`
}

type Service struct {
	Name           string   `json:"name"`
	Id             string   `json:"id"`
	Description    string   `json:"description"`
	Tags           []string `json:"tags"`
	Requires       []string `json:"requires"`
	Bindable       bool     `json:"bindable"`
	Metadata       any      `json:"metadata"`
	PlanUpdateable bool     `json:"plan_updateable"`
	Plans          []Plan   `json:"plans"`
}

type Plan struct {
	Id              string          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Metadata        PlanMetadata    `json:"metadata"`
	Free            bool            `json:"free"`
	Bindable        bool            `json:"bindable"`
	Schemas         Schemas         `json:"schemas"`
	MaintenanceInfo MaintenanceInfo `json:"maintenance_info"`
}

type PlanMetadata struct {
	Bullets     []string `json:"bullets"`
	Costs       []any    `json:"costs"`
	DisplayName string   `json:"displayName"`
}

type Schemas struct {
	ServiceInstance ServiceInstanceSchema `json:"service_instance"`
	ServiceBinding  ServiceBindingSchema  `json:"service_binding"`
}

type ServiceInstanceSchema struct {
	Create InputParametersSchema `json:"create"`
	Update InputParametersSchema `json:"update"`
}

type ServiceBindingSchema struct {
	Create InputParametersSchema `json:"create"`
}

type InputParametersSchema struct {
	parameters map[string]any `json:"parameters"`
}

type MaintenanceInfo struct {
	Version string
}
