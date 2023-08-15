package helpers

import (
	"fmt"
	"strconv"

	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
	"k8s.io/client-go/kubernetes"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

func getClientSet() *kubernetes.Clientset {
	GinkgoHelper()

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())
	clientSet, err := kubernetes.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred())

	return clientSet
}

func GetClusterVersion() (int, int) {
	GinkgoHelper()

	serverVersion, err := getClientSet().ServerVersion()
	Expect(err).NotTo(HaveOccurred())

	majorVersion, err := strconv.Atoi(serverVersion.Major)
	if err != nil {
		Skip(fmt.Sprintf("cannot determine kubernetes server major version: %v", err))
	}

	minorVersion, err := strconv.Atoi(serverVersion.Minor)
	if err != nil {
		Skip(fmt.Sprintf("cannot determine kubernetes server minor version: %v", err))
	}

	return majorVersion, minorVersion
}
