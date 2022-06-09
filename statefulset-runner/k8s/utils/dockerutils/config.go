package dockerutils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
)

const DockerConfigKey string = ".dockerconfigjson"

type Config struct {
	Auths map[string]DockerAuth `json:"auths"`
}

type DockerAuth struct {
	User     string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

func NewDockerConfig(host, user, password string) *Config {
	return &Config{
		Auths: map[string]DockerAuth{
			host: {
				User:     user,
				Password: password,
				Auth: base64.StdEncoding.EncodeToString(
					[]byte(fmt.Sprintf("%s:%s", user, password)),
				),
			},
		},
	}
}

func (c *Config) JSON() (string, error) {
	jsonBytes, err := json.Marshal(c)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal docker config json")
	}

	return string(jsonBytes), nil
}
