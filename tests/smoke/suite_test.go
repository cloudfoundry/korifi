package smoke_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/fail_handler"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/types"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	rest "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type SmokeTestSharedData struct {
	CfAdmin          string `json:"cf_admin"`
	RootNamespace    string `json:"root_namespace"`
	OrgName          string `json:"org_name"`
	BrokerOrgName    string `json:"broker_org_name"`
	SpaceName        string `json:"space_name"`
	AppsDomain       string `json:"apps_domain"`
	BuildpackAppName string `json:"buildpack_app_name"`
	DockerAppName    string `json:"docker_app_name"`
	BrokerURL        string `json:"broker_url"`
	FLockPath        string `json:"f_lock_path"`
}

var sharedData SmokeTestSharedData

func TestSmoke(t *testing.T) {
	RegisterFailHandler(fail_handler.New("CF CLI Tests",
		fail_handler.Hook{
			Matcher: fail_handler.Always,
			Hook: func(config *rest.Config, failure fail_handler.TestFailure) {
				printCfApp(config)
				fail_handler.PrintKorifiLogs(config, "", failure.StartTime)
				printBuildLogs(config)
			},
		}).Fail)

	SetDefaultEventuallyTimeout(helpers.EventuallyTimeout())
	SetDefaultEventuallyPollingInterval(helpers.EventuallyPollingInterval())
	RunSpecs(t, "CF CLI Tests Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	setCFHome(GinkgoParallelProcess())

	lockDir, err := os.MkdirTemp("", "")
	Expect(err).NotTo(HaveOccurred())

	sharedData = SmokeTestSharedData{
		CfAdmin:          uuid.NewString(),
		RootNamespace:    helpers.GetDefaultedEnvVar("ROOT_NAMESPACE", "cf"),
		OrgName:          uuid.NewString(),
		BrokerOrgName:    uuid.NewString(),
		SpaceName:        uuid.NewString(),
		AppsDomain:       helpers.GetRequiredEnvVar("APP_FQDN"),
		BuildpackAppName: uuid.NewString(),
		DockerAppName:    uuid.NewString(),
		FLockPath:        filepath.Join(lockDir, "lock"),
	}
	serviceAccountFactory := helpers.NewServiceAccountFactory(sharedData.RootNamespace)

	cfAdminToken := serviceAccountFactory.CreateAdminServiceAccount(sharedData.CfAdmin)
	helpers.AddUserToKubeConfig(sharedData.CfAdmin, cfAdminToken)

	Expect(helpers.Cf("api", helpers.GetRequiredEnvVar("API_SERVER_ROOT"), "--skip-ssl-validation")).To(Exit(0))
	Expect(helpers.Cf("auth", sharedData.CfAdmin)).To(Exit(0))

	brokerSpaceName := uuid.NewString()
	brokerAppName := uuid.NewString()
	Expect(helpers.Cf("create-org", sharedData.BrokerOrgName)).To(Exit(0))
	Expect(helpers.Cf("create-space", "-o", sharedData.BrokerOrgName, brokerSpaceName)).To(Exit(0))
	Expect(helpers.Cf("target", "-o", sharedData.BrokerOrgName, "-s", brokerSpaceName)).To(Exit(0))
	Expect(helpers.Cf("push", brokerAppName, "-p", "../assets/sample-broker")).To(Exit(0))
	sharedData.BrokerURL = helpers.GetInClusterURL(getAppGUID(brokerAppName))

	Expect(helpers.Cf("create-org", sharedData.OrgName)).To(Exit(0))
	Expect(helpers.Cf("create-space", "-o", sharedData.OrgName, sharedData.SpaceName)).To(Exit(0))
	Expect(helpers.Cf("target", "-o", sharedData.OrgName, "-s", sharedData.SpaceName)).To(Exit(0))

	Expect(helpers.Cf("push", sharedData.BuildpackAppName, "-p", "../assets/dorifi")).To(Exit(0))
	Expect(helpers.Cf("push", sharedData.DockerAppName, "-o", "eirini/dorini")).To(Exit(0))

	sharedDataBytes, err := json.Marshal(sharedData)
	Expect(err).NotTo(HaveOccurred())
	return sharedDataBytes
}, func(sharedDataBytes []byte) {
	sharedData = SmokeTestSharedData{}
	Expect(json.Unmarshal(sharedDataBytes, &sharedData)).To(Succeed())
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	setCFHome(GinkgoParallelProcess())

	Expect(helpers.Cf("api", helpers.GetRequiredEnvVar("API_SERVER_ROOT"), "--skip-ssl-validation")).To(Exit(0))
	Expect(helpers.Cf("auth", sharedData.CfAdmin)).To(Exit(0))

	Expect(helpers.Cf("delete-org", sharedData.OrgName, "-f").Wait()).To(Exit())
	Expect(helpers.Cf("delete-org", sharedData.BrokerOrgName, "-f").Wait()).To(Exit())
	serviceAccountFactory := helpers.NewServiceAccountFactory(sharedData.RootNamespace)

	serviceAccountFactory.DeleteServiceAccount(sharedData.CfAdmin)
	helpers.RemoveUserFromKubeConfig(sharedData.CfAdmin)

	Expect(os.RemoveAll(filepath.Dir(sharedData.FLockPath))).To(Succeed())
})

