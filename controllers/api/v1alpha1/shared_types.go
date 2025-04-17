package v1alpha1

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	VersionLabelKey = "korifi.cloudfoundry.org/version"

	CFAppGUIDLabelKey        = "korifi.cloudfoundry.org/app-guid"
	CFAppRevisionKey         = "korifi.cloudfoundry.org/app-rev"
	CFAppDisplayNameKey      = "korifi.cloudfoundry.org/display-name"
	CFAppLastStopRevisionKey = "korifi.cloudfoundry.org/last-stop-app-rev"
	CFAppDefaultRevision     = "0"
	CFPackageGUIDLabelKey    = "korifi.cloudfoundry.org/package-guid"
	CFBuildGUIDLabelKey      = "korifi.cloudfoundry.org/build-guid"
	CFProcessGUIDLabelKey    = "korifi.cloudfoundry.org/process-guid"
	CFProcessTypeLabelKey    = "korifi.cloudfoundry.org/process-type"
	CFDomainGUIDLabelKey     = "korifi.cloudfoundry.org/domain-guid"
	CFDomainNameLabelKey     = "korifi.cloudfoundry.org/domain-name"
	CFRouteGUIDLabelKey      = "korifi.cloudfoundry.org/route-guid"
	CFTaskGUIDLabelKey       = "korifi.cloudfoundry.org/task-guid"

	GUIDLabelKey            = "korifi.cloudfoundry.org/guid"
	SpaceGUIDKey            = "korifi.cloudfoundry.org/space-guid"
	ServiceBindingTypeLabel = "korifi.cloudfoundry.org/service-binding-type"

	PodIndexLabelKey = "apps.kubernetes.io/pod-index"

	StagingConditionType   = "Staging"
	SucceededConditionType = "Succeeded"

	PropagateRoleBindingAnnotation    = "cloudfoundry.org/propagate-cf-role"
	PropagateServiceAccountAnnotation = "cloudfoundry.org/propagate-service-account"
	PropagateDeletionAnnotation       = "cloudfoundry.org/propagate-deletion"
	PropagatedFromLabel               = "cloudfoundry.org/propagated-from"

	RelationshipsLabelPrefix    = "korifi.cloudfoundry.org/rel-"
	RelServiceBrokerGUIDLabel   = RelationshipsLabelPrefix + "service-broker-guid"
	RelServiceBrokerNameLabel   = RelationshipsLabelPrefix + "service-broker-name"
	RelServiceOfferingGUIDLabel = RelationshipsLabelPrefix + "service-offering-guid"
	RelServiceOfferingNameLabel = RelationshipsLabelPrefix + "service-offering-name"

	InstanceStateDown     InstanceState = "DOWN"
	InstanceStateCrashed  InstanceState = "CRASHED"
	InstanceStateStarting InstanceState = "STARTING"
	InstanceStateRunning  InstanceState = "RUNNING"
)

type Lifecycle struct {
	// The CF Lifecycle type.
	// Only "buildpack" and "docker" are currently allowed
	Type LifecycleType `json:"type"`
	// Data used to specify details for the Lifecycle
	Data LifecycleData `json:"data"`
}

// LifecycleType inform the platform of how to build droplets and run apps
// allow only values "buildpack" or "docker"
// +kubebuilder:validation:Enum=buildpack;docker
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

// +kubebuilder:validation:Enum=DOWN;CRASHED;STARTING;RUNNING
type InstanceState string

type InstanceStatus struct {
	// The state of the instance
	State InstanceState `json:"state"`

	// The time the instance got into this status; nil if unknown
	// +kubebuilder:validation:Optional
	Timestamp *metav1.Time `json:"timestamp"`
}

type ServiceBinding struct {
	// the guid of the CFserviceBinding
	GUID string `json:"guid"`

	// The name of binding. Used as binding name when projecting the secret onto the workload
	Name string `json:"name"`

	// Name of the binding secret
	Secret string `json:"secret"`
}

func AsMap(obj *runtime.RawExtension) (map[string]any, error) {
	if obj == nil {
		return nil, nil
	}

	var m map[string]any
	err := json.Unmarshal(obj.Raw, &m)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func AsRawExtension(obj map[string]any) (*runtime.RawExtension, error) {
	rawBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	return &runtime.RawExtension{
		Raw: rawBytes,
	}, nil
}
