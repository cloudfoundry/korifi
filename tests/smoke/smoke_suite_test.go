package smoke_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/fail_handler"

	"github.com/cloudfoundry/cf-test-helpers/generator"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	gomegatypes "github.com/onsi/gomega/types"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	rest "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const NamePrefix = "cf-on-k8s-smoke"

var (
	appsDomain            string
	buildpackAppName      string
	cfAdmin               string
	dockerAppName         string
	orgName               string
	rootNamespace         string
	serviceAccountFactory *helpers.ServiceAccountFactory
	spaceName             string
)

func TestSmoke(t *testing.T) {
	RegisterFailHandler(fail_handler.New("Smoke Tests", map[gomegatypes.GomegaMatcher]func(*rest.Config, string){
		fail_handler.Always: func(config *rest.Config, _ string) {
			printCfApp(config)
			fail_handler.PrintPodsLogs(config, []fail_handler.PodContainerDescriptor{
				{
					Namespace:  "korifi",
					LabelKey:   "app",
					LabelValue: "korifi-controllers",
					Container:  "manager",
				},
			})
		},
	}))
	SetDefaultEventuallyTimeout(helpers.EventuallyTimeout())
	SetDefaultEventuallyPollingInterval(helpers.EventuallyPollingInterval())
	RunSpecs(t, "Smoke Tests Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	rootNamespace = helpers.GetDefaultedEnvVar("ROOT_NAMESPACE", "cf")
	serviceAccountFactory = helpers.NewServiceAccountFactory(rootNamespace)

	Expect(
		helpers.Kubectl("get", "namespace/"+rootNamespace),
	).To(Exit(0), "Could not find root namespace called %q", rootNamespace)

	cfAdmin = uuid.NewString()
	cfAdminToken := serviceAccountFactory.CreateAdminServiceAccount(cfAdmin)
	helpers.AddUserToKubeConfig(cfAdmin, cfAdminToken)

	Expect(helpers.Cf("api", helpers.GetRequiredEnvVar("API_SERVER_ROOT"), "--skip-ssl-validation")).To(Exit(0))
	Expect(helpers.Cf("auth", cfAdmin)).To(Exit(0))

	appsDomain = helpers.GetRequiredEnvVar("APP_FQDN")
	orgName = generator.PrefixedRandomName(NamePrefix, "org")
	spaceName = generator.PrefixedRandomName(NamePrefix, "space")
	buildpackAppName = generator.PrefixedRandomName(NamePrefix, "buildpackapp")
	dockerAppName = generator.PrefixedRandomName(NamePrefix, "dockerapp")

	Expect(helpers.Cf("create-org", orgName)).To(Exit(0))
	Expect(helpers.Cf("create-space", "-o", orgName, spaceName)).To(Exit(0))
	Expect(helpers.Cf("target", "-o", orgName, "-s", spaceName)).To(Exit(0))

	Expect(helpers.Cf("push", buildpackAppName, "-p", "../assets/dorifi")).To(Exit(0))
	Expect(helpers.Cf("push", dockerAppName, "-o", "eirini/dorini")).To(Exit(0))
})

var _ = AfterSuite(func() {
	if CurrentSpecReport().State.Is(types.SpecStateFailed) {
		printAppReport(buildpackAppName)
	}

	Expect(helpers.Cf("delete-org", orgName, "-f").Wait()).To(Exit(0))

	serviceAccountFactory.DeleteServiceAccount(cfAdmin)
	helpers.RemoveUserFromKubeConfig(cfAdmin)
})

func sessionOutput(session *Session) string {
	GinkgoHelper()

	Expect(session.ExitCode()).To(Equal(0))
	return strings.TrimSpace(string(session.Out.Contents()))
}

func printCfApp(config *rest.Config) {
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme.Scheme))
	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "failed to create k8s client: %v\n", err)
		return
	}

	cfAppNamespace := sessionOutput(helpers.Cf("space", spaceName, "--guid"))
	cfAppGUID := sessionOutput(helpers.Cf("app", buildpackAppName, "--guid"))

	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfAppGUID,
			Namespace: cfAppNamespace,
		},
	}

	if err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfApp), cfApp); err != nil {
		fmt.Fprintf(GinkgoWriter, "failed to get cfapp in namespace %q: %v\n", cfAppNamespace, err)
		return
	}

	fmt.Fprintf(GinkgoWriter, "\n\n========== cfapp %s/%s (skipping managed fields) ==========\n", cfApp.Namespace, cfApp.Name)
	cfApp.ManagedFields = []metav1.ManagedFieldsEntry{}
	cfAppBytes, err := yaml.Marshal(cfApp)
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "failed marshalling cfapp: %v\n", err)
		return
	}
	fmt.Fprintln(GinkgoWriter, string(cfAppBytes))
}
