package v1alpha1

type AppQuotas struct {
	// +kubebuilder:validation:Optional
	PerProcessMemoryInMb int64 `json:"per_process_memory_in_mb"`
	// +kubebuilder:validation:Optional
	TotalMemoryInMb int64 `json:"total_memory_in_mb"`
	// +kubebuilder:validation:Optional
	TotalInstances int64 `json:"total_instances"`
	// +kubebuilder:validation:Optional
	LogRateLimitInBytesPerSecond int64 `json:"log_rate_limit_in_bytes_per_second"`
	// +kubebuilder:validation:Optional
	PerAppTasks int64 `json:"per_app_tasks"`
}

type AppQuotasPatch struct {
	PerProcessMemoryInMb         *int64 `json:"per_process_memory_in_mb"`
	TotalMemoryInMb              *int64 `json:"total_memory_in_mb"`
	TotalInstances               *int64 `json:"total_instances"`
	LogRateLimitInBytesPerSecond *int64 `json:"log_rate_limit_in_bytes_per_second"`
	PerAppTasks                  *int64 `json:"per_app_tasks"`
}

func (aq *AppQuotas) Patch(p AppQuotasPatch) {
	if p.PerProcessMemoryInMb != nil {
		aq.PerProcessMemoryInMb = *p.PerProcessMemoryInMb
	}
	if p.TotalMemoryInMb != nil {
		aq.TotalMemoryInMb = *p.TotalMemoryInMb
	}
	if p.TotalInstances != nil {
		aq.TotalInstances = *p.TotalInstances
	}
	if p.LogRateLimitInBytesPerSecond != nil {
		aq.LogRateLimitInBytesPerSecond = *p.LogRateLimitInBytesPerSecond
	}
	if p.PerAppTasks != nil {
		aq.PerAppTasks = *p.PerAppTasks
	}
}

type ServiceQuotas struct {
	// +kubebuilder:validation:Optional
	PaidServicesAllowed bool `json:"paid_services_allowed"`
	// +kubebuilder:validation:Optional
	TotalServiceInstances int64 `json:"total_service_instances"`
	// +kubebuilder:validation:Optional
	TotalServiceKeys int64 `json:"total_service_keys"`
}

type ServiceQuotasPatch struct {
	PaidServicesAllowed   *bool  `json:"paid_services_allowed"`
	TotalServiceInstances *int64 `json:"total_service_instances"`
	TotalServiceKeys      *int64 `json:"total_service_keys"`
}

func (sq *ServiceQuotas) Patch(p ServiceQuotasPatch) {
	if p.PaidServicesAllowed != nil {
		sq.PaidServicesAllowed = *p.PaidServicesAllowed
	}
	if p.TotalServiceInstances != nil {
		sq.TotalServiceInstances = *p.TotalServiceInstances
	}
	if p.TotalServiceKeys != nil {
		sq.TotalServiceKeys = *p.TotalServiceKeys
	}
}

type RouteQuotas struct {
	// +kubebuilder:validation:Optional
	TotalRoutes int64 `json:"total_routes"`
	// +kubebuilder:validation:Optional
	TotalReservedPorts int64 `json:"total_reserved_ports"`
}

type RouteQuotasPatch struct {
	TotalRoutes        *int64 `json:"total_routes"`
	TotalReservedPorts *int64 `json:"total_reserved_ports"`
}

func (rq *RouteQuotas) Patch(p RouteQuotasPatch) {
	if p.TotalRoutes != nil {
		rq.TotalRoutes = *p.TotalRoutes
	}
	if p.TotalReservedPorts != nil {
		rq.TotalReservedPorts = *p.TotalReservedPorts
	}
}
