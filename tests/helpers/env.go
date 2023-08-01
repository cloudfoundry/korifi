package helpers

import (
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func GetRequiredEnvVar(envVarName string) string {
	ginkgo.GinkgoHelper()

	value, ok := os.LookupEnv(envVarName)
	gomega.Expect(ok).To(gomega.BeTrue(), envVarName+" environment variable is required, but was not provided.")
	return value
}

func GetDefaultedEnvVar(envVarName, defaultValue string) string {
	value, ok := os.LookupEnv(envVarName)
	if !ok {
		return defaultValue
	}
	return value
}
