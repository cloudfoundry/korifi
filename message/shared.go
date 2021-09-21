package message

type Lifecycle struct {
	Type string        `json:"type" validate:"required"`
	Data LifecycleData `json:"data" validate:"required"`
}

type LifecycleData struct {
	Buildpacks []string `json:"buildpacks" validate:"required"`
	Stack      string   `json:"stack" validate:"required"`
}

type Relationship struct {
	Data *RelationshipData `json:"data" validate:"required"`
}

type RelationshipData struct {
	GUID string `json:"guid" validate:"required"`
}

type Metadata struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}
