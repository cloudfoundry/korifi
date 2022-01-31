package integration_test

import (
	"context"
	"encoding/json"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
)

var _ = Describe("GET /v3/apps/:guid/env", func() {
	var namespace *corev1.Namespace

	BeforeEach(func() {
		clientFactory := repositories.NewUnprivilegedClientFactory(k8sConfig)
		identityProvider := new(fake.IdentityProvider)
		namespacePermissions := authorization.NewNamespacePermissions(k8sClient, identityProvider, "root-ns")

		appRepo := repositories.NewAppRepo(k8sClient, clientFactory, namespacePermissions)
		domainRepo := repositories.NewDomainRepo(k8sClient)
		processRepo := repositories.NewProcessRepo(k8sClient)
		routeRepo := repositories.NewRouteRepo(k8sClient, clientFactory)
		dropletRepo := repositories.NewDropletRepo(k8sClient, clientFactory)
		podRepo := repositories.NewPodRepo(k8sClient)
		scaleProcess := actions.NewScaleProcess(processRepo).Invoke
		scaleAppProcess := actions.NewScaleAppProcess(appRepo, processRepo, scaleProcess).Invoke
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewAppHandler(
			logf.Log.WithName("integration tests"),
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			podRepo,
			scaleAppProcess,
			decoderValidator,
		)
		apiHandler.RegisterRoutes(router)

		namespaceGUID := generateGUID()
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceGUID}}
		Expect(
			k8sClient.Create(context.Background(), namespace),
		).To(Succeed())

		DeferCleanup(func() {
			_ = k8sClient.Delete(context.Background(), namespace)
		})

		createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespaceGUID)
	})

	When("the app has environment variables", func() {
		var envVars map[string]string

		const (
			appGUID    = "my-app-1"
			secretName = "my-env-secret"
			key1       = "KEY1"
			key2       = "KEY2"
		)

		BeforeEach(func() {
			envVars = map[string]string{
				key1: "VAL1",
				key2: "VAL2",
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace.Name,
				},
				StringData: envVars,
			}
			Expect(
				k8sClient.Create(context.Background(), secret),
			).To(Succeed())

			_ = createAppWithGUID(namespace.Name, appGUID, secretName)
		})

		It("includes them in the response body", func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", serverURI("/v3/apps/", appGUID, "/env"), nil)
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("Content-type", "application/json")
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(200))
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))

			var response map[string]interface{}
			Expect(
				json.NewDecoder(rr.Body).Decode(&response),
			).To(Succeed())

			Expect(response).To(HaveKeyWithValue("environment_variables", MatchAllKeys(Keys{
				key1: BeEquivalentTo(envVars[key1]),
				key2: BeEquivalentTo(envVars[key2]),
			})))
			Expect(response).To(HaveKeyWithValue("staging_env_json", BeEmpty()))
			Expect(response).To(HaveKeyWithValue("running_env_json", BeEmpty()))
			Expect(response).To(HaveKeyWithValue("system_env_json", BeEmpty()))
			Expect(response).To(HaveKeyWithValue("application_env_json", BeEmpty()))
		})
	})
})

func createAppWithGUID(space, guid, secretName string) *workloadsv1alpha1.CFApp {
	cfApp := &workloadsv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: space,
		},
		Spec: workloadsv1alpha1.CFAppSpec{
			Name:          "name-for-" + guid,
			EnvSecretName: secretName,
			DesiredState:  "STOPPED",
			Lifecycle: workloadsv1alpha1.Lifecycle{
				Type: "buildpack",
				Data: workloadsv1alpha1.LifecycleData{
					Buildpacks: []string{"java"},
				},
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

	return cfApp
}
