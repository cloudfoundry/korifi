package eirinictrl

import "errors"

const (
	// Environment Variable Names
	EnvEiriniCertsDir = "EIRINI_CERTS_DIR"

	EnvPodName              = "POD_NAME"
	EnvCFInstanceIP         = "CF_INSTANCE_IP"
	EnvCFInstanceIndex      = "CF_INSTANCE_INDEX"
	EnvCFInstanceGUID       = "CF_INSTANCE_GUID"
	EnvCFInstanceInternalIP = "CF_INSTANCE_INTERNAL_IP"
	EnvCFInstanceAddr       = "CF_INSTANCE_ADDR"
	EnvCFInstancePort       = "CF_INSTANCE_PORT"
	EnvCFInstancePorts      = "CF_INSTANCE_PORTS"

	EiriniCertsDir = "/etc/eirini/certs"
)

var ErrNotFound = errors.New("not found")

var ErrInvalidInstanceIndex = errors.New("invalid instance index")

type ControllerConfig struct {
	KubeConfig `yaml:",inline"`

	ApplicationServiceAccount               string `yaml:"application_service_account"`
	RegistrySecretName                      string `yaml:"registry_secret_name"`
	AllowRunImageAsRoot                     bool   `yaml:"allow_run_image_as_root"`
	UnsafeAllowAutomountServiceAccountToken bool   `yaml:"unsafe_allow_automount_service_account_token"`
	DefaultMinAvailableInstances            string `yaml:"default_min_available_instances"`

	WorkloadsNamespace string

	PrometheusPort int `yaml:"prometheus_port"`
	TaskTTLSeconds int `yaml:"task_ttl_seconds"`

	LeaderElectionID        string
	LeaderElectionNamespace string

	WebhookPort int32 `yaml:"webhook_port"`
}

type KubeConfig struct {
	ConfigPath string `yaml:"kube_config_path"`
}
