package e2e_test

import (
	"archive/zip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"
	"code.cloudfoundry.org/korifi/tests/e2e/helpers"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"gopkg.in/yaml.v3"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	correlationId string

	adminClient             *helpers.CorrelatedRestyClient
	certClient              *helpers.CorrelatedRestyClient
	tokenClient             *helpers.CorrelatedRestyClient
	longCertClient          *helpers.CorrelatedRestyClient
	apiServerRoot           string
	serviceAccountName      string
	serviceAccountToken     string
	certUserName            string
	certPEM                 string
	longCertUserName        string
	longCertPEM             string
	rootNamespace           string
	appFQDN                 string
	commonTestOrgGUID       string
	commonTestOrgName       string
	procfileAppBitsFile     string
	nodeAppBitsFile         string
	doraAppBitsFile         string
	multiProcessAppBitsFile string
	assetsTmpDir            string
	clusterVersionMinor     int
	clusterVersionMajor     int
)

type resource struct {
	Name          string        `json:"name,omitempty"`
	GUID          string        `json:"guid,omitempty"`
	Relationships relationships `json:"relationships,omitempty"`
	CreatedAt     string        `json:"created_at,omitempty"`
	UpdatedAt     string        `json:"updated_at,omitempty"`
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
	resource `json:",inline"`
	State    string `json:"state,omitempty"`
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
	Credentials  map[string]string `json:"credentials"`
	InstanceType string            `json:"type"`
}

type appLogResource struct {
	Envelopes appLogResourceEnvelopes `json:"envelopes"`
}

