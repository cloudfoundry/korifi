package e2e_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/tests/e2e/helpers"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	certsv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var (
	k8sClient           client.WithWatch
	adminClient         *resty.Client
	certClient          *resty.Client
	tokenClient         *resty.Client
	clientset           *kubernetes.Clientset
	rootNamespace       string
	apiServerRoot       string
	appFQDN             string
	appDomainGUID       string
	serviceAccountName  string
	serviceAccountToken string
	tokenAuthHeader     string
	certUserName        string
	certSigningReq      *certsv1.CertificateSigningRequest
	certAuthHeader      string
	adminAuthHeader     string
	certPEM             string
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
	Name      string                               `yaml:"name"`
	Processes []manifestApplicationProcessResource `yaml:"processes"`
	Routes    []manifestRouteResource              `yaml:"routes"`
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
	Expect(networkingv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.NewWithWatch(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	rootNamespace = mustHaveEnv("ROOT_NAMESPACE")
	appFQDN = mustHaveEnv("APP_FQDN")

	appDomainGUID = createDomain(appFQDN)

	return []byte(appDomainGUID)
}, func(appDomainGUIDBytes []byte) {
	SetDefaultEventuallyTimeout(240 * time.Second)
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	apiServerRoot = mustHaveEnv("API_SERVER_ROOT")

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	Expect(hnsv1alpha2.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(networkingv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	adminAuthHeader = "ClientCert " + obtainAdminUserCert()

	k8sClient, err = client.NewWithWatch(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	clientset, err = kubernetes.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred())

	rootNamespace = mustHaveEnv("ROOT_NAMESPACE")
	appFQDN = mustHaveEnv("APP_FQDN")

	ensureServerIsUp()

	serviceAccountName = generateGUID("token-user")
	serviceAccountToken = obtainServiceAccountToken(serviceAccountName)

	certUserName = generateGUID("cert-user")
	certSigningReq, certPEM = obtainClientCert(certUserName)
	certAuthHeader = "ClientCert " + certPEM

	appDomainGUID = string(appDomainGUIDBytes)
})

var _ = BeforeEach(func() {
	tokenAuthHeader = fmt.Sprintf("Bearer %s", serviceAccountToken)
	adminClient = resty.New().SetBaseURL(apiServerRoot).SetAuthScheme("ClientCert").SetAuthToken(obtainAdminUserCert()).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	certClient = resty.New().SetBaseURL(apiServerRoot).SetAuthScheme("ClientCert").SetAuthToken(certPEM).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	tokenClient = resty.New().SetBaseURL(apiServerRoot).SetAuthToken(serviceAccountToken).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
})

var _ = SynchronizedAfterSuite(func() {
	deleteServiceAccount(serviceAccountName)
	deleteCSR(certSigningReq)
}, func() {
	deleteDomain(appDomainGUID)
})

func mustHaveEnv(key string) string {
	val, ok := os.LookupEnv(key)
	Expect(ok).To(BeTrue(), "must set env var %q", key)

	return val
}

func ensureServerIsUp() {
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

func deleteOrg(name string) {
	if name == "" {
		return
	}

	deleteSubnamespace(rootNamespace, name)
}

func asyncDeleteOrg(orgID string, wg *sync.WaitGroup) {
	go func() {
		defer wg.Done()
		defer GinkgoRecover()

		deleteOrg(orgID)
	}()
}

func deleteSubnamespace(parent, name string) {
	if parent == "" || name == "" {
		return
	}

	ctx := context.Background()

	subnsList := &hnsv1alpha2.SubnamespaceAnchorList{}
	Expect(k8sClient.List(ctx, subnsList, client.InNamespace(name))).To(Succeed())

	var wg sync.WaitGroup
	wg.Add(len(subnsList.Items))
	for _, subns := range subnsList.Items {
		go func(subns string) {
			defer wg.Done()
			defer GinkgoRecover()

			deleteSubnamespace(name, subns)
		}(subns.Name)
	}
	wg.Wait()

	anchor := hnsv1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: parent,
			Name:      name,
		},
	}
	err := k8sClient.Delete(ctx, &anchor)
	if errors.IsNotFound(err) {
		return
	}
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&anchor), &anchor)

		return errors.IsNotFound(err)
	}).Should(BeTrue())
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
		return "", fmt.Errorf("expected status code %d, got %d", http.StatusCreated, resp.StatusCode())
	}

	return space.GUID, nil
}

