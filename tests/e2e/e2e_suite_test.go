package e2e_test

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/fail_handler"

	"code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	correlationId string

	serviceAccountFactory *helpers.ServiceAccountFactory
	adminServiceAccount   string
	adminClient           *helpers.CorrelatedRestyClient

	apiServerRoot string

	rootNamespace           string
	appFQDN                 string
	commonTestOrgGUID       string
	brokerOrgGUID           string
	commonTestOrgName       string
	defaultAppBitsFile      string
	multiProcessAppBitsFile string
	serviceBrokerURL        string
)

type resource struct {
	Name          string            `json:"name,omitempty"`
	GUID          string            `json:"guid,omitempty"`
	Relationships relationships     `json:"relationships,omitempty"`
	CreatedAt     string            `json:"created_at,omitempty"`
	UpdatedAt     string            `json:"updated_at,omitempty"`
	Metadata      *metadata         `json:"metadata,omitempty"`
	Credentials   map[string]string `json:"credentials,omitempty"`
}

type relationships map[string]relationship

type relationship struct {
	Data resource `json:"data"`
}

type resourceList[T any] struct {
	Resources []T `json:"resources"`
}

type credentialsResponse struct {
	Credentials map[string]any `json:"credentials"`
}

type responseResource struct {
	Name      string    `json:"name,omitempty"`
	GUID      string    `json:"guid,omitempty"`
	CreatedAt string    `json:"created_at,omitempty"`
	UpdatedAt string    `json:"updated_at,omitempty"`
	Metadata  *metadata `json:"metadata,omitempty"`
}

type resourceListWithInclusion struct {
	Resources []resource    `json:"resources"`
	Included  *includedApps `json:",omitempty"`
}

type includedApps struct {
	Apps []resource `json:"apps"`
}

type bareResource struct {
	GUID string `json:"guid,omitempty"`
	Name string `json:"name,omitempty"`
}

type appResource struct {
	resource  `json:",inline"`
	Lifecycle *lifecycle `json:"lifecycle,omitempty"`
	State     string     `json:"state,omitempty"`
}

type taskResource struct {
	resource `json:",inline"`
	Command  string `json:"command,omitempty"`
	State    string `json:"state,omitempty"`
}

type typedResource struct {
	resource `json:",inline"`
	Type     string    `json:"type,omitempty"`
	Metadata *metadata `json:"metadata,omitempty"`
}

type packageResource struct {
	typedResource `json:",inline"`
	Data          *packageData `json:"data,omitempty"`
}

type packageData struct {
	Image string `json:"image"`
}

type metadata struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type mapRouteResource struct {
	Destinations []destinationRef `json:"destinations"`
}

type destinationRef struct {
	App resource `json:"app"`
}

type buildResource struct {
	resource `json:",inline"`
	Package  resource `json:"package"`
}

type dropletResource struct {
	Data resource `json:"data"`
}

type statsUsage struct {
	Time *string  `json:"time,omitempty"`
	CPU  *float64 `json:"cpu,omitempty"`
	Mem  *int64   `json:"mem,omitempty"`
	Disk *int64   `json:"disk,omitempty"`
}

type statsResource struct {
	Usage statsUsage `json:"usage"`
}

type manifestResource struct {
	Version      int                   `yaml:"version"`
	Applications []applicationResource `yaml:"applications"`
}

type applicationResource struct {
	Name         string                               `yaml:"name"`
	Command      string                               `yaml:"command"`
	DefaultRoute bool                                 `yaml:"default-route"`
	RandomRoute  bool                                 `yaml:"random-route"`
	NoRoute      bool                                 `yaml:"no-route"`
	Processes    []manifestApplicationProcessResource `yaml:"processes"`
	Routes       []manifestRouteResource              `yaml:"routes"`
	Memory       string                               `yaml:"memory,omitempty"`
	Metadata     metadata                             `yaml:"metadata,omitempty"`
	Buildpacks   []string                             `yaml:"buildpacks"`
	Services     []serviceResource                    `yaml:"services"`
	Docker       dockerResource                       `yaml:"docker,omitempty"`
}

type dockerResource struct {
	Image string `yaml:"image"`
}

type serviceResource struct {
	Name        string `yaml:"name"`
	BindingName string `yaml:"binding_name"`
}

type servicePlanResource struct {
	Name string `yaml:"name"`
	GUID string `yaml:"guid"`
}

