package helpers

import (
	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
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

func AddUserToKubeConfig(userName, userToken string) {
	GinkgoHelper()

	authInfo := clientcmdapi.NewAuthInfo()
	authInfo.Token = userToken

	configAccess := clientcmd.NewDefaultPathOptions()
	config, err := configAccess.GetStartingConfig()
	Expect(err).NotTo(HaveOccurred())
	config.AuthInfos[userName] = authInfo

	Expect(clientcmd.ModifyConfig(configAccess, *config, false)).To(Succeed())
}

func RemoveUserFromKubeConfig(userName string) {
	configAccess := clientcmd.NewDefaultPathOptions()
	config, err := configAccess.GetStartingConfig()
	Expect(err).NotTo(HaveOccurred())
	delete(config.AuthInfos, userName)

	Expect(clientcmd.ModifyConfig(configAccess, *config, false)).To(Succeed())
}
