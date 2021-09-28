package cf

import (
	"gopkg.in/yaml.v3"
	"os"
)

type ControllerConfig struct {
	KpackImageTag string `yaml:"kpack_image_tag"`
}

func LoadConfigFromPath(path string) (*ControllerConfig, error) {
	var config ControllerConfig
	configFile, err := os.Open(path)
	if err != nil {
		return &config, err
	}
	defer configFile.Close()
	decoder := yaml.NewDecoder(configFile)
	err = decoder.Decode(&config)
	return &config, err
}
