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

type ServiceQuotas struct {
	// +kubebuilder:validation:Optional
	PaidServicesAllowed bool `json:"paid_services_allowed"`
	// +kubebuilder:validation:Optional
	TotalServiceInstances int64 `json:"total_service_instances"`
	// +kubebuilder:validation:Optional
	TotalServiceKeys int64 `json:"total_service_keys"`
}

type RouteQuotas struct {
	// +kubebuilder:validation:Optional
	TotalRoutes int64 `json:"total_routes"`
	// +kubebuilder:validation:Optional
	TotalReservedPorts int64 `json:"total_reserved_ports"`
}
