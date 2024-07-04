package crds_test

import (
	"context"
	"encoding/json"
	"testing"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/fail_handler"
	"code.cloudfoundry.org/korifi/tools"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func init() {
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme.Scheme))
}

func TestCrds(t *testing.T) {
	failHandler = fail_handler.New("CRDs Tests", fail_handler.Hook{
		Matcher: fail_handler.Always,
		Hook: func(config *rest.Config, failure fail_handler.TestFailure) {
			fail_handler.PrintKorifiLogs(config, "", failure.StartTime)
			printBuildLogs(config)
		},
	})
	RegisterFailHandler(failHandler.Fail)
	SetDefaultEventuallyTimeout(helpers.EventuallyTimeout())
	SetDefaultEventuallyPollingInterval(helpers.EventuallyPollingInterval())
	RunSpecs(t, "CRDs Suite")
}

var (
	failHandler        *fail_handler.Handler
	defaultAppBitsFile string

	ctx          context.Context
	k8sClient    client.Client
	k8sClientSet *k8sclient.Clientset

	rootNamespace string
	cfUser        string
	cfUserToken   string

	testOrg   *korifiv1alpha1.CFOrg
	testSpace *korifiv1alpha1.CFSpace
)

func printBuildLogs(config *rest.Config) {
	if testSpace == nil {
		return
	}

	if testSpace.Status.GUID == "" {
		return
	}

	fail_handler.PrintAllBuildLogs(config, testSpace.Status.GUID)
}

type sharedSetupData struct {
	DefaultAppBitsFile string
	RootNamespace      string
	CfUser             string
	CfUserToken        string
}

var _ = SynchronizedBeforeSuite(func() []byte {
	rootNamespace = helpers.GetDefaultedEnvVar("ROOT_NAMESPACE", "cf")
	serviceAccountFactory := helpers.NewServiceAccountFactory(rootNamespace)

	cfUser = uuid.NewString()
	cfUserToken = serviceAccountFactory.CreateServiceAccount(cfUser)
	helpers.AddUserToKubeConfig(cfUser, cfUserToken)

	bs, err := json.Marshal(sharedSetupData{
		DefaultAppBitsFile: helpers.ZipDirectory(
			helpers.GetDefaultedEnvVar("DEFAULT_APP_BITS_PATH", "../assets/dorifi"),
		),
		RootNamespace: rootNamespace,
		CfUser:        cfUser,
		CfUserToken:   cfUserToken,
	})
	Expect(err).NotTo(HaveOccurred())

	return bs
}, func(bs []byte) {
	var sharedSetup sharedSetupData
	err := json.Unmarshal(bs, &sharedSetup)
	Expect(err).NotTo(HaveOccurred())

	defaultAppBitsFile = sharedSetup.DefaultAppBitsFile
	rootNamespace = sharedSetup.RootNamespace
	cfUser = sharedSetup.CfUser
	cfUserToken = sharedSetup.CfUserToken

	ctx = context.Background()

	kubeconfig, err := ctrl.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(kubeconfig, client.Options{})
	Expect(err).NotTo(HaveOccurred())

	k8sClientSet, err = k8sclient.NewForConfig(kubeconfig)
	Expect(err).NotTo(HaveOccurred())
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	serviceAccountFactory := helpers.NewServiceAccountFactory(rootNamespace)
	serviceAccountFactory.DeleteServiceAccount(cfUser)
	helpers.RemoveUserFromKubeConfig(cfUser)
})

var _ = BeforeEach(func() {
	testOrg = &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: rootNamespace,
			Name:      uuid.NewString(),
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: uuid.NewString(),
		},
	}

	Expect(k8sClient.Create(ctx, testOrg)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testOrg), testOrg)).To(Succeed())
		g.Expect(meta.IsStatusConditionTrue(testOrg.StatusConditions(), korifiv1alpha1.StatusConditionReady)).To(BeTrue())
	}).Should(Succeed())

	testSpace = &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testOrg.Status.GUID,
			Name:      uuid.NewString(),
		},
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: uuid.NewString(),
		},
	}

	Expect(k8sClient.Create(ctx, testSpace)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testSpace), testSpace)).To(Succeed())
		g.Expect(meta.IsStatusConditionTrue(testSpace.StatusConditions(), korifiv1alpha1.StatusConditionReady)).To(BeTrue())
	}).Should(Succeed())
})

var _ = AfterEach(func() {
	Expect(k8sClient.Delete(ctx, testOrg.DeepCopy(), &client.DeleteOptions{
		PropagationPolicy: tools.PtrTo(metav1.DeletePropagationBackground),
	})).To(Succeed())
})