func createSpace(spaceName, orgGUID string) string {
	spaceGUID, err := createSpaceRaw(spaceName, orgGUID)
	Expect(err).NotTo(HaveOccurred(), `create space "`+spaceName+`" in orgGUID "`+orgGUID+`" should have succeeded`)

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

func obtainServiceAccountToken(name string) string {
	var err error

	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rootNamespace,
		},
	}
	err = k8sClient.Create(context.Background(), &serviceAccount)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&serviceAccount), &serviceAccount); err != nil {
			return err
		}

		if len(serviceAccount.Secrets) != 1 {
			return fmt.Errorf("expected exactly 1 secret, got %d", len(serviceAccount.Secrets))
		}

		return nil
	}, "120s").Should(Succeed())

	tokenSecret := corev1.Secret{}
	Eventually(func() error {
		return k8sClient.Get(context.Background(), client.ObjectKey{Name: serviceAccount.Secrets[0].Name, Namespace: rootNamespace}, &tokenSecret)
	}).Should(Succeed())

	return string(tokenSecret.Data["token"])
}

func deleteServiceAccount(name string) {
	if name == "" {
		return
	}

	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rootNamespace,
		},
	}

	Expect(k8sClient.Delete(context.Background(), &serviceAccount)).To(Succeed())
}

func obtainClientCert(name string) (*certsv1.CertificateSigningRequest, string) {
	privKey, err := rsa.GenerateKey(rand.Reader, 1024)
	Expect(err).NotTo(HaveOccurred())

	template := x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: name},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	Expect(err).NotTo(HaveOccurred())

	k8sCSR := &certsv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: uuid.NewString(),
		},
		Spec: certsv1.CertificateSigningRequestSpec{
			Request:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes}),
			SignerName: "kubernetes.io/kube-apiserver-client",
			Usages:     []certsv1.KeyUsage{certsv1.UsageClientAuth},
		},
	}

	Expect(k8sClient.Create(context.Background(), k8sCSR)).To(Succeed())

	k8sCSR.Status.Conditions = append(k8sCSR.Status.Conditions, certsv1.CertificateSigningRequestCondition{
		Type:   certsv1.CertificateApproved,
		Status: "True",
	})

	k8sCSR, err = clientset.CertificatesV1().CertificateSigningRequests().UpdateApproval(context.Background(), k8sCSR.Name, k8sCSR, metav1.UpdateOptions{})
	Expect(err).NotTo(HaveOccurred())

	var certPEM []byte
	Eventually(func() ([]byte, error) {
		err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(k8sCSR), k8sCSR)
		if err != nil {
			return nil, err
		}

		if len(k8sCSR.Status.Certificate) == 0 {
			return nil, nil
		}

		certPEM = k8sCSR.Status.Certificate

		return certPEM, nil
	}).ShouldNot(BeEmpty())

	buf := bytes.NewBuffer(certPEM)
	Expect(pem.Encode(buf, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})).To(Succeed())

	return k8sCSR, base64.StdEncoding.EncodeToString(buf.Bytes())
}

func obtainAdminUserCert() string {
	crtBytes, err := base64.StdEncoding.DecodeString(mustHaveEnv("CF_ADMIN_CERT"))
	Expect(err).NotTo(HaveOccurred())
	keyBytes, err := base64.StdEncoding.DecodeString(mustHaveEnv("CF_ADMIN_KEY"))
	Expect(err).NotTo(HaveOccurred())

	return base64.StdEncoding.EncodeToString(append(crtBytes, keyBytes...))
}

