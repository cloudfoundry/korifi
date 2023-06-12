package payloads

type Lifecycle struct {
	Type string        `json:"type" validate:"required"`
	Data LifecycleData `json:"data" validate:"required"`
}

type LifecycleData struct {
	Buildpacks []string `json:"buildpacks" validate:"required"`
	Stack      string   `json:"stack" validate:"required"`
}