type appLogResourceEnvelopes struct {
	Batch []loggregator_v2.Envelope `json:"batch"`
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

func getCorrelationId() string {
	return correlationId
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(helpers.E2EFailHandler(getCorrelationId))
	RunSpecs(t, "E2E Suite")
}

type sharedSetupData struct {
	CommonOrgName string `json:"commonOrgName"`
	CommonOrgGUID string `json:"commonOrgGuid"`
	NodeAppBitsFile string `json:"nodeAppBitsFile"`
	DoraAppBitsFile string `json:"doraAppBitsFile"`
	MultiProcessAppBitsFile string `json:"multiProcessAppBitsFile"`
	ProcfileAppBitsFile string `json:"procfileAppBitsFile"`
}

var _ = SynchronizedBeforeSuite(func() []byte {
	commonTestSetup()
	commonTestOrgName = generateGUID("common-test-org")
	commonTestOrgGUID = createOrg(commonTestOrgName)
	createOrgRole("organization_user", certUserName, commonTestOrgGUID)

	prepareAssets()

	bs, err := json.Marshal(sharedSetupData{
		CommonOrgName: commonTestOrgName,
		CommonOrgGUID: commonTestOrgGUID,
		NodeAppBitsFile: nodeAppBitsFile,
		DoraAppBitsFile: doraAppBitsFile,
		MultiProcessAppBitsFile: multiProcessAppBitsFile,
		ProcfileAppBitsFile: procfileAppBitsFile,
	})
	Expect(err).NotTo(HaveOccurred())
	return bs
}, func(bs []byte) {
	var sharedSetup sharedSetupData
	err := json.Unmarshal(bs, &sharedSetup)
	Expect(err).NotTo(HaveOccurred())

	commonTestOrgGUID = sharedSetup.CommonOrgGUID
	commonTestOrgName = sharedSetup.CommonOrgName

	nodeAppBitsFile = sharedSetup.NodeAppBitsFile
	doraAppBitsFile = sharedSetup.DoraAppBitsFile
	multiProcessAppBitsFile = sharedSetup.MultiProcessAppBitsFile
	procfileAppBitsFile = getAppBitsFileFromPath(sharedSetup)

	SetDefaultEventuallyTimeout(240 * time.Second)
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	commonTestSetup()
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	os.RemoveAll(assetsTmpDir)
	deleteOrg(commonTestOrgGUID)
})

var _ = BeforeEach(func() {
	correlationId = uuid.NewString()
})

func mustHaveEnv(key string) string {
	val, ok := os.LookupEnv(key)
	ExpectWithOffset(1, ok).To(BeTrue(), "must set env var %q", key)

	return val
}

func makeClient(certEnvVar, tokenEnvVar string) *helpers.CorrelatedRestyClient {
	cert := os.Getenv(certEnvVar)
	if cert != "" {
		return helpers.NewCorrelatedRestyClient(apiServerRoot, getCorrelationId).SetAuthScheme("ClientCert").SetAuthToken(cert).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	}

	token := os.Getenv(tokenEnvVar)
	if token != "" {
		return helpers.NewCorrelatedRestyClient(apiServerRoot, getCorrelationId).SetAuthToken(token).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	}

	Fail(fmt.Sprintf("One of %q or %q should have a value, but they are both empty", certEnvVar, tokenEnvVar))
	return nil
}

func ensureServerIsUp() {
	EventuallyWithOffset(1, func() (int, error) {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		resp, err := http.Get(apiServerRoot)
		if err != nil {
			return 0, err
		}

		resp.Body.Close()

		return resp.StatusCode, nil
	}, "5m").Should(Equal(http.StatusOK), "API Server at %s was not running after 5 minutes", apiServerRoot)
}

func prepareAssets() {
	var err error
	assetsTmpDir, err = os.MkdirTemp("", "e2e-test-assets")
	Expect(err).NotTo(HaveOccurred())

	procfileAppBitsFile = filepath.Join(assetsTmpDir, "procfile.zip")
	Expect(zipAsset("assets/procfile", procfileAppBitsFile)).To(Succeed())

	nodeAppBitsFile = filepath.Join(assetsTmpDir, "node.zip")
	Expect(zipAsset("assets/vendored/node", nodeAppBitsFile)).To(Succeed())

	doraAppBitsFile = filepath.Join(assetsTmpDir, "dora.zip")
	Expect(zipAsset("assets/vendored/dora", doraAppBitsFile)).To(Succeed())

	multiProcessAppBitsFile = filepath.Join(assetsTmpDir, "multi-process.zip")
	Expect(zipAsset("assets/multi-process", multiProcessAppBitsFile)).To(Succeed())
}

func generateGUID(prefix string) string {
	guid := uuid.NewString()

	return fmt.Sprintf("%s-%s", prefix, guid[:13])
}

func deleteOrg(guid string) {
	if guid == "" {
		return
	}

	resp, err := adminClient.R().
		Delete("/v3/organizations/" + guid)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusAccepted))
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
	orgGUID, err := createOrgRaw(orgName)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

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
	if guid == "" {
		return
	}

	resp, err := adminClient.R().
		Delete("/v3/spaces/" + guid)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusAccepted))
}

func createSpace(spaceName, orgGUID string) string {
	spaceGUID, err := createSpaceRaw(spaceName, orgGUID)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), `create space "`+spaceName+`" in orgGUID "`+orgGUID+`" should have succeeded`)

	return spaceGUID
}

func applySpaceManifest(manifest manifestResource, spaceGUID string) {
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

// createRole creates an org or space role
// You should probably invoke this via createOrgRole or createSpaceRole
func createRole(roleName, orgSpaceType, userName, orgSpaceGUID string) string {
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

	ExpectWithOffset(2, err).NotTo(HaveOccurred())
	ExpectWithOffset(2, resp).To(HaveRestyStatusCode(http.StatusCreated))

	return createdRole.GUID
}

func createOrgRole(roleName, userName, orgGUID string) string {
	return createRole(roleName, "organization", userName, orgGUID)
}

func createSpaceRole(roleName, userName, spaceGUID string) string {
	return createRole(roleName, "space", userName, spaceGUID)
}

func createApp(spaceGUID, name string) string {
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

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusCreated))
	Expect(app.GUID).NotTo(BeEmpty())
	Expect(app.Name).To(Equal(name))
	Expect(app.CreatedAt).NotTo(BeEmpty())
	Expect(app.Relationships).NotTo(BeNil())
	Expect(app.Relationships).To(HaveKey("space"))
	Expect(app.Relationships["space"].Data.GUID).To(Equal(spaceGUID))

	return app.GUID
}

func setEnv(appName string, envVars map[string]interface{}) {
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
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusOK))
}

