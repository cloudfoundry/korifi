package config

import (
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/tools"
)

const defaultJobTTL = 24 * time.Hour

type JobTaskRunnerConfig struct {
	JobTTL string `yaml:"jobTTL"`
}

func LoadFromPath(path string) (*JobTaskRunnerConfig, error) {
	var config JobTaskRunnerConfig
	err := tools.LoadConfigInto(&config, path)
	if err != nil {
		return nil, fmt.Errorf("failed loading config: %w", err)
	}

	return &config, nil
}

func (c JobTaskRunnerConfig) ParseJobTTL() (time.Duration, error) {
	if c.JobTTL == "" {
		return defaultJobTTL, nil
	}

	return tools.ParseDuration(c.JobTTL)
}