var _ = BeforeEach(func() {
	setCFHome(GinkgoParallelProcess())

	Expect(helpers.Cf("api", helpers.GetRequiredEnvVar("API_SERVER_ROOT"), "--skip-ssl-validation")).To(Exit(0))
	Expect(helpers.Cf("auth", sharedData.CfAdmin)).To(Exit(0))
	Expect(helpers.Cf("target", "-o", sharedData.OrgName, "-s", sharedData.SpaceName)).To(Exit(0))
})

func setCFHome(ginkgoNode int) {
	cfHomeDir, err := os.MkdirTemp("", fmt.Sprintf("ginkgo-%d", ginkgoNode))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		Expect(os.RemoveAll(cfHomeDir)).To(Succeed())
	})
	os.Setenv("CF_HOME", cfHomeDir)
}

func sessionOutput(session *Session) (string, error) {
	if session.ExitCode() != 0 {
		return "", fmt.Errorf("Session %v exited with exit code %d: %s",
			session.Command,
			session.ExitCode(),
			string(session.Err.Contents()),
		)
	}
	return strings.TrimSpace(string(session.Out.Contents())), nil
}

func appResponseShould(appName, requestPath string, matchExpectations types.GomegaMatcher) {
	GinkgoHelper()

	var httpClient http.Client
	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	Eventually(func(g Gomega) {
		resp, err := httpClient.Get(fmt.Sprintf("https://%s.%s%s", appName, sharedData.AppsDomain, requestPath))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(matchExpectations)
	}).Should(Succeed())
}

func printCfApp(config *rest.Config) {
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme.Scheme))
	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "failed to create k8s client: %v\n", err)
		return
	}

	cfAppNamespace, err := sessionOutput(helpers.Cf("space", sharedData.SpaceName, "--guid"))
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "failed to run 'cf space %s --guid': %v\n", sharedData.SpaceName, err)
		return
	}
	cfAppGUID, err := sessionOutput(helpers.Cf("app", sharedData.BuildpackAppName, "--guid"))
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "failed to run 'cf app %s --guid': %v\n", sharedData.BuildpackAppName, err)
		return
	}

	if err := printObject(k8sClient, &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfAppGUID,
			Namespace: cfAppNamespace,
		},
	}); err != nil {
		fmt.Fprintf(GinkgoWriter, "failed printing cfapp: %v\n", err)
		return
	}
}

func printObject(k8sClient client.Client, obj client.Object) error {
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
		return fmt.Errorf("failed to get object %q: %v\n", client.ObjectKeyFromObject(obj), err)
	}

	fmt.Fprintf(GinkgoWriter, "\n\n========== %T %s/%s (skipping managed fields) ==========\n", obj, obj.GetNamespace(), obj.GetName())
	obj.SetManagedFields([]metav1.ManagedFieldsEntry{})
	objBytes, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed marshalling object %v: %v\n", obj, err)
	}
	fmt.Fprintln(GinkgoWriter, string(objBytes))
	return nil
}

func printBuildLogs(config *rest.Config) {
	spaceGUID, err := sessionOutput(helpers.Cf("space", sharedData.SpaceName, "--guid").Wait())
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "failed to get space guid: %v\n", err)
		return
	}
	fail_handler.PrintAllBuildLogs(config, spaceGUID)
}

func getAppGUID(appName string) string {
	GinkgoHelper()

	session := helpers.Cf("app", appName, "--guid")
	Expect(session).To(Exit(0))
	return string(session.Out.Contents())
}

func matchSubstrings(substrings ...string) types.GomegaMatcher {
	return MatchRegexp(strings.Join(substrings, ".*"))
}
