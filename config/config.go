package config

import (
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	ServerURL  string `yaml:"serverURL"`
	ServerPort int    `yaml:"serverPort"`
}

func LoadFromPath(path string) (*Config, error) {
	var config Config
	configFile, err := os.Open(path)
	if err != nil {
		return &config, err
	}
	defer configFile.Close()
	decoder := yaml.NewDecoder(configFile)
	err = decoder.Decode(&config)
	return &config, err
}
