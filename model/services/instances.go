package services

type LastOperation struct {
	// +kubebuilder:validation:Enum=create;update;delete
	Type string `json:"type"`
	// +kubebuilder:validation:Enum=initial;in progress;succeeded;failed
	State string `json:"state"`

	//+kubebuilder:validation:Optional
	Description string `json:"description"`
}
