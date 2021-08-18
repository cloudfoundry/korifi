package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	ServerURL string `json:"serverURL"`
	ServerPort int `json:"serverPort"`
}

func LoadConfigFromPath(path string) (*Config,error) {
	var config Config
	configFile, err := os.Open(path)
	if err != nil {
		return &config, err
	}
	defer configFile.Close()
	decoder := json.NewDecoder(configFile)
	err = decoder.Decode(&config)
	return &config, err
}