package dockercfg

import (
	"encoding/base64"
	"encoding/json"
)

type dockerConfigJSON struct {
	Auths map[string]dockerConfigEntry `json:"auths" datapolicy:"token"`
}

type dockerConfigEntry struct {
	Auth string `json:"auth,omitempty"`
}

func GenerateDockerCfgSecretData(username, password, server string) ([]byte, error) {
	if server == "" || server == "index.docker.io" {
		server = "https://index.docker.io/v1/"
	}

	dockerConfigAuth := dockerConfigEntry{
		Auth: encodeDockerConfigFieldAuth(username, password),
	}
	result := dockerConfigJSON{
		Auths: map[string]dockerConfigEntry{server: dockerConfigAuth},
	}

	return json.Marshal(result)
}

// encodeDockerConfigFieldAuth returns base64 encoding of the username and password string
func encodeDockerConfigFieldAuth(username, password string) string {
	fieldValue := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(fieldValue))
}
