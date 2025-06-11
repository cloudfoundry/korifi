package descriptors_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/authorization/testhelpers"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/cache"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	ctx                   context.Context
	testEnv               *envtest.Environment
	k8sClient             client.WithWatch
	restClient            restclient.Interface
	rootNamespace         string
	authInfo              authorization.Info
	userClientFactory     authorization.UserClientFactory
	spaceDeveloperRole    *rbacv1.ClusterRole
	orgUserRole           *rbacv1.ClusterRole
	rootNamespaceUserRole *rbacv1.ClusterRole
	userName              string
	nsPerms               *authorization.NamespacePermissions
)

func TestDescriptors(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Descriptors Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	Expect(metav1.AddMetaToScheme(scheme.Scheme)).To(Succeed())
	metav1.AddToGroupVersion(scheme.Scheme, metav1.SchemeGroupVersion)
	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())
})

var _ = BeforeEach(func() {
	ctx = context.Background()

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "helm", "korifi", "controllers", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}
	_, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	userName = uuid.NewString()
	cert, key := testhelpers.ObtainClientCert(testEnv, userName)
	authInfo = authorization.Info{
		CertData: testhelpers.JoinCertAndKey(cert, key),
	}
	ctx = authorization.NewContext(ctx, &authInfo)

	k8sClient, err = client.NewWithWatch(testEnv.Config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	rootNamespaceUserRole = createClusterRole(context.Background(), "cf_root_namespace_user")
	orgUserRole = createClusterRole(context.Background(), "cf_org_user")
	spaceDeveloperRole = createClusterRole(context.Background(), "cf_space_developer")

	clientset, err := k8sclient.NewForConfig(testEnv.Config)
	Expect(err).NotTo(HaveOccurred())
	restClient = clientset.RESTClient()

	httpClient, err := restclient.HTTPClientFor(testEnv.Config)
	Expect(err).NotTo(HaveOccurred())
	mapper, err := apiutil.NewDynamicRESTMapper(testEnv.Config, httpClient)
	Expect(err).NotTo(HaveOccurred())
	tokenInspector := authorization.NewTokenReviewer(k8sClient)
	certInspector := authorization.NewCertInspector(testEnv.Config)
	baseIDProvider := authorization.NewCertTokenIdentityProvider(tokenInspector, certInspector)
	idProvider := authorization.NewCachingIdentityProvider(baseIDProvider, cache.NewExpiring())
	nsPerms = authorization.NewNamespacePermissions(k8sClient, idProvider)

	userClientFactory = authorization.NewUnprivilegedClientFactory(testEnv.Config, mapper, scheme.Scheme).
		WithWrappingFunc(func(client client.WithWatch) client.WithWatch {
			return k8s.NewRetryingClient(client, k8s.IsForbidden, k8s.NewDefaultBackoff())
		}).
		WithWrappingFunc(func(client client.WithWatch) client.WithWatch {
			return authorization.NewSpaceFilteringClient(client, k8sClient, authorization.NewSpaceFilteringOpts(nsPerms))
		})

	rootNamespace = uuid.NewString()
	Expect(k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: rootNamespace,
		},
	})).To(Succeed())
	createRoleBinding(context.Background(), userName, rootNamespaceUserRole.Name, rootNamespace)
})

var _ = AfterEach(func() {
	Expect(testEnv.Stop()).To(Succeed())
})

func createClusterRole(ctx context.Context, filename string) *rbacv1.ClusterRole {
	filepath := filepath.Join("..", "..", "..", "..", "helm", "korifi", "controllers", "cf_roles", filename+".yaml")
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

func createRoleBinding(ctx context.Context, userName, roleName, namespace string) rbacv1.RoleBinding {
	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: namespace,
			Labels:    map[string]string{},
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
	return roleBinding
}

func createOrg(ctx context.Context, displayName string) *korifiv1alpha1.CFOrg {
	guid := uuid.NewString()
	cfOrg := &korifiv1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: rootNamespace,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: displayName,
		},
	}
	Expect(k8sClient.Create(ctx, cfOrg)).To(Succeed())

	meta.SetStatusCondition(&(cfOrg.Status.Conditions), metav1.Condition{
		Type:    korifiv1alpha1.StatusConditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  "cus",
		Message: "cus",
	})
	Expect(k8sClient.Status().Update(ctx, cfOrg)).To(Succeed())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfOrg.Name,
			Labels: map[string]string{
				korifiv1alpha1.CFOrgDisplayNameKey: cfOrg.Spec.DisplayName,
			},
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

	return cfOrg
}

func createSpace(ctx context.Context, orgGUID, name string) *korifiv1alpha1.CFSpace {
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
		Type:    korifiv1alpha1.StatusConditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  "cus",
		Message: "cus",
	})
	Expect(k8sClient.Status().Update(ctx, cfSpace)).To(Succeed())

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfSpace.Name,
			Labels: map[string]string{
				korifiv1alpha1.CFSpaceDisplayNameKey: cfSpace.Spec.DisplayName,
			},
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

	return cfSpace
}
