package cf

import (
	"os"

	"gopkg.in/yaml.v3"
)

type ControllerConfig struct {
	KpackImageTag string `yaml:"kpackImageTag"`
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
