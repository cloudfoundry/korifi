package tests

import (
	"os"
	"path/filepath"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func GetKubeconfig() string {
	kubeconfPath := os.Getenv("INTEGRATION_KUBECONFIG")
	if kubeconfPath != "" {
		return kubeconfPath
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		Fail("INTEGRATION_KUBECONFIG not provided, failed to use default: " + err.Error())
	}

	kubeconfPath = filepath.Join(homeDir, ".kube", "config")

	_, err = os.Stat(kubeconfPath)
	if os.IsNotExist(err) {
		return ""
	}

	Expect(err).NotTo(HaveOccurred())

	return kubeconfPath
}

func GetEiriniDockerHubPassword() string {
	return lookupOptionalEnv("EIRINIUSER_PASSWORD")
}

func GetEiriniSystemNamespace() string {
	return lookupOptionalEnv("EIRINI_SYSTEM_NS")
}

func GetEiriniWorkloadsNamespace() string {
	return lookupOptionalEnv("EIRINI_WORKLOADS_NS")
}

func GetEiriniAddress() string {
	return lookupOptionalEnv("EIRINI_ADDRESS")
}

func lookupOptionalEnv(key string) string {
	value, set := os.LookupEnv(key)
	if !set || value == "" {
		Skip("Please export optional environment variable " + key + " to run this test")
	}

	return value
}

func GetTelepresenceServiceName() string {
	serviceName := os.Getenv("TELEPRESENCE_SERVICE_NAME")
	Expect(serviceName).ToNot(BeEmpty())

	return serviceName
}

func GetTelepresencePort() int {
	startPort := os.Getenv("TELEPRESENCE_EXPOSE_PORT_START")
	Expect(startPort).ToNot(BeEmpty())

	portNo, err := strconv.Atoi(startPort)
	Expect(err).NotTo(HaveOccurred())

	return portNo + GinkgoParallelProcess() - 1
}
