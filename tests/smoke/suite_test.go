package smoke_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/broker"
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
	SpaceName        string `json:"space_name"`
	AppsDomain       string `json:"apps_domain"`
	BuildpackAppName string `json:"buildpack_app_name"`
	DockerAppName    string `json:"docker_app_name"`
	BrokerAppName    string `json:"broker_app_name"`
}

var sharedData SmokeTestSharedData

func TestSmoke(t *testing.T) {
	RegisterFailHandler(fail_handler.New("CF CLI Tests",
		fail_handler.Hook{
			Matcher: fail_handler.Always,
			Hook: func(config *rest.Config, failure fail_handler.TestFailure) {
				printCfApp(config)
				fail_handler.PrintKorifiLogs(config, "", failure.StartTime)
				printBuildLogs(config, sharedData.SpaceName)
			},
		}).Fail)

	SetDefaultEventuallyTimeout(helpers.EventuallyTimeout())
	SetDefaultEventuallyPollingInterval(helpers.EventuallyPollingInterval())
	RunSpecs(t, "CF CLI Tests Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	data := SmokeTestSharedData{
		CfAdmin:          uuid.NewString(),
		RootNamespace:    helpers.GetDefaultedEnvVar("ROOT_NAMESPACE", "cf"),
		OrgName:          uuid.NewString(),
		SpaceName:        uuid.NewString(),
		AppsDomain:       helpers.GetRequiredEnvVar("APP_FQDN"),
		BuildpackAppName: uuid.NewString(),
		DockerAppName:    uuid.NewString(),
		BrokerAppName:    uuid.NewString(),
	}
	serviceAccountFactory := helpers.NewServiceAccountFactory(data.RootNamespace)

	cfAdminToken := serviceAccountFactory.CreateAdminServiceAccount(data.CfAdmin)
	helpers.AddUserToKubeConfig(data.CfAdmin, cfAdminToken)

	Expect(helpers.Cf("api", helpers.GetRequiredEnvVar("API_SERVER_ROOT"), "--skip-ssl-validation")).To(Exit(0))
	Expect(helpers.Cf("auth", data.CfAdmin)).To(Exit(0))

	Expect(helpers.Cf("create-org", data.OrgName)).To(Exit(0))
	Expect(helpers.Cf("create-space", "-o", data.OrgName, data.SpaceName)).To(Exit(0))
	Expect(helpers.Cf("target", "-o", data.OrgName, "-s", data.SpaceName)).To(Exit(0))

	Expect(helpers.Cf("push", data.BuildpackAppName, "-p", "../assets/dorifi")).To(Exit(0))
	Expect(helpers.Cf("push", data.BrokerAppName, "-p", "../assets/sample-broker")).To(Exit(0))
	Expect(helpers.Cf("push", data.DockerAppName, "-o", "eirini/dorini")).To(Exit(0))

	sharedDataBytes, err := json.Marshal(data)
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
	Expect(helpers.Cf("delete-org", sharedData.OrgName, "-f").Wait()).To(Exit())
	serviceAccountFactory := helpers.NewServiceAccountFactory(sharedData.RootNamespace)

	serviceAccountFactory.DeleteServiceAccount(sharedData.CfAdmin)
	helpers.RemoveUserFromKubeConfig(sharedData.CfAdmin)
})

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

func printBuildLogs(config *rest.Config, spaceName string) {
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

func cleanupBroker(brokerName string) {
	GinkgoHelper()

	Expect(helpers.Cf("delete-service-broker", "-f", brokerName)).To(Exit(0))
	broker.NewCatalogPurger(sharedData.RootNamespace).ForBrokerName(brokerName).Purge()
}

func matchSubstrings(substrings ...string) types.GomegaMatcher {
	return MatchRegexp(strings.Join(substrings, ".*"))
}
