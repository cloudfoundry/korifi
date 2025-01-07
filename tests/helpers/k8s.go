package helpers

import (
	"context"

	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	GinkgoHelper()

	configAccess := clientcmd.NewDefaultPathOptions()
	config, err := configAccess.GetStartingConfig()
	Expect(err).NotTo(HaveOccurred())
	delete(config.AuthInfos, userName)

	Expect(clientcmd.ModifyConfig(configAccess, *config, false)).To(Succeed())
}

func EnsureCreate(k8sClient client.Client, obj client.Object) {
	GinkgoHelper()

	Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)).To(Succeed())
	}).Should(Succeed())
}

func EnsurePatch[T any, PT k8s.ObjectWithDeepCopy[T]](k8sClient client.Client, obj PT, modifyFunc func(PT)) {
	GinkgoHelper()

	Expect(k8s.Patch(context.Background(), k8sClient, obj, func() {
		modifyFunc(obj)
	})).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		objCopy := obj.DeepCopy()
		modifyFunc(objCopy)
		g.Expect(equality.Semantic.DeepEqual(objCopy, obj)).To(BeTrue())
	}).Should(Succeed())
}

func EnsureDelete(k8sClient client.Client, obj client.Object) {
	GinkgoHelper()

	Expect(k8sClient.Delete(context.Background(), obj)).To(Succeed())
	Eventually(func(g Gomega) {
		err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)
		g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
	}).Should(Succeed())
}
