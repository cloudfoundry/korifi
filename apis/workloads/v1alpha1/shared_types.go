package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	cfAppGUIDLabelKey     = "workloads.cloudfoundry.org/app-guid"
	cfProcessGUIDLabelKey = "workloads.cloudfoundry.org/process-guid"
	cfProcessTypeLabelKey = "workloads.cloudfoundry.org/process-type"
)

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
	Buildpacks []string `json:"buildpacks,omitempty"`
	Stack      string   `json:"stack"`
}

// Registry is used by CFPackage and CFDroplet to identify Registry and secrets to access the image provided
type Registry struct {
	// Image specifies the location of the source image
	Image string `json:"image"`
	// ImagePullSecrets specifies a list of secrets required to access the image
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}
