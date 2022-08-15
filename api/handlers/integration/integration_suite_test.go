package integration_test

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/authorization/testhelpers"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	servicebindingv1beta1 "github.com/servicebinding/service-binding-controller/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var (
	testEnv               *envtest.Environment
	k8sClient             client.WithWatch
	k8sConfig             *rest.Config
	namespaceRetriever    repositories.NamespaceRetriever
	server                *http.Server
	port                  int
	rr                    *httptest.ResponseRecorder
	req                   *http.Request
	router                *mux.Router
	serverURL             *url.URL
	userName              string
	ctx                   context.Context
	adminRole             *rbacv1.ClusterRole
	spaceDeveloperRole    *rbacv1.ClusterRole
	spaceManagerRole      *rbacv1.ClusterRole
	orgUserRole           *rbacv1.ClusterRole
	orgManagerRole        *rbacv1.ClusterRole
	rootNamespaceUserRole *rbacv1.ClusterRole
	rootNamespace         string
	clientFactory         authorization.UserK8sClientFactory
	nsPermissions         *authorization.NamespacePermissions
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "controllers", "config", "controllers", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	k8sConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sConfig).NotTo(BeNil())

	err = korifiv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = servicebindingv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.NewWithWatch(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	Expect(err).NotTo(HaveOccurred())
	Expect(dynamicClient).NotTo(BeNil())
	namespaceRetriever = repositories.NewNamespaceRetriver(dynamicClient)
	Expect(namespaceRetriever).NotTo(BeNil())

	rand.Seed(time.Now().UnixNano())

	ctx = context.Background()
	adminRole = createClusterRole(ctx, "cf_admin")
	spaceDeveloperRole = createClusterRole(ctx, "cf_space_developer")
	spaceManagerRole = createClusterRole(ctx, "cf_space_manager")
	orgManagerRole = createClusterRole(ctx, "cf_org_manager")
	orgUserRole = createClusterRole(ctx, "cf_org_user")
	rootNamespaceUserRole = createClusterRole(ctx, "cf_root_namespace_user")
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
})

var _ = BeforeEach(func() {
	rootNamespace = generateGUIDWithPrefix("root")

	mapper, err := apiutil.NewDynamicRESTMapper(k8sConfig)
	Expect(err).NotTo(HaveOccurred())
	clientFactory = authorization.NewUnprivilegedClientFactory(k8sConfig, mapper, authorization.NewDefaultBackoff())
	tokenInspector := authorization.NewTokenReviewer(k8sClient)
	certInspector := authorization.NewCertInspector(k8sConfig)
	identityProvider := authorization.NewCertTokenIdentityProvider(tokenInspector, certInspector)
	nsPermissions = authorization.NewNamespacePermissions(k8sClient, identityProvider, rootNamespace)

	userName = generateGUID()

	cert, key := testhelpers.ObtainClientCert(testEnv, userName)
	authInfo := authorization.Info{CertData: testhelpers.JoinCertAndKey(cert, key)}
	ctx = authorization.NewContext(context.Background(), &authInfo)

	Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())

	createRoleBinding(ctx, userName, rootNamespaceUserRole.Name, rootNamespace)

	rr = httptest.NewRecorder()
	router = mux.NewRouter()
	router.Use(handlers.NewCorrelationIDMiddleware().Middleware)

	port = 1024 + rand.Intn(8975)

	serverAddr := fmt.Sprintf("localhost:%d", port)
	serverURL, err = url.Parse("http://" + serverAddr)
	Expect(err).NotTo(HaveOccurred())

	server = &http.Server{Addr: serverAddr, Handler: router}
	go func() {
		defer GinkgoRecover()
		Expect(
			server.ListenAndServe(),
		).To(MatchError("http: Server closed"))
	}()
})

var _ = AfterEach(func() {
	Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())

	Expect(server.Close()).To(Succeed())
})

func serverURI(paths ...string) string {
	return fmt.Sprintf("%s%s", serverURL, strings.Join(paths, ""))
}

func generateGUID() string {
	return uuid.NewString()
}

func generateGUIDWithPrefix(prefix string) string {
	return prefix + uuid.NewString()
}

func createOrgWithCleanup(ctx context.Context, name string) *korifiv1alpha1.CFOrg {
	guid := uuid.NewString()
	cfOrg := &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: rootNamespace,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: name,
		},
	}
	Expect(k8sClient.Create(ctx, cfOrg)).To(Succeed())

	meta.SetStatusCondition(&(cfOrg.Status.Conditions), metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "cus",
		Message: "cus",
	})
	Expect(k8sClient.Status().Update(ctx, cfOrg)).To(Succeed())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfOrg.Name,
			Labels: map[string]string{
				korifiv1alpha1.OrgNameLabel: cfOrg.Spec.DisplayName,
			},
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

	DeferCleanup(func() {
		_ = k8sClient.Delete(ctx, cfOrg)
		_ = k8sClient.Delete(ctx, namespace)
	})

	return cfOrg
}

func createSpaceWithCleanup(ctx context.Context, orgGUID, name string) *korifiv1alpha1.CFSpace {
	guid := uuid.NewString()
	cfSpace := &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: orgGUID,
		},
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: name,
		},
	}
	Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())

	cfSpace.Status.GUID = cfSpace.Name
	meta.SetStatusCondition(&(cfSpace.Status.Conditions), metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "cus",
		Message: "cus",
	})
	Expect(k8sClient.Status().Update(ctx, cfSpace)).To(Succeed())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfSpace.Name,
			Labels: map[string]string{
				korifiv1alpha1.SpaceNameLabel: cfSpace.Spec.DisplayName,
			},
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

	DeferCleanup(func() {
		_ = k8sClient.Delete(ctx, cfSpace)
		_ = k8sClient.Delete(ctx, namespace)
	})

	return cfSpace
}

func createClusterRole(ctx context.Context, filename string) *rbacv1.ClusterRole {
	filepath := filepath.Join("..", "..", "..", "controllers", "config", "controllers", "cf_roles", filename+".yaml")
	content, err := os.ReadFile(filepath)
	Expect(err).NotTo(HaveOccurred())

	decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDecoder()
	clusterRole := &rbacv1.ClusterRole{}
	err = runtime.DecodeInto(decoder, content, clusterRole)
	Expect(err).NotTo(HaveOccurred())

	clusterRole.Name = "cf-" + clusterRole.Name
	Expect(k8sClient.Create(ctx, clusterRole)).To(Succeed())

	return clusterRole
}

func createRoleBinding(ctx context.Context, userName, roleName, namespace string) {
	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateGUID(),
			Namespace: namespace,
		},
		Subjects: []rbacv1.Subject{{
			Kind: rbacv1.UserKind,
			Name: userName,
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: roleName,
		},
	}
	Expect(k8sClient.Create(ctx, &roleBinding)).To(Succeed())
}
