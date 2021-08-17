package v1alpha1

// ResourceReference defines a reference to an instance of a resource in Kubernetes
type ResourceReference struct {
	Kind       CFKind `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Name       string `json:"name"`
}

// CFKind defines allowed value for Kind in ResourceReference
// +kubebuilder:validation:Enum=CFApp;CFBuild;CFPackage;CFDroplet;CFProcess
type CFKind string

type Lifecycle struct {
	// Specifies the CF Lifecycle type:
	// Valid values are:
	// "buildpack": stage the app using kpack
	Type LifecycleType `json:"type"`
	// Lifecycle data used to specify details for the Lifecycle
	Data LifecycleData `json:"data"`
}

// LifecycleType inform the platform of how to build droplets and run apps
// allow only values "buildpack"
// +kubebuilder:validation:Enum=buildpack
type LifecycleType string

// Shared by CFApp and CFBuild
type LifecycleData struct {
	// List of buildpacks used to build the app
	Buildpacks []string `json:"buildpacks"`
	Stack string `json:"stack"`
}


