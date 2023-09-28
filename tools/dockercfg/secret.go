package dockercfg

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateDockerConfigSecret(
	secretNamespace string,
	secretName string,
	dockerConfigs ...DockerServerConfig,
) (*corev1.Secret, error) {
	dockerCfg, err := generateDockerCfgSecretData(dockerConfigs...)
	if err != nil {
		return nil, fmt.Errorf("failed to generate docker config secret data: %w", err)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNamespace,
			Name:      secretName,
		},
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: dockerCfg,
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}, nil
}

type dockerConfigJSON struct {
	Auths map[string]dockerConfigEntry `json:"auths" datapolicy:"token"`
}

type dockerConfigEntry struct {
	Auth string `json:"auth,omitempty"`
}

type DockerServerConfig struct {
	Server   string
	Username string
	Password string
}

func generateDockerCfgSecretData(entries ...DockerServerConfig) ([]byte, error) {
	result := dockerConfigJSON{
		Auths: map[string]dockerConfigEntry{},
	}

	for _, config := range entries {
		server := config.Server
		if server == "" || server == "index.docker.io" {
			server = "https://index.docker.io/v1/"
		}

		result.Auths[server] = dockerConfigEntry{
			Auth: encodeDockerConfigFieldAuth(config.Username, config.Password),
		}

	}

	return json.Marshal(result)
}

func encodeDockerConfigFieldAuth(username, password string) string {
	fieldValue := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(fieldValue))
}
