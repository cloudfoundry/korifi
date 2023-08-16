package e2e_test

import (
	"archive/zip"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/fail_handler"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
	commonTestOrgName       string
	assetsTmpDir            string
	defaultAppBitsFile      string
	multiProcessAppBitsFile string
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
	Usage statsUsage
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
}

type serviceResource struct {
	Name        string `yaml:"name"`
	BindingName string `yaml:"binding_name"`
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
	Tags         []string          `json:"tags"`
	Credentials  map[string]string `json:"credentials"`
	InstanceType string            `json:"type"`
}

type appLogResource struct {
	Envelopes appLogResourceEnvelopes `json:"envelopes"`
}

type appLogResourceEnvelopes struct {
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

func TestE2E(t *testing.T) {
	RegisterFailHandler(fail_handler.New("E2E Tests", map[types.GomegaMatcher]func(*rest.Config, string){
		fail_handler.Always: func(config *rest.Config, _ string) {
			fail_handler.PrintPodsLogs(config, []fail_handler.PodContainerDescriptor{
				{
					Namespace:     systemNamespace(),
					LabelKey:      "app",
					LabelValue:    "korifi-api",
					Container:     "korifi-api",
					CorrelationId: correlationId,
				},
				{
					Namespace:  systemNamespace(),
					LabelKey:   "app",
					LabelValue: "korifi-controllers",
					Container:  "manager",
				},
			})
		},
		ContainSubstring("Droplet not found"): func(config *rest.Config, message string) {
			printDropletNotFoundDebugInfo(config, message)
		},
		ContainSubstring("404"): func(config *rest.Config, _ string) {
			printAllRoleBindings(config)
		},
	}))
	RunSpecs(t, "E2E Suite")
}

type sharedSetupData struct {
	CommonOrgName            string `json:"commonOrgName"`
	CommonOrgGUID            string `json:"commonOrgGuid"`
	DefaultAppBitsFile       string `json:"defaultAppBitsFile"`
	MultiProcessAppBitsFile  string `json:"multiProcessAppBitsFile"`
	AdminServiceAccount      string `json:"admin_service_account"`
	AdminServiceAccountToken string `json:"admin_service_account_token"`
}

var _ = SynchronizedBeforeSuite(func() []byte {
	commonTestSetup()

	adminServiceAccount = uuid.NewString()
	adminServiceAccountToken := serviceAccountFactory.CreateAdminServiceAccount(adminServiceAccount)

	adminClient = makeTokenClient(adminServiceAccountToken)

	commonTestOrgName = generateGUID("common-test-org")
	commonTestOrgGUID = createOrg(commonTestOrgName)

	var err error
	assetsTmpDir, err = os.MkdirTemp("", "e2e-test-assets")
	Expect(err).NotTo(HaveOccurred())

	sharedData := sharedSetupData{
		CommonOrgName: commonTestOrgName,
		CommonOrgGUID: commonTestOrgGUID,
		// Some environments where Korifi does not manage the ClusterBuilder lack a standalone Procfile buildpack
		// The DEFAULT_APP_BITS_PATH and DEFAULT_APP_RESPONSE environment variables are a workaround to allow e2e tests to run
		// with a different app in these environments.
		// See https://github.com/cloudfoundry/korifi/issues/2355 for refactoring ideas
		DefaultAppBitsFile:       zipAsset(helpers.GetDefaultedEnvVar("DEFAULT_APP_BITS_PATH", "../assets/dorifi")),
		MultiProcessAppBitsFile:  zipAsset("../assets/multi-process"),
		AdminServiceAccount:      adminServiceAccount,
		AdminServiceAccountToken: adminServiceAccountToken,
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
	multiProcessAppBitsFile = sharedSetup.MultiProcessAppBitsFile
	adminServiceAccount = sharedSetup.AdminServiceAccount
	adminClient = makeTokenClient(sharedSetup.AdminServiceAccountToken)

	SetDefaultEventuallyTimeout(helpers.EventuallyTimeout())
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	os.RemoveAll(assetsTmpDir)
	deleteOrg(commonTestOrgGUID)
	serviceAccountFactory.DeleteServiceAccount(adminServiceAccount)
})

var _ = BeforeEach(func() {
	correlationId = uuid.NewString()
})

func makeCertClientForUserName(userName string, validFor time.Duration) *helpers.CorrelatedRestyClient {
	GinkgoHelper()

	if _, ok := os.LookupEnv("CSR_SIGNING_DISALLOWED"); ok {
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

func deleteOrg(guid string) {
	GinkgoHelper()

	if guid == "" {
		return
	}

	resp, err := adminClient.R().
		Delete("/v3/organizations/" + guid)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusAccepted))
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

func createApp(spaceGUID, name string) string {
	GinkgoHelper()

	var app resource

	resp, err := adminClient.R().
		SetBody(appResource{
			resource: resource{
				Name:          name,
				Relationships: relationships{"space": {Data: resource{GUID: spaceGUID}}},
			},
		}).
		SetResult(&app).
		Post("/v3/apps")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
	Expect(app.GUID).NotTo(BeEmpty())
	Expect(app.Name).To(Equal(name))
	Expect(app.CreatedAt).NotTo(BeEmpty())
	Expect(app.Relationships).NotTo(BeNil())
	Expect(app.Relationships).To(HaveKey("space"))
	Expect(app.Relationships["space"].Data.GUID).To(Equal(spaceGUID))

	return app.GUID
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

func createServiceBinding(appGUID, instanceGUID, bindingName string) string {
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

func createPackage(appGUID string) string {
	GinkgoHelper()

	var pkg resource
	resp, err := adminClient.R().
		SetBody(typedResource{
			Type: "bits",
			resource: resource{
				Relationships: relationships{
					"app": relationship{Data: resource{GUID: appGUID}},
				},
			},
		}).
		SetResult(&pkg).
		Post("/v3/packages")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))

	return pkg.GUID
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

func startApp(appGUID string) {
	GinkgoHelper()

	resp, err := adminClient.R().
		Post("/v3/apps/" + appGUID + "/actions/start")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
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
	pkgGUID := createPackage(appGUID)
	uploadTestApp(pkgGUID, appBitsFile)
	buildGUID := createBuild(pkgGUID)
	waitForDroplet(buildGUID)
	setCurrentDroplet(appGUID, buildGUID)
	startApp(appGUID)

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

func curlApp(appGUID, path string) []byte {
	GinkgoHelper()

	url := getAppRoute(appGUID)
	var body []byte
	Eventually(func(g Gomega) {
		r, err := skipSSLClient.Get("https://" + url + path)
		g.Expect(err).NotTo(HaveOccurred())
		defer r.Body.Close()
		g.Expect(r).To(HaveHTTPStatus(http.StatusOK))
		body, err = io.ReadAll(r.Body)
		g.Expect(err).NotTo(HaveOccurred())
	}).Should(Succeed())

	return body
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
	apiServerRoot = helpers.GetRequiredEnvVar("API_SERVER_ROOT")
	rootNamespace = helpers.GetRequiredEnvVar("ROOT_NAMESPACE")

	appFQDN = helpers.GetRequiredEnvVar("APP_FQDN")

	ensureServerIsUp()

	serviceAccountFactory = helpers.NewServiceAccountFactory(rootNamespace)
}

func zipAsset(src string) string {
	GinkgoHelper()

	file, err := os.CreateTemp("", "*.zip")
	if err != nil {
		Expect(err).NotTo(HaveOccurred())
	}
	defer file.Close()

	w := zip.NewWriter(file)
	defer w.Close()

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		fp, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fp.Close()

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		fh := &zip.FileHeader{
			Name: rel,
		}
		fh.SetMode(info.Mode())

		f, err := w.CreateHeader(fh)
		if err != nil {
			return err
		}

		_, err = io.Copy(f, fp)
		if err != nil {
			return err
		}

		return nil
	}
	err = filepath.Walk(src, walker)
	Expect(err).NotTo(HaveOccurred())

	return file.Name()
}

func systemNamespace() string {
	systemNS, found := os.LookupEnv("SYSTEM_NAMESPACE")
	if found {
		return systemNS
	}

	return "korifi"
}

func getCorrelationId() string {
	return correlationId
}

func printDropletNotFoundDebugInfo(config *rest.Config, message string) {
	fmt.Fprint(GinkgoWriter, "\n\n========== Droplet not found debug log (start) ==========\n")

	fmt.Fprint(GinkgoWriter, "\n========== Kpack logs ==========\n")
	fail_handler.PrintPodsLogs(config, []fail_handler.PodContainerDescriptor{
		{
			Namespace:  "kpack",
			LabelKey:   "app",
			LabelValue: "kpack-controller",
		},
		{
			Namespace:  "kpack",
			LabelKey:   "app",
			LabelValue: "kpack-webhook",
		},
	})

	dropletGUID, err := getDropletGUID(message)
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "Failed to get droplet GUID from message %v\n", err)
		return
	}

	fmt.Fprint(GinkgoWriter, "\n\n========== Droplet build logs ==========\n")
	fmt.Fprintf(GinkgoWriter, "DropletGUID: %q\n", dropletGUID)
	fail_handler.PrintPodsLogs(config, []fail_handler.PodContainerDescriptor{
		{
			LabelKey:   "korifi.cloudfoundry.org/build-workload-name",
			LabelValue: dropletGUID,
		},
	})
	fail_handler.PrintPodEvents(config, []fail_handler.PodContainerDescriptor{
		{
			LabelKey:   "korifi.cloudfoundry.org/build-workload-name",
			LabelValue: dropletGUID,
		},
	})

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
