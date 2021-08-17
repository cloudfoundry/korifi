package v1alpha1

const (
	BuildpackLifecycle LifecycleType = "buildpack"
	DockerPackage      PackageType   = "docker"

	StartedState DesiredState = "STARTED"
	StoppedState DesiredState = "STOPPED"

	HTTPHealthCheckType    HealthCheckType = "http"
	PortHealthCheckType    HealthCheckType = "port"
	ProcessHealthCheckType HealthCheckType = "process"
)