func getEnv(appName string) map[string]interface{} {
	var env map[string]interface{}

	resp, err := adminClient.R().
		SetResult(&env).
		SetPathParam("appName", appName).
		Get("/v3/apps/{appName}/env")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusOK))

	return env
}

func getProcess(appGUID, processType string) processResource {
	var process processResource
	EventuallyWithOffset(1, func(g Gomega) {
		resp, err := adminClient.R().
			SetResult(&process).
			Get("/v3/apps/" + appGUID + "/processes/" + processType)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
	}).Should(Succeed())

	return process
}

func createServiceInstance(spaceGUID, name string) string {
	var serviceInstance typedResource

	resp, err := adminClient.R().
		SetBody(typedResource{
			Type: "user-provided",
			resource: resource{
				Name:          name,
				Relationships: relationships{"space": {Data: resource{GUID: spaceGUID}}},
			},
		}).
		SetResult(&serviceInstance).
		Post("/v3/service_instances")

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp.StatusCode()).To(Equal(http.StatusCreated))

	return serviceInstance.GUID
}

func listServiceInstances() resourceList[resource] {
	var serviceInstances resourceList[resource]

	resp, err := adminClient.R().
		SetResult(&serviceInstances).
		Get("/v3/service_instances")

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp.StatusCode()).To(Equal(http.StatusOK))

	return serviceInstances
}

func createServiceBinding(appGUID, instanceGUID string) string {
	var pkg resource
	resp, err := adminClient.R().
		SetBody(typedResource{
			Type: "app",
			resource: resource{
				Relationships: relationships{"app": {Data: resource{GUID: appGUID}}, "service_instance": {Data: resource{GUID: instanceGUID}}},
			},
		}).
		SetResult(&pkg).
		Post("/v3/service_credential_bindings")

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp.StatusCode()).To(Equal(http.StatusCreated))

	return pkg.GUID
}

func createPackage(appGUID string) string {
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

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusCreated))

	return pkg.GUID
}

func createBuild(packageGUID string) string {
	var build resource

	resp, err := adminClient.R().
		SetBody(buildResource{Package: resource{GUID: packageGUID}}).
		SetResult(&build).
		Post("/v3/builds")

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusCreated))

	return build.GUID
}

func waitForDroplet(buildGUID string) {
	EventuallyWithOffset(1, func() (*resty.Response, error) {
		resp, err := adminClient.R().
			Get("/v3/droplets/" + buildGUID)
		return resp, err
	}).Should(HaveRestyStatusCode(http.StatusOK))
}

func setCurrentDroplet(appGUID, dropletGUID string) {
	resp, err := adminClient.R().
		SetBody(dropletResource{Data: resource{GUID: dropletGUID}}).
		Patch("/v3/apps/" + appGUID + "/relationships/current_droplet")

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusOK))
}

func startApp(appGUID string) {
	resp, err := adminClient.R().
		Post("/v3/apps/" + appGUID + "/actions/start")

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusOK))
}

func uploadTestApp(pkgGUID, appBitsFile string) {
	resp, err := adminClient.R().
		SetFiles(map[string]string{
			"bits": appBitsFile,
		}).Post("/v3/packages/" + pkgGUID + "/upload")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusOK))
}

func getAppGUIDFromName(appName string) string {
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
	appName := generateGUID("app")
	return pushTestAppWithName(spaceGUID, appBitsFile, appName), appName
}

func pushTestAppWithName(spaceGUID, appBitsFile string, appName string) string {
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
	res := resourceList[bareResource]{}
	resp, err := adminClient.R().
		SetResult(&res).
		Get("/v3/domains")

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusOK))

	for _, d := range res.Resources {
		if d.Name == domainName {
			return d.GUID
		}
	}

	Fail(fmt.Sprintf("no domain found for domainName: %q", domainName))

	return ""
}

func createRoute(host, path string, spaceGUID, domainGUID string) string {
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

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusCreated))

	return route.GUID
}