type manifestApplicationProcessResource struct {
	Type    string  `yaml:"type"`
	Command *string `yaml:"command"`
}

type manifestRouteResource struct {
	Route *string `yaml:"route"`
}

type domainResource struct {
	resource `json:",inline"`
	Internal bool `json:"internal"`
}

type routeResource struct {
	resource `json:",inline"`
	Host     string `json:"host"`
	Path     string `json:"path"`
	URL      string `json:"url,omitempty"`
}

type destinationsResource struct {
	Destinations []destination `json:"destinations"`
}

type scaleResource struct {
	Instances  int `json:"instances,omitempty"`
	MemoryInMB int `json:"memory_in_mb,omitempty"`
	DiskInMB   int `json:"disk_in_mb,omitempty"`
}

type commandResource struct {
	Command string `json:"command"`
}

type destination struct {
	GUID string       `json:"guid"`
	App  bareResource `json:"app"`
}

type serviceInstanceResource struct {
	resource     `json:",inline"`
	Tags         []string       `json:"tags"`
	Credentials  map[string]any `json:"credentials"`
	InstanceType string         `json:"type"`
}

type securityGroupResource struct {
	Name  string `json:"name"`
	GUID  string `json:"guid,omitempty"`
	Rules []payloads.SecurityGroupRule
}

type relationshipDataResource struct {
	Data []payloads.RelationshipData `json:"data"`
}

type serviceBrokerResource struct {
	resource       `json:",inline"`
	URL            string                              `json:"url"`
	Authentication serviceBrokerAuthenticationResource `json:"authentication"`
}

type serviceBrokerAuthenticationResource struct {
	Type        string         `json:"type"`
	Credentials map[string]any `json:"credentials"`
}

type logCacheResponse struct {
	Envelopes envelopeBatch `json:"envelopes"`
}

type envelopeBatch struct {
	Batch []loggregator_v2.Envelope `json:"batch"`
}

type appUpdateResource struct {
	Metadata  *metadataPatch `json:"metadata,omitempty"`
	Lifecycle *lifecycle     `json:"lifecycle,omitempty"`
	Name      *string        `json:"name,omitempty"`
}

type lifecycle struct {
	Type string        `json:"type"`
	Data lifecycleData `json:"data"`
}

type lifecycleData struct {
	Buildpacks []string `json:"buildpacks"`
	Stack      string   `json:"stack"`
}

type cfErrs struct {
	Errors []cfErr
}

type processResource struct {
	resource  `json:",inline"`
	Type      string `json:"type"`
	Instances int    `json:"instances"`
	Command   string `yaml:"command"`
}