func deleteCSR(csr *certsv1.CertificateSigningRequest) {
	if csr == nil {
		return
	}

	Expect(k8sClient.Delete(context.Background(), csr)).To(Succeed())
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

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))

	return app.GUID
}

func getProcess(appGUID, processType string) string {
	var processList resourceList
	Eventually(func(g Gomega) {
		resp, err := adminClient.R().
			SetResult(&processList).
			Get("/v3/processes?app_guids=" + appGUID)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
		g.Expect(processList.Resources).NotTo(BeEmpty())
	}).Should(Succeed())

	Expect(processList.Resources).To(HaveLen(1))
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

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))

	return serviceInstance.GUID
}

func listServiceInstances() resourceList {
	var serviceInstances resourceList

	resp, err := adminClient.R().
		SetResult(&serviceInstances).
		Get("/v3/service_instances")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))

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

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))

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

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))

	return pkg.GUID
}

func createBuild(packageGUID string) string {
	var build resource

	resp, err := adminClient.R().
		SetBody(buildResource{Package: resource{GUID: packageGUID}}).
		SetResult(&build).
		Post("/v3/builds")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))

	return build.GUID
}

func waitForDroplet(buildGUID string) {
	Eventually(func() (*resty.Response, error) {
		resp, err := adminClient.R().
			Get("/v3/droplets/" + buildGUID)
		return resp, err
	}).Should(HaveRestyStatusCode(http.StatusOK))
}

func setCurrentDroplet(appGUID, dropletGUID string) {
	resp, err := adminClient.R().
		SetBody(dropletResource{Data: resource{GUID: dropletGUID}}).
		Patch("/v3/apps/" + appGUID + "/relationships/current_droplet")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
}

func startApp(appGUID string) {
	resp, err := adminClient.R().
		Post("/v3/apps/" + appGUID + "/actions/start")

	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
}

func uploadNodeApp(pkgGUID string) {
	resp, err := adminClient.R().
		SetFiles(map[string]string{
			"bits": "assets/node.zip",
		}).Post("/v3/packages/" + pkgGUID + "/upload")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
}

// pushNodeApp creates a running node app in the given space
func pushNodeApp(spaceGUID string) string {
	appGUID := createApp(spaceGUID, generateGUID("app"))
	pkgGUID := createPackage(appGUID)
	uploadNodeApp(pkgGUID)
	buildGUID := createBuild(pkgGUID)
	waitForDroplet(buildGUID)
	setCurrentDroplet(appGUID, buildGUID)
	startApp(appGUID)

	return appGUID
}

func createDomain(name string) string {
	domain := networkingv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: rootNamespace,
		},
		Spec: networkingv1alpha1.CFDomainSpec{
			Name: name,
		},
	}
	err := k8sClient.Create(context.Background(), &domain)
	Expect(err).NotTo(HaveOccurred())

	return domain.Name
}

func deleteDomain(guid string) {
	if guid == "" {
		return
	}

	Expect(k8sClient.Delete(context.Background(), &networkingv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{Namespace: rootNamespace, Name: guid},
	})).To(Succeed())
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
	Expect(resp.StatusCode()).To(Equal(http.StatusNotFound))
	Expect(errResp.Errors).To(ConsistOf(
		cfErr{
			Detail: resource + " not found. Ensure it exists and you have access to it.",
			Title:  "CF-ResourceNotFound",
			Code:   10010,
		},
	))
}

func expectForbiddenError(resp *resty.Response, errResp cfErrs) {
	Expect(resp.StatusCode()).To(Equal(http.StatusForbidden))
	Expect(errResp.Errors).To(ConsistOf(
		cfErr{
			Detail: "You are not authorized to perform the requested action",
			Title:  "CF-NotAuthorized",
			Code:   10003,
		},
	))
}
