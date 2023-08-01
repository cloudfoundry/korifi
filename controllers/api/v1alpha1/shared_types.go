package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	CFAppGUIDLabelKey        = "korifi.cloudfoundry.org/app-guid"
	CFAppRevisionKey         = "korifi.cloudfoundry.org/app-rev"
	CFAppLastStopRevisionKey = "korifi.cloudfoundry.org/last-stop-app-rev"
	CFAppRevisionKeyDefault  = "0"
	CFPackageGUIDLabelKey    = "korifi.cloudfoundry.org/package-guid"
	CFBuildGUIDLabelKey      = "korifi.cloudfoundry.org/build-guid"
	CFProcessGUIDLabelKey    = "korifi.cloudfoundry.org/process-guid"
	CFProcessTypeLabelKey    = "korifi.cloudfoundry.org/process-type"
	CFDomainGUIDLabelKey     = "korifi.cloudfoundry.org/domain-guid"
	CFRouteGUIDLabelKey      = "korifi.cloudfoundry.org/route-guid"
	CFTaskGUIDLabelKey       = "korifi.cloudfoundry.org/task-guid"

	CFBindingTypeLabelKey = "korifi.cloudfoundry.org/binding-type"

	StagingConditionType   = "Staging"
	ReadyConditionType     = "Ready"
	SucceededConditionType = "Succeeded"

	PropagateRoleBindingAnnotation    = "cloudfoundry.org/propagate-cf-role"
	PropagateServiceAccountAnnotation = "cloudfoundry.org/propagate-service-account"
	PropagateDeletionAnnotation       = "cloudfoundry.org/propagate-deletion"
	PropagatedFromLabel               = "cloudfoundry.org/propagated-from"
)

type Lifecycle struct {
	// The CF Lifecycle type.
	// Only "buildpack" is currently allowed
	Type LifecycleType `json:"type"`
	// Data used to specify details for the Lifecycle
	Data LifecycleData `json:"data"`
}

// LifecycleType inform the platform of how to build droplets and run apps
// allow only values "buildpack"
// +kubebuilder:validation:Enum=buildpack
type LifecycleType string

// LifecycleData is shared by CFApp and CFBuild
type LifecycleData struct {
	// Buildpacks to include in auto-detection when building the app image.
	// If no values are specified, then all available buildpacks will be used for auto-detection
	Buildpacks []string `json:"buildpacks,omitempty"`

	// Stack to use when building the app image
	Stack string `json:"stack"`
}

// Registry is used by CFPackage and CFBuild/Droplet to identify Registry and secrets to access the image provided
type Registry struct {
	// The location of the source image
	Image string `json:"image"`
	// A list of secrets required to pull the image from its repository
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// RequiredLocalObjectReference is a reference to an object in the same namespace.
// Unlike k8s.io/api/core/v1/LocalObjectReference, name is required.
type RequiredLocalObjectReference struct {
	Name string `json:"name"`
}
