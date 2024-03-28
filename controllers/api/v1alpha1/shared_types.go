package v1alpha1

import (
	"golang.org/x/exp/maps"
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

	RelationshipsLabelPrefix = "korifi.cloudfoundry.org/rel-"
	RelSpaceLabel            = RelationshipsLabelPrefix + "space"
	RelOrgLabel              = RelationshipsLabelPrefix + "org"
	RelServiceBrokerLabel    = RelationshipsLabelPrefix + "service_broker"
	RelServiceOfferingLabel  = RelationshipsLabelPrefix + "service_offering"
	RelSpaceQuotaLabel       = RelationshipsLabelPrefix + "space_quota"
	RelOrgQuotaLabel         = RelationshipsLabelPrefix + "org_quota"

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

func PatchStringMap(sm map[string]string, p map[string]*string) {
	for key, value := range p {
		if value == nil {
			delete(sm, key)
		} else {
			sm[key] = *value
		}
	}
}

type Metadata struct {
	// +kubebuilder:validation:Optional
	Labels map[string]string `json:"labels,omitempty"`
	// +kubebuilder:validation:Optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

type MetadataPatch struct {
	Labels      map[string]*string `json:"labels,omitempty"`
	Annotations map[string]*string `json:"annotations,omitempty"`
}

func (md *Metadata) Patch(p MetadataPatch) {
	PatchStringMap(md.Labels, p.Labels)
	PatchStringMap(md.Annotations, p.Annotations)
}

type Relationship struct {
	GUID string `json:"guid"`
}

func (in *Relationship) DeepCopy() *Relationship {
	if in == nil {
		return nil
	}
	out := new(Relationship)
	out.GUID = in.GUID
	return out
}

func (in *Relationship) DeepCopyInto(out *Relationship) {
	*out = *in
	out.GUID = in.GUID
}

type ToOneRelationship struct {
	//+kubebuilder:validation:Optional
	Data Relationship `json:"data"`
}

type ToManyRelationship struct {
	//+kubebuilder:validation:Optional
	Data []Relationship `json:"data"`
}

func (tm *ToManyRelationship) Patch(other ToManyRelationship) {
	guidMap := map[string]Relationship{}
	for _, r := range tm.Data {
		guidMap[r.GUID] = r
	}
	for _, r2 := range other.Data {
		guidMap[r2.GUID] = r2
	}
	tm.Data = maps.Values(guidMap)
}

type CFResourceState struct {
	Status  string
	Details string
}

const (
	ReadyStatus  = "ready"
	FailedStatus = "failed"
)

type CFResource struct {
	GUID      string           `json:"guid"`
	CreatedAt string           `json:"created_at"`
	UpdatedAt *string          `json:"updated_at"`
	State     *CFResourceState `json:"state"`
}

func (r CFResource) IsReady() bool {
	return r.State != nil && r.State.Status == ReadyStatus
}

func (r CFResource) IsFailed() bool {
	return r.State != nil && r.State.Status == FailedStatus
}

type BasicAuthentication struct {
	Type        string                         `json:"type"`
	Credentials BasicAuthenticationCredentials `json:"credentials"`
}

type BasicAuthenticationCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
