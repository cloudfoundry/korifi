package e2e_test

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/tests/e2e/helpers"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var (
	adminClient         *resty.Client
	certClient          *resty.Client
	tokenClient         *resty.Client
	apiServerRoot       string
	serviceAccountName  string
	serviceAccountToken string
	tokenAuthHeader     string
	certUserName        string
	certAuthHeader      string
	adminAuthHeader     string
	certPEM             string

	rootNamespace     string
	appFQDN           string
	commonTestOrgGUID string
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

type resourceList struct {
	Resources []resource `json:"resources"`
}

type responseResource struct {
	Name      string `json:"name,omitempty"`
	GUID      string `json:"guid,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type responseResourceList struct {
	Resources []responseResource `json:"resources"`
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

type bareResourceList struct {
	Resources []bareResource `json:""`
}

type appResource struct {
	resource `json:",inline"`
	State    string `json:"state,omitempty"`
}

type typedResource struct {
	resource `json:",inline"`
	Type     string `json:"type,omitempty"`
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

type statsResourceList struct {
	Resources []presenter.ProcessStatsResource `json:"resources"`
}

type manifestResource struct {
	Version      int                   `yaml:"version"`
	Applications []applicationResource `yaml:"applications"`
}

type applicationResource struct {
	Name         string                               `yaml:"name"`
	DefaultRoute bool                                 `yaml:"default-route"`
	Processes    []manifestApplicationProcessResource `yaml:"processes"`
	Routes       []manifestRouteResource              `yaml:"routes"`
}

type manifestApplicationProcessResource struct {
	Type    string  `yaml:"type"`
	Command *string `yaml:"command"`
}

type manifestRouteResource struct {
	Route *string `yaml:"route"`
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

type cfErrs struct {
	Errors []cfErr
}

type cfErr struct {
	Detail string `json:"detail"`
	Title  string `json:"title"`
	Code   int    `json:"code"`
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(helpers.E2EFailHandler)
	RunSpecs(t, "E2E Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	commonTestSetup()

	commonTestOrgGUID = createOrg(generateGUID("common-test-org"))
	return []byte(commonTestOrgGUID)
}, func(commonOrgGUIDBytes []byte) {
	commonTestOrgGUID = string(commonOrgGUIDBytes)

	SetDefaultEventuallyTimeout(240 * time.Second)
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	commonTestSetup()

	createOrgRole("organization_user", rbacv1.UserKind, certUserName, commonTestOrgGUID)
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	deleteOrg(commonTestOrgGUID)
})

func mustHaveEnv(key string) string {
	val, ok := os.LookupEnv(key)
	ExpectWithOffset(1, ok).To(BeTrue(), "must set env var %q", key)

	return val
}

func mustHaveEnvIdx(key string, idx int) string {
	val := mustHaveEnv(key)
	vals := strings.Fields(val)
	Expect(len(vals)).To(BeNumerically(">=", idx), val)

	return vals[idx-1]
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
		return "", fmt.Errorf("expected status code %d, got %d", http.StatusCreated, resp.StatusCode())
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
		return "", fmt.Errorf("expected status code %d, got %d", http.StatusCreated, resp.StatusCode())
	}

	return space.GUID, nil
}

func deleteSpace(guid string) {
	if guid == "" {
		return
	}

	resp, err := adminClient.R().
		Delete("/v3/spaces/" + guid)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusAccepted))
}

func createSpace(spaceName, orgGUID string) string {
	spaceGUID, err := createSpaceRaw(spaceName, orgGUID)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), `create space "`+spaceName+`" in orgGUID "`+orgGUID+`" should have succeeded`)

	return spaceGUID
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
func createRole(roleName, kind, orgSpaceType, userName, orgSpaceGUID string) {
	rolesURL := apiServerRoot + apis.RolesPath

	userOrServiceAccount := "user"
	if kind == rbacv1.ServiceAccountKind {
		userOrServiceAccount = "kubernetesServiceAccount"
	}

	payload := typedResource{
		Type: roleName,
		resource: resource{
			Relationships: relationships{
				userOrServiceAccount: relationship{Data: resource{GUID: userName}},
				orgSpaceType:         relationship{Data: resource{GUID: orgSpaceGUID}},
			},
		},
	}

	var resultErr cfErrs
	resp, err := adminClient.R().
		SetBody(payload).
		SetError(&resultErr).
		Post(rolesURL)

	ExpectWithOffset(2, err).NotTo(HaveOccurred())
	ExpectWithOffset(2, resp).To(HaveRestyStatusCode(http.StatusCreated))
}

func createOrgRole(roleName, kind, userName, orgGUID string) {
	createRole(roleName, kind, "organization", userName, orgGUID)
}

func createSpaceRole(roleName, kind, userName, spaceGUID string) {
	createRole(roleName, kind, "space", userName, spaceGUID)
}

func obtainAdminUserCert() string {
	crtBytes, err := base64.StdEncoding.DecodeString(mustHaveEnv("CF_ADMIN_CERT"))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	keyBytes, err := base64.StdEncoding.DecodeString(mustHaveEnv("CF_ADMIN_KEY"))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	return base64.StdEncoding.EncodeToString(append(crtBytes, keyBytes...))
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
		Patch(fmt.Sprintf("/v3/apps/%s/environment_variables", appName))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusOK))
}

func getEnv(appName string) map[string]interface{} {
	var env map[string]interface{}

	resp, err := adminClient.R().
		SetResult(&env).
		Get(fmt.Sprintf("/v3/apps/%s/env", appName))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusOK))

	return env
}

func getProcess(appGUID, processType string) string {
	var processList resourceList
	EventuallyWithOffset(1, func(g Gomega) {
		resp, err := adminClient.R().
			SetResult(&processList).
			Get("/v3/processes?app_guids=" + appGUID)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
		g.Expect(processList.Resources).NotTo(BeEmpty())
	}).Should(Succeed())

	ExpectWithOffset(1, processList.Resources).To(HaveLen(1))
	return processList.Resources[0].GUID
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

func listServiceInstances() resourceList {
	var serviceInstances resourceList

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

func uploadTestApp(pkgGUID string) {
	resp, err := adminClient.R().
		SetFiles(map[string]string{
			"bits": "assets/procfile.zip",
		}).Post("/v3/packages/" + pkgGUID + "/upload")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp).To(HaveRestyStatusCode(http.StatusOK))
}

func pushTestApp(spaceGUID string) string {
	appGUID := createApp(spaceGUID, generateGUID("app"))
	pkgGUID := createPackage(appGUID)
	uploadTestApp(pkgGUID)
	buildGUID := createBuild(pkgGUID)
	waitForDroplet(buildGUID)
	setCurrentDroplet(appGUID, buildGUID)
	startApp(appGUID)

	return appGUID
}

func getDomainGUID(domainName string) string {
	res := bareResourceList{}
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
	Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
	Expect(errResp.Errors).To(ConsistOf(
		cfErr{
			Detail: detail,
			Title:  "CF-UnprocessableEntity",
			Code:   10008,
		},
	))
}

func commonTestSetup() {
	apiServerRoot = mustHaveEnv("API_SERVER_ROOT")
	rootNamespace = mustHaveEnv("ROOT_NAMESPACE")
	serviceAccountName = mustHaveEnvIdx("E2E_SERVICE_ACCOUNTS", GinkgoParallelProcess())
	serviceAccountToken = mustHaveEnvIdx("E2E_SERVICE_ACCOUNT_TOKENS", GinkgoParallelProcess())
	certUserName = mustHaveEnvIdx("E2E_USER_NAMES", GinkgoParallelProcess())
	certPEM = mustHaveEnvIdx("E2E_USER_PEMS", GinkgoParallelProcess())
	appFQDN = mustHaveEnv("APP_FQDN")

	ensureServerIsUp()

	adminClient = resty.New().SetBaseURL(apiServerRoot).SetAuthScheme("ClientCert").SetAuthToken(obtainAdminUserCert()).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	adminAuthHeader = "ClientCert " + obtainAdminUserCert()
	certAuthHeader = "ClientCert " + certPEM
	tokenAuthHeader = fmt.Sprintf("Bearer %s", serviceAccountToken)
	certClient = resty.New().SetBaseURL(apiServerRoot).SetAuthScheme("ClientCert").SetAuthToken(certPEM).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	tokenClient = resty.New().SetBaseURL(apiServerRoot).SetAuthToken(serviceAccountToken).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
}
