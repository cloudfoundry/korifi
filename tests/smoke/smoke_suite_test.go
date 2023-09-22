package smoke_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/fail_handler"

	"github.com/cloudfoundry/cf-test-helpers/cf"
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
			_, _ = runCfCmd("apps")
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
	SetDefaultEventuallyTimeout(5 * time.Minute)
	SetDefaultEventuallyPollingInterval(5 * time.Second)
	RunSpecs(t, "Smoke Tests Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	rootNamespace = helpers.GetDefaultedEnvVar("ROOT_NAMESPACE", "cf")
	serviceAccountFactory = helpers.NewServiceAccountFactory(rootNamespace)

	Eventually(
		helpers.Kubectl("get", "namespace/"+rootNamespace),
	).Should(Exit(0), "Could not find root namespace called %q", rootNamespace)

	cfAdmin = uuid.NewString()
	cfAdminToken := serviceAccountFactory.CreateAdminServiceAccount(cfAdmin)
	helpers.AddUserToKubeConfig(cfAdmin, cfAdminToken)

	loginAs(cfAdmin)

	appsDomain = helpers.GetRequiredEnvVar("APP_FQDN")
	orgName = generator.PrefixedRandomName(NamePrefix, "org")
	spaceName = generator.PrefixedRandomName(NamePrefix, "space")
	buildpackAppName = generator.PrefixedRandomName(NamePrefix, "buildpackapp")
	dockerAppName = generator.PrefixedRandomName(NamePrefix, "dockerapp")

	Eventually(cf.Cf("create-org", orgName)).Should(Exit(0))
	Eventually(cf.Cf("create-space", "-o", orgName, spaceName)).Should(Exit(0))
	Eventually(cf.Cf("target", "-o", orgName, "-s", spaceName)).Should(Exit(0))

	Eventually(
		cf.Cf("push", buildpackAppName, "-p", "../assets/dorifi"),
	).Should(Exit(0))

	Eventually(
		cf.Cf("push", dockerAppName, "-o", "eirini/dorini"),
	).Should(Exit(0))
})

var _ = AfterSuite(func() {
	if CurrentSpecReport().State.Is(types.SpecStateFailed) {
		printAppReport(buildpackAppName)
	}

	Eventually(func() *Session {
		return cf.Cf("delete-org", orgName, "-f").Wait()
	}).Should(Exit(0))

	serviceAccountFactory.DeleteServiceAccount(cfAdmin)
	helpers.RemoveUserFromKubeConfig(cfAdmin)
})

func loginAs(user string) {
	apiArguments := []string{
		"api",
		helpers.GetRequiredEnvVar("API_SERVER_ROOT"),
		"--skip-ssl-validation",
	}
	Eventually(cf.Cf(apiArguments...)).Should(Exit(0))

	// Stdin contains username followed by 2 return carriages. Firtst one
	// enters the username and second one skips the org selection prompt that
	// is presented if there is more than one org
	loginSession := cf.CfWithStdin(bytes.NewBufferString(user+"\n\n"), "login")
	Eventually(loginSession).Should(Exit(0))
}

func runCfCmd(args ...string) (string, error) {
	session := cf.Cf(args...)
	<-session.Exited
	if session.ExitCode() != 0 {
		return "", fmt.Errorf("cf %s exited with code %d", strings.Join(args, " "), session.ExitCode())
	}

	return strings.TrimSpace(string(session.Out.Contents())), nil
}

func printCfApp(config *rest.Config) {
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme.Scheme))
	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "failed to create k8s client: %v\n", err)
		return
	}

	cfAppNamespace, err := runCfCmd("space", spaceName, "--guid")
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "failed to get app space guid: %v\n", err)
		return
	}

	cfAppGUID, err := runCfCmd("app", buildpackAppName, "--guid")
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "failed to get app guid: %v\n", err)
		return
	}

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
