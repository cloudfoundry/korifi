package smoke_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/fail_handler"
	"github.com/cloudfoundry/cf-test-helpers/cf"
	"github.com/cloudfoundry/cf-test-helpers/generator"
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
)

const NamePrefix = "cf-on-k8s-smoke"

var (
	orgName          string
	spaceName        string
	appName          string
	appsDomain       string
	appRouteProtocol string
)

func TestSmoke(t *testing.T) {
	RegisterFailHandler(fail_handler.New("Smoke Tests", map[gomegatypes.GomegaMatcher]func(*rest.Config){
		fail_handler.Always: func(config *rest.Config) {
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
	apiArguments := []string{"api", helpers.GetRequiredEnvVar("SMOKE_TEST_API_ENDPOINT")}
	skipSSL := os.Getenv("SMOKE_TEST_SKIP_SSL") == "true"
	if skipSSL {
		apiArguments = append(apiArguments, "--skip-ssl-validation")
	}

	Eventually(cf.Cf(apiArguments...)).Should(Exit(0))

	loginAs(helpers.GetRequiredEnvVar("SMOKE_TEST_USER"))

	appRouteProtocol = helpers.GetDefaultedEnvVar("SMOKE_TEST_APP_ROUTE_PROTOCOL", "https")
	appsDomain = helpers.GetRequiredEnvVar("SMOKE_TEST_APPS_DOMAIN")
	orgName = generator.PrefixedRandomName(NamePrefix, "org")
	spaceName = generator.PrefixedRandomName(NamePrefix, "space")
	appName = generator.PrefixedRandomName(NamePrefix, "app")

	Eventually(cf.Cf("create-org", orgName)).Should(Exit(0))
	Eventually(cf.Cf("create-space", "-o", orgName, spaceName)).Should(Exit(0))
	Eventually(cf.Cf("target", "-o", orgName, "-s", spaceName)).Should(Exit(0))

	Eventually(
		cf.Cf("push", appName, "-p", "../assets/dorifi"),
	).Should(Exit(0))
})

var _ = AfterSuite(func() {
	if CurrentSpecReport().State.Is(types.SpecStateFailed) {
		printAppReport(appName)
	}

	if orgName != "" {
		Eventually(func() *Session {
			return cf.Cf("delete-org", orgName, "-f").Wait()
		}).Should(Exit(0))
	}
})

func runCfCmd(args ...string) (string, error) {
	session := cf.Cf(args...)
	<-session.Exited
	if session.ExitCode() != 0 {
		return "", fmt.Errorf("cf %s exited with code %d", strings.Join(args, " "), session.ExitCode())
	}

	return string(session.Out.Contents()), nil
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

	cfAppGUID, err := runCfCmd("app", appName, "--guid")
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
