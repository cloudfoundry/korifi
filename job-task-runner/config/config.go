package config

import (
	"fmt"
	"os"
	"time"

	"code.cloudfoundry.org/korifi/tools"
)

const defaultJobTTL = 24 * time.Hour

type JobTaskRunnerConfig struct {
	JobTTL string
}

func LoadFromEnv() *JobTaskRunnerConfig {
	return &JobTaskRunnerConfig{
		JobTTL: mustHaveEnv("JOB_TTL"),
	}
}

func (c JobTaskRunnerConfig) ParseJobTTL() (time.Duration, error) {
	if c.JobTTL == "" {
		return defaultJobTTL, nil
	}

	return tools.ParseDuration(c.JobTTL)
}

func mustHaveEnv(name string) string {
	value, ok := os.LookupEnv(name)
	if !ok {
		panic(fmt.Sprintf("Env var %s not set", name))
	}

	return value
}
