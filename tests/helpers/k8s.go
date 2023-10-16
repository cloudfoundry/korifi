package helpers

import (
	"fmt"
	"strings"

	"github.com/cloudfoundry/cf-test-helpers/commandreporter"
	"github.com/cloudfoundry/cf-test-helpers/commandstarter"
	. "github.com/onsi/ginkgo/v2"    //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"       //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega/gexec" //lint:ignore ST1001 this is a test file
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

func Kubectl(args ...string) *Session {
	cmdStarter := commandstarter.NewCommandStarter()
	return kubectlWithCustomReporter(cmdStarter, commandreporter.NewCommandReporter(), args...)
}

func KubectlApply(stdinText string, sprintfArgs ...any) *Session {
	cmdStarter := commandstarter.NewCommandStarterWithStdin(
		strings.NewReader(
			fmt.Sprintf(stdinText, sprintfArgs...),
		),
	)
	return kubectlWithCustomReporter(cmdStarter, commandreporter.NewCommandReporter(), "apply", "-f=-")
}

func kubectlWithCustomReporter(cmdStarter *commandstarter.CommandStarter, reporter *commandreporter.CommandReporter, args ...string) *Session {
	request, err := cmdStarter.Start(reporter, "kubectl", args...)
	if err != nil {
		panic(err)
	}

	return request.Wait()
}
