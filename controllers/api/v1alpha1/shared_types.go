package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	CFAppGUIDLabelKey       = "korifi.cloudfoundry.org/app-guid"
	CFAppRevisionKey        = "korifi.cloudfoundry.org/app-rev"
	CFAppRevisionKeyDefault = "0"
	CFPackageGUIDLabelKey   = "korifi.cloudfoundry.org/package-guid"
	CFBuildGUIDLabelKey     = "korifi.cloudfoundry.org/build-guid"
	CFProcessGUIDLabelKey   = "korifi.cloudfoundry.org/process-guid"
	CFProcessTypeLabelKey   = "korifi.cloudfoundry.org/process-type"
	CFDomainGUIDLabelKey    = "korifi.cloudfoundry.org/domain-guid"
	CFRouteGUIDLabelKey     = "korifi.cloudfoundry.org/route-guid"
	CFTaskGUIDLabelKey      = "korifi.cloudfoundry.org/task-guid"

	StagingConditionType   = "Staging"
	ReadyConditionType     = "Ready"
	SucceededConditionType = "Succeeded"

	PropagateRoleBindingAnnotation = "cloudfoundry.org/propagate-cf-role"
	PropagatedFromLabel            = "cloudfoundry.org/propagated-from"
)

type Lifecycle struct {
	// Specifies the CF Lifecycle type:
	// Valid values are:
	// "buildpack": stage the app using buildpacks
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

// Registry is used by CFPackage and CFBuild/Droplet to identify Registry and secrets to access the image provided
type Registry struct {
	// Image specifies the location of the source image
	Image string `json:"image"`
	// ImagePullSecrets specifies a list of secrets required to access the image
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}
