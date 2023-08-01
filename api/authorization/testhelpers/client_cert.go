package testhelpers

import (
	. "github.com/onsi/gomega" //lint:ignore ST1001 this is a test file
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func ObtainClientCert(testEnv *envtest.Environment, name string) ([]byte, []byte) {
	authUser, err := testEnv.ControlPlane.AddUser(envtest.User{Name: name}, testEnv.Config)
	Expect(err).NotTo(HaveOccurred())

	userConfig := authUser.Config()
	return userConfig.CertData, userConfig.KeyData
}

func JoinCertAndKey(certData, keyData []byte) []byte {
	result := []byte{}
	result = append(result, certData...)
	result = append(result, keyData...)

	return result
}
