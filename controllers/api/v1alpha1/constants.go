package v1alpha1

const (
	BuildpackLifecycle LifecycleType = "buildpack"
	DockerPackage      PackageType   = "docker"

	StartedState AppState = "STARTED"
	StoppedState AppState = "STOPPED"

	HTTPHealthCheckType    HealthCheckType = "http"
	PortHealthCheckType    HealthCheckType = "port"
	ProcessHealthCheckType HealthCheckType = "process"

	StatusConditionReady = "Ready"
)