func addDestinationForRoute(appGUID, routeGUID string) []string {
	var destinations destinationsResource

	resp, err := adminClient.R().
		SetBody(mapRouteResource{
			Destinations: []destinationRef{
				{App: resource{GUID: appGUID}},
			},
		}).
		SetResult(&destinations).
		Post("/v3/routes/" + routeGUID + "/destinations")

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusOK))

	var destinationGUIDs []string
	for _, destination := range destinations.Destinations {
		destinationGUIDs = append(destinationGUIDs, destination.GUID)
	}

	return destinationGUIDs
}

func expectNotFoundError(resp *resty.Response, errResp cfErrs, resource string) {
	ExpectWithOffset(1, resp.StatusCode()).To(Equal(http.StatusNotFound))
	ExpectWithOffset(1, errResp.Errors).To(ConsistOf(
		cfErr{
			Detail: resource + " not found. Ensure it exists and you have access to it.",
			Title:  "CF-ResourceNotFound",
			Code:   10010,
		},
	))
}

func expectForbiddenError(resp *resty.Response, errResp cfErrs) {
	ExpectWithOffset(1, resp.StatusCode()).To(Equal(http.StatusForbidden))
	ExpectWithOffset(1, errResp.Errors).To(ConsistOf(
		cfErr{
			Detail: "You are not authorized to perform the requested action",
			Title:  "CF-NotAuthorized",
			Code:   10003,
		},
	))
}

func expectUnprocessableEntityError(resp *resty.Response, errResp cfErrs, detail string) {
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
	ExpectWithOffset(1, errResp.Errors).To(ConsistOf(
		cfErr{
			Detail: detail,
			Title:  "CF-UnprocessableEntity",
			Code:   10008,
		},
	))
}

func getAppBitsFileFromPath(sharedSetup sharedSetupData) string {
   val, ok := os.LookupEnv("APP_BITS_PATH")
   if !ok {
       return sharedSetup.ProcfileAppBitsFile
   }

	 switch val {
	 case "assets/vendored/node":
		 return sharedSetup.NodeAppBitsFile
	 case "assets/vendored/dora":
		 return sharedSetup.DoraAppBitsFile
	 case "assets/multi-process":
		 return sharedSetup.MultiProcessAppBitsFile
	 default:
		 return sharedSetup.ProcfileAppBitsFile
	 }
}

func commonTestSetup() {
	apiServerRoot = mustHaveEnv("API_SERVER_ROOT")
	rootNamespace = mustHaveEnv("ROOT_NAMESPACE")
	serviceAccountName = mustHaveEnv("E2E_SERVICE_ACCOUNT")
	serviceAccountToken = mustHaveEnv("E2E_SERVICE_ACCOUNT_TOKEN")

	certUserName = mustHaveEnv("E2E_USER_NAME")
	certPEM = os.Getenv("E2E_USER_PEM")

	longCertUserName = mustHaveEnv("E2E_LONGCERT_USER_NAME")
	longCertPEM = os.Getenv("E2E_LONGCERT_USER_PEM")

	appFQDN = mustHaveEnv("APP_FQDN")

	clusterVersionMinor, _ = strconv.Atoi(mustHaveEnv("CLUSTER_VERSION_MINOR"))
	clusterVersionMajor, _ = strconv.Atoi(mustHaveEnv("CLUSTER_VERSION_MAJOR"))

	ensureServerIsUp()

	adminClient = makeClient("CF_ADMIN_PEM", "CF_ADMIN_TOKEN")
	certClient = makeClient("E2E_USER_PEM", "E2E_USER_TOKEN")
	tokenClient = helpers.NewCorrelatedRestyClient(apiServerRoot, getCorrelationId).SetAuthToken(serviceAccountToken).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	longCertClient = helpers.NewCorrelatedRestyClient(apiServerRoot, getCorrelationId).SetAuthScheme("ClientCert").SetAuthToken(longCertPEM).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
}

func zipAsset(src, output string) error {
	file, err := os.Create(output)
	if err != nil {
		panic(err)
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
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		f, err := w.Create(rel)
		if err != nil {
			return err
		}

		_, err = io.Copy(f, file)
		if err != nil {
			return err
		}

		return nil
	}
	err = filepath.Walk(src, walker)
	if err != nil {
		panic(err)
	}

	return err
}
