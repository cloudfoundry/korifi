package helpers

import (
	. "github.com/onsi/gomega"
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