type metadataPatch struct {
	Annotations *map[string]string `json:"annotations,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
}

type metadataResource struct {
	Metadata *metadataPatch `json:"metadata,omitempty"`
}

type cfErr struct {
	Detail string `json:"detail"`
	Title  string `json:"title"`
	Code   int    `json:"code"`
}

type planVisibilityResource struct {
	Type          string                            `json:"type"`
	Organizations []payloads.VisibilityOrganization `json:"organizations"`
}

type buildpackResource struct {
	resource `json:",inline"`
	Stack    string `json:"stack"`
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(fail_handler.New("E2E Tests",
		fail_handler.Hook{
			Matcher: fail_handler.Always,
			Hook: func(config *rest.Config, failure fail_handler.TestFailure) {
				fail_handler.PrintKorifiLogs(config, correlationId, failure.StartTime)
			},
		},
		fail_handler.Hook{
			Matcher: ContainSubstring("Droplet not found"),
			Hook: func(config *rest.Config, failure fail_handler.TestFailure) {
				printDropletNotFoundDebugInfo(config, failure.Message)
			},
		},
		fail_handler.Hook{
			Matcher: ContainSubstring("404"),
			Hook: func(config *rest.Config, _ fail_handler.TestFailure) {
				printAllRoleBindings(config)
			},
		},
	).Fail)
	RunSpecs(t, "E2E Suite")
}

type sharedSetupData struct {
	CommonOrgName            string `json:"commonOrgName"`
	CommonOrgGUID            string `json:"commonOrgGuid"`
	DefaultAppBitsFile       string `json:"defaultAppBitsFile"`
	ServiceBrokerURL         string `json:"service_broker_url"`
	MultiProcessAppBitsFile  string `json:"multiProcessAppBitsFile"`
	AdminServiceAccount      string `json:"admin_service_account"`
	AdminServiceAccountToken string `json:"admin_service_account_token"`
	BrokerOrgGUID            string `json:"broker_org_guid"`
}

var _ = SynchronizedBeforeSuite(func() []byte {
	commonTestSetup()

	adminServiceAccount = uuid.NewString()
	adminServiceAccountToken := serviceAccountFactory.CreateAdminServiceAccount(adminServiceAccount)

	adminClient = makeTokenClient(adminServiceAccountToken)

	commonTestOrgName = generateGUID("common-test-org")
	commonTestOrgGUID = createOrg(commonTestOrgName)
	brokerOrgGUID = createOrg(uuid.NewString())

	sharedData := sharedSetupData{
		CommonOrgName: commonTestOrgName,
		CommonOrgGUID: commonTestOrgGUID,
		DefaultAppBitsFile: helpers.ZipDirectory(
			helpers.GetDefaultedEnvVar("DEFAULT_APP_BITS_PATH", "../assets/dorifi"),
		),
		ServiceBrokerURL: pushSampleBroker(helpers.ZipDirectory(
			helpers.GetDefaultedEnvVar("DEFAULT_SERVICE_BROKER_BITS_PATH", "../assets/sample-broker"),
		)),
		MultiProcessAppBitsFile:  helpers.ZipDirectory("../assets/multi-process"),
		AdminServiceAccount:      adminServiceAccount,
		AdminServiceAccountToken: adminServiceAccountToken,
		BrokerOrgGUID:            brokerOrgGUID,
	}

	bs, err := json.Marshal(sharedData)
	Expect(err).NotTo(HaveOccurred())

	return bs
}, func(bs []byte) {
	commonTestSetup()

	var sharedSetup sharedSetupData
	err := json.Unmarshal(bs, &sharedSetup)
	Expect(err).NotTo(HaveOccurred())

	commonTestOrgGUID = sharedSetup.CommonOrgGUID
	commonTestOrgName = sharedSetup.CommonOrgName
	defaultAppBitsFile = sharedSetup.DefaultAppBitsFile
	serviceBrokerURL = sharedSetup.ServiceBrokerURL
	multiProcessAppBitsFile = sharedSetup.MultiProcessAppBitsFile
	adminServiceAccount = sharedSetup.AdminServiceAccount
	adminClient = makeTokenClient(sharedSetup.AdminServiceAccountToken)
	brokerOrgGUID = sharedSetup.BrokerOrgGUID

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	expectJobCompletes(deleteOrg(commonTestOrgGUID))
	expectJobCompletes(deleteOrg(brokerOrgGUID))
	serviceAccountFactory.DeleteServiceAccount(adminServiceAccount)
})

var _ = BeforeEach(func() {
	correlationId = uuid.NewString()
})

func makeCertClientForUserName(userName string, validFor time.Duration) *helpers.CorrelatedRestyClient {
	GinkgoHelper()

	disallowed, found := os.LookupEnv("CSR_SIGNING_DISALLOWED")
	if found && disallowed == "true" {
		Skip("CSR singing is not allowed on this environment")
	}

	return helpers.NewCorrelatedRestyClient(apiServerRoot, getCorrelationId).
		SetAuthScheme("ClientCert").
		SetAuthToken(base64.StdEncoding.EncodeToString(helpers.CreateTrustedCertificatePEM(userName, validFor))).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
}

func makeTokenClient(token string) *helpers.CorrelatedRestyClient {
	return helpers.NewCorrelatedRestyClient(apiServerRoot, getCorrelationId).
		SetAuthScheme("Bearer").
		SetAuthToken(token).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
}

func ensureServerIsUp() {
	GinkgoHelper()

	Eventually(func() (int, error) {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		resp, err := http.Get(apiServerRoot)
		if err != nil {
			return 0, err
		}

		resp.Body.Close()

		return resp.StatusCode, nil
	}, "5m").Should(Equal(http.StatusOK), "API Server at %s was not running after 5 minutes", apiServerRoot)
}

func generateGUID(prefix string) string {
	guid := uuid.NewString()

	return fmt.Sprintf("%s-%s", prefix, guid[:13])
}

func deleteOrg(guid string) *resty.Response {
	GinkgoHelper()

	if guid == "" {
		return nil
	}

	resp, err := adminClient.R().
		Delete("/v3/organizations/" + guid)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusAccepted))
	expectJobCompletes(resp)

	return resp
}

func createOrgRaw(orgName string) (string, error) {
	var org resource
	resp, err := adminClient.R().
		SetBody(resource{Name: orgName}).
		SetResult(&org).
		Post("/v3/organizations")
	if err != nil {
		return "", err
	}
	if resp.StatusCode() != http.StatusCreated {
		return "", fmt.Errorf("expected status code %d, got %d, body: %s", http.StatusCreated, resp.StatusCode(), string(resp.Body()))
	}

	return org.GUID, nil
}

func createOrg(orgName string) string {
	GinkgoHelper()

	orgGUID, err := createOrgRaw(orgName)
	Expect(err).NotTo(HaveOccurred())

	return orgGUID
}

func asyncCreateOrg(orgName string, createdOrgGUID *string, wg *sync.WaitGroup, errChan chan error) {
	go func() {
		defer wg.Done()
		defer GinkgoRecover()

		var err error
		*createdOrgGUID, err = createOrgRaw(orgName)
		if err != nil {
			errChan <- err
			return
		}
	}()
}

func createSpaceRaw(spaceName, orgGUID string) (string, error) {
	var space resource
	resp, err := adminClient.R().
		SetBody(resource{
			Name: spaceName,
			Relationships: relationships{
				"organization": relationship{Data: resource{GUID: orgGUID}},
			},
		}).
		SetResult(&space).
		Post("/v3/spaces")
	if err != nil {
		return "", err
	}

	if resp.StatusCode() != http.StatusCreated {
		return "", fmt.Errorf("expected status code %d, got %d, body: %s", http.StatusCreated, resp.StatusCode(), string(resp.Body()))
	}

	return space.GUID, nil
}

func deleteSpace(guid string) {
	GinkgoHelper()

	if guid == "" {
		return
	}

	resp, err := adminClient.R().
		Delete("/v3/spaces/" + guid)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusAccepted))
	expectJobCompletes(resp)
}

func createSpace(spaceName, orgGUID string) string {
	GinkgoHelper()

	spaceGUID, err := createSpaceRaw(spaceName, orgGUID)
	Expect(err).NotTo(HaveOccurred(), `create space "`+spaceName+`" in orgGUID "`+orgGUID+`" should have succeeded`)

	return spaceGUID
}

func applySpaceManifest(manifest manifestResource, spaceGUID string) {
	GinkgoHelper()

	manifestBytes, err := yaml.Marshal(manifest)
	Expect(err).NotTo(HaveOccurred())
	resp, err := adminClient.R().
		SetHeader("Content-type", "application/x-yaml").
		SetBody(manifestBytes).
		Post("/v3/spaces/" + spaceGUID + "/actions/apply_manifest")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusAccepted))
}

func asyncCreateSpace(spaceName, orgGUID string, createdSpaceGUID *string, wg *sync.WaitGroup, errChan chan error) {
	go func() {
		defer wg.Done()
		defer GinkgoRecover()

		var err error
		*createdSpaceGUID, err = createSpaceRaw(spaceName, orgGUID)
		if err != nil {
			errChan <- err
			return
		}
	}()
}

func createRoleRaw(roleName, orgSpaceType, userName, orgSpaceGUID string) (string, error) {
	rolesURL := apiServerRoot + "/v3/roles"

	payload := typedResource{
		Type: roleName,
		resource: resource{
			Relationships: relationships{
				"user":       relationship{Data: resource{GUID: userName}},
				orgSpaceType: relationship{Data: resource{GUID: orgSpaceGUID}},
			},
		},
	}

	var resultErr cfErrs
	var createdRole typedResource
	resp, err := adminClient.R().
		SetBody(payload).
		SetResult(&createdRole).
		SetError(&resultErr).
		Post(rolesURL)
	if err != nil {
		return "", err
	}

	if resp.StatusCode() != http.StatusCreated {
		return "", fmt.Errorf("Failed to create %s role %q for user %q in %s %q; status code %d; response: %q", orgSpaceType, roleName, userName, orgSpaceType, orgSpaceGUID, resp.StatusCode(), string(resp.Body()))
	}

	return createdRole.GUID, nil
}

// createRole creates an org or space role
// You should probably invoke this via createOrgRole or createSpaceRole
func createRole(roleName, orgSpaceType, userName, orgSpaceGUID string) string {
	GinkgoHelper()

	roleGuid, err := createRoleRaw(roleName, orgSpaceType, userName, orgSpaceGUID)
	Expect(err).NotTo(HaveOccurred())
	return roleGuid
}

func createOrgRole(roleName, userName, orgGUID string) string {
	GinkgoHelper()

	return createRole(roleName, "organization", userName, orgGUID)
}

func createSpaceRole(roleName, userName, spaceGUID string) string {
	GinkgoHelper()

	return createRole(roleName, "space", userName, spaceGUID)
}

func createBuildpackApp(spaceGUID, name string) string {
	GinkgoHelper()

	return createApp(appResource{
		resource: resource{
			Name:          name,
			Relationships: relationships{"space": {Data: resource{GUID: spaceGUID}}},
		},
	})
}

func createApp(app appResource) string {
	GinkgoHelper()

	var result resource

	resp, err := adminClient.R().
		SetBody(app).
		SetResult(&result).
		Post("/v3/apps")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
	Expect(result.GUID).NotTo(BeEmpty())
	Expect(result.Name).To(Equal(app.Name))

	return result.GUID
}

func setEnv(appName string, envVars map[string]interface{}) {
	GinkgoHelper()

	resp, err := adminClient.R().
		SetBody(
			struct {
				Var map[string]interface{} `json:"var"`
			}{
				Var: envVars,
			},
		).
		SetPathParam("appName", appName).
		Patch("/v3/apps/{appName}/environment_variables")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
}

func getAppEnv(appName string) map[string]interface{} {
	GinkgoHelper()

	var env map[string]interface{}

	resp, err := adminClient.R().
		SetResult(&env).
		SetPathParam("appName", appName).
		Get("/v3/apps/{appName}/env")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

	return env
}

func getApp(appGUID string) appResource {
	GinkgoHelper()

	var app appResource
	Eventually(func(g Gomega) {
		resp, err := adminClient.R().
			SetResult(&app).
			Get("/v3/apps/" + appGUID)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
	}).Should(Succeed())

	return app
}

func getProcess(appGUID, processType string) processResource {
	GinkgoHelper()

	var process processResource
	Eventually(func(g Gomega) {
		resp, err := adminClient.R().
			SetResult(&process).
			Get("/v3/apps/" + appGUID + "/processes/" + processType)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
	}).Should(Succeed())

	return process
}

func createServiceInstance(spaceGUID, name string, credentials map[string]string) string {
	GinkgoHelper()

	var serviceInstance typedResource

	resp, err := adminClient.R().
		SetBody(typedResource{
			Type: "user-provided",
			resource: resource{
				Name:          name,
				Relationships: relationships{"space": {Data: resource{GUID: spaceGUID}}},
				Credentials:   credentials,
			},
		}).
		SetResult(&serviceInstance).
		Post("/v3/service_instances")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))

	return serviceInstance.GUID
}

func listServiceInstances(names ...string) resourceList[serviceInstanceResource] {
	GinkgoHelper()

	var namesQuery string
	if len(names) > 0 {
		namesQuery = "?names=" + strings.Join(names, ",")
	}

	var serviceInstances resourceList[serviceInstanceResource]
	resp, err := adminClient.R().
		SetResult(&serviceInstances).
		Get("/v3/service_instances" + namesQuery)

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))

	return serviceInstances
}

func createUPSIServiceBinding(appGUID, instanceGUID, bindingName string) string {
	GinkgoHelper()

	var serviceCredentialBinding resource

	resp, err := adminClient.R().
		SetBody(typedResource{
			Type: "app",
			resource: resource{
				Name:          bindingName,
				Relationships: relationships{"app": {Data: resource{GUID: appGUID}}, "service_instance": {Data: resource{GUID: instanceGUID}}},
			},
		}).
		SetResult(&serviceCredentialBinding).
		Post("/v3/service_credential_bindings")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated), string(resp.Body()))

	return serviceCredentialBinding.GUID
}

func createManagedServiceBinding(appGUID, instanceGUID, bindingName string) string {
	GinkgoHelper()

	resp, err := adminClient.R().
		SetBody(typedResource{
			Type: "app",
			resource: resource{
				Name:          bindingName,
				Relationships: relationships{"app": {Data: resource{GUID: appGUID}}, "service_instance": {Data: resource{GUID: instanceGUID}}},
			},
		}).
		Post("/v3/service_credential_bindings")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(SatisfyAll(
		HaveRestyStatusCode(http.StatusAccepted),
		HaveRestyHeaderWithValue("Location", ContainSubstring("/v3/jobs/managed_service_binding.create~")),
	))
	jobURL := resp.Header().Get("Location")
	expectJobCompletes(resp)

	jobURLSplit := strings.Split(jobURL, "~")
	Expect(jobURLSplit).To(HaveLen(2))
	return jobURLSplit[1]
}

func getServiceBindingsForApp(appGUID string) []resource {
	GinkgoHelper()

	var serviceBindings resourceList[resource]
	resp, err := adminClient.R().
		SetResult(&serviceBindings).
		Get("/v3/service_credential_bindings?app_guids=" + appGUID)

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK), string(resp.Body()))

	return serviceBindings.Resources
}

func createBitsPackage(appGUID string) string {
	GinkgoHelper()

	return createPackage(packageResource{
		typedResource: typedResource{
			Type: "bits",
			resource: resource{
				Relationships: relationships{
					"app": relationship{Data: resource{GUID: appGUID}},
				},
			},
		},
	})
}

func createPackage(pkg packageResource) string {
	var result resource
	resp, err := adminClient.R().
		SetBody(pkg).
		SetResult(&result).
		Post("/v3/packages")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))

	return result.GUID
}

func createBuild(packageGUID string) string {
	GinkgoHelper()

	var build resource

	resp, err := adminClient.R().
		SetBody(buildResource{Package: resource{GUID: packageGUID}}).
		SetResult(&build).
		Post("/v3/builds")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))

	return build.GUID
}

func createDeployment(appGUID string) string {
	GinkgoHelper()

	var deployment resource

	resp, err := adminClient.R().
		SetBody(resource{
			Relationships: relationships{
				"app": relationship{
					Data: resource{
						GUID: appGUID,
					},
				},
			},
		}).
		SetResult(&deployment).
		Post("/v3/deployments")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))

	return deployment.GUID
}

func waitForDroplet(buildGUID string) {
	GinkgoHelper()

	Eventually(func() (*resty.Response, error) {
		resp, err := adminClient.R().
			Get("/v3/droplets/" + buildGUID)
		return resp, err
	}).Should(HaveRestyStatusCode(http.StatusOK))
}

func setCurrentDroplet(appGUID, dropletGUID string) {
	GinkgoHelper()

	resp, err := adminClient.R().
		SetBody(dropletResource{Data: resource{GUID: dropletGUID}}).
		Patch("/v3/apps/" + appGUID + "/relationships/current_droplet")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
}

func waitAppStaged(appGUID string) {
	var currentDroplet struct {
		GUID  string `json:"guid"`
		State string `json:"state"`
	}

	Eventually(func(g Gomega) {
		resp, err := adminClient.R().
			SetResult(&currentDroplet).
			Get("/v3/apps/" + appGUID + "/droplets/current")
		Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
		fmt.Fprintf(GinkgoWriter, "currentDroplet = %+v\n", currentDroplet)
		g.Expect(currentDroplet.GUID).NotTo(BeEmpty())
		g.Expect(currentDroplet.State).To(Equal("STAGED"))
	}).Should(Succeed())
}

func appAction(appGUID, action string) {
	GinkgoHelper()

	resp, err := adminClient.R().
		Post("/v3/apps/" + appGUID + "/actions/" + action)

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
}

func restartApp(appGUID string) {
	GinkgoHelper()
	appAction(appGUID, "restart")
}

func uploadTestApp(pkgGUID, appBitsFile string) {
	GinkgoHelper()

	resp, err := adminClient.R().
		SetFiles(map[string]string{
			"bits": appBitsFile,
		}).Post("/v3/packages/" + pkgGUID + "/upload")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
}

func getAppGUIDFromName(appName string) string {
	GinkgoHelper()

	var appGUID string
	Eventually(func(g Gomega) {
		var result resourceList[resource]
		resp, err := adminClient.R().SetResult(&result).Get("/v3/apps?names=" + appName)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
		g.Expect(result.Resources).To(HaveLen(1))
		appGUID = result.Resources[0].GUID
	}).Should(Succeed())

	return appGUID
}

func createAppViaManifest(spaceGUID, appName string) string {
	GinkgoHelper()

	manifest := manifestResource{
		Version: 1,
		Applications: []applicationResource{{
			Name:         appName,
			Memory:       "128MB",
			DefaultRoute: true,
		}},
	}
	applySpaceManifest(manifest, spaceGUID)

	return getAppGUIDFromName(appName)
}

func pushTestApp(spaceGUID, appBitsFile string) (string, string) {
	GinkgoHelper()

	appName := generateGUID("app")
	return pushTestAppWithName(spaceGUID, appBitsFile, appName), appName
}

func pushTestAppWithName(spaceGUID, appBitsFile string, appName string) string {
	GinkgoHelper()

	appGUID := createAppViaManifest(spaceGUID, appName)
	pkgGUID := createBitsPackage(appGUID)
	uploadTestApp(pkgGUID, appBitsFile)
	buildGUID := createBuild(pkgGUID)
	waitForDroplet(buildGUID)
	setCurrentDroplet(appGUID, buildGUID)
	waitAppStaged(appGUID)
	restartApp(appGUID)

	return appGUID
}

func getAppRoute(appGUID string) string {
	GinkgoHelper()

	var routes resourceList[routeResource]
	resp, err := adminClient.R().
		SetResult(&routes).
		Get("/v3/apps/" + appGUID + "/routes")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
	Expect(routes.Resources).ToNot(BeEmpty())
	return routes.Resources[0].URL
}

var skipSSLClient = http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
}

func curlApp(appGUID string, pathSegments ...string) string {
	GinkgoHelper()

	url := getAppRoute(appGUID)
	var body []byte
	Eventually(func(g Gomega) {
		r, err := skipSSLClient.Get("https://" + url + path.Join(pathSegments...))
		g.Expect(err).NotTo(HaveOccurred())
		defer r.Body.Close()
		g.Expect(r).To(HaveHTTPStatus(http.StatusOK))
		body, err = io.ReadAll(r.Body)
		g.Expect(err).NotTo(HaveOccurred())
	}).Should(Succeed())

	return string(body)
}

func curlAppJSON(appGUID string, pathSegments ...string) map[string]any {
	data := map[string]interface{}{}
	Expect(json.Unmarshal([]byte(curlApp(appGUID, pathSegments...)), &data)).To(Succeed())
	return data
}

func getDomainGUID(domainName string) string {
	GinkgoHelper()

	res := resourceList[bareResource]{}
	resp, err := adminClient.R().
		SetResult(&res).
		Get("/v3/domains")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

	for _, d := range res.Resources {
		if d.Name == domainName {
			return d.GUID
		}
	}

	Fail(fmt.Sprintf("no domain found for domainName: %q", domainName))

	return ""
}

func createRoute(host, path string, spaceGUID, domainGUID string) string {
	GinkgoHelper()

	var route resource

	resp, err := adminClient.R().
		SetBody(routeResource{
			Host: host,
			Path: path,
			resource: resource{
				Relationships: relationships{
					"domain": {Data: resource{GUID: domainGUID}},
					"space":  {Data: resource{GUID: spaceGUID}},
				},
			},
		}).
		SetResult(&route).
		Post("/v3/routes")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))

	return route.GUID
}

func addDestinationForRoute(appGUID, routeGUID string) []string {
	GinkgoHelper()

	var destinations destinationsResource

	resp, err := adminClient.R().
		SetBody(mapRouteResource{
			Destinations: []destinationRef{
				{App: resource{GUID: appGUID}},
			},
		}).
		SetResult(&destinations).
		Post("/v3/routes/" + routeGUID + "/destinations")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))

	var destinationGUIDs []string
	for _, destination := range destinations.Destinations {
		destinationGUIDs = append(destinationGUIDs, destination.GUID)
	}

	return destinationGUIDs
}

func commonTestSetup() {
	SetDefaultEventuallyTimeout(helpers.EventuallyTimeout())
	SetDefaultEventuallyPollingInterval(helpers.EventuallyPollingInterval())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	apiServerRoot = helpers.GetRequiredEnvVar("API_SERVER_ROOT")
	rootNamespace = helpers.GetRequiredEnvVar("ROOT_NAMESPACE")

	appFQDN = helpers.GetRequiredEnvVar("APP_FQDN")

	ensureServerIsUp()

	serviceAccountFactory = helpers.NewServiceAccountFactory(rootNamespace)
}

func getCorrelationId() string {
	return correlationId
}

func printDropletNotFoundDebugInfo(config *rest.Config, message string) {
	fmt.Fprint(GinkgoWriter, "\n\n========== Droplet not found debug log (start) ==========\n")

	dropletGUID, err := getDropletGUID(message)
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "Failed to get droplet GUID from message %v\n", err)
		return
	}

	fail_handler.PrintBuildLogs(config, dropletGUID)
	fmt.Fprint(GinkgoWriter, "\n\n========== Droplet not found debug log (end) ==========\n\n")
}

func getDropletGUID(message string) (string, error) {
	r := regexp.MustCompile(`Request.*droplets/(.*)`)
	matches := r.FindStringSubmatch(message)
	if len(matches) != 2 {
		return "", fmt.Errorf("message does not match regex: %s", r.String())
	}

	return matches[1], nil
}

func printAllRoleBindings(config *rest.Config) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "failed to create clientset: %v\n", err)
		return
	}

	list, err := clientset.RbacV1().RoleBindings("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("failed getting rolebindings: %v", err)
		return
	}

	fmt.Fprint(GinkgoWriter, "\n\n========== Expected 404 debug log ==========\n\n")
	for _, b := range list.Items {
		fmt.Fprintf(GinkgoWriter, "Name: %s, Namespace: %s, RoleKind: %s, RoleName: %s, Subjects: \n",
			b.Name, b.Namespace, b.RoleRef.Kind, b.RoleRef.Name)
		for _, s := range b.Subjects {
			fmt.Fprintf(GinkgoWriter, "\tKind: %s, Name: %s, Namespace: %s\n", s.Kind, s.Name, s.Namespace)
		}
	}
	fmt.Fprint(GinkgoWriter, "\n\n========== Expected 404 debug log (end) ==========\n\n")
}

func pushSampleBroker(brokerBitsFile string) string {
	brokerSpaceGUID := createSpace(uuid.NewString(), brokerOrgGUID)
	brokerAppGUID, _ := pushTestApp(brokerSpaceGUID, brokerBitsFile)
	Expect(curlApp(brokerAppGUID)).To(ContainSubstring("Hi, I'm the sample broker!"))
	return helpers.GetInClusterURL(brokerAppGUID)
}

func createBrokerAsync(brokerURL, username, password string) (string, string) {
	resp, err := adminClient.R().
		SetBody(serviceBrokerResource{
			resource: resource{
				Name: uuid.NewString(),
			},
			URL: brokerURL,
			Authentication: serviceBrokerAuthenticationResource{
				Type: "basic",
				Credentials: map[string]any{
					"username": username,
					"password": password,
				},
			},
		}).
		Post("/v3/service_brokers")
	Expect(err).NotTo(HaveOccurred())

	Expect(resp).To(SatisfyAll(
		HaveRestyStatusCode(http.StatusAccepted),
		HaveRestyHeaderWithValue("Location", ContainSubstring("/v3/jobs/service_broker.create~")),
	))

	jobURL := resp.Header().Get("Location")
	jobURLSplit := strings.Split(jobURL, "~")
	Expect(jobURLSplit).To(HaveLen(2))
	brokerGUID := jobURLSplit[1]
	return brokerGUID, jobURL
}

func createBroker(brokerURL string) string {
	brokerGUID, jobURL := createBrokerAsync(brokerURL, "broker-user", "broker-password")
	Eventually(func(g Gomega) {
		resp, err := adminClient.R().Get(jobURL)
		g.Expect(err).NotTo(HaveOccurred())
		jobRespBody := string(resp.Body())
		g.Expect(jobRespBody).To(ContainSubstring("COMPLETE"))
	}).Should(Succeed())

	plans := resourceList[resource]{}
	listResp, err := adminClient.R().SetResult(&plans).Get("/v3/service_plans?service_broker_guids=" + brokerGUID)
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp).To(HaveRestyStatusCode(http.StatusOK))

	for _, plan := range plans.Resources {
		resp, err := adminClient.R().
			SetBody(planVisibilityResource{
				Type: "public",
			}).
			Post(fmt.Sprintf("/v3/service_plans/%s/visibility", plan.GUID))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(SatisfyAll(
			HaveRestyStatusCode(http.StatusOK),
		))
	}

	return brokerGUID
}

func expectJobCompletes(resp *resty.Response) {
	GinkgoHelper()

	Expect(resp).To(SatisfyAll(
		HaveRestyHeaderWithValue("Location", Not(BeEmpty())),
	))

	jobURL := resp.Header().Get("Location")
	Eventually(func(g Gomega) {
		jobResp, err := adminClient.R().Get(jobURL)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(jobResp.Body())).To(ContainSubstring("COMPLETE"))
	}).Should(Succeed())
}
