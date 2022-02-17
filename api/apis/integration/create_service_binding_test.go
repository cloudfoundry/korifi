package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
)

var _ = Describe("POST /v3/service_credential_bindings", func() {
	const (
		appGUID             = "the-app-guid"
		serviceInstanceGUID = "the-service-instance-guid"
		secretName          = "the-secret-name"
	)

	var spaceGUID string

	BeforeEach(func() {
		org := createOrgAnchorAndNamespace(ctx, rootNamespace, generateGUID())
		space := createSpaceAnchorAndNamespace(ctx, org.Name, "spacename-"+generateGUID())
		spaceGUID = space.Name

		createRoleBinding(ctx, userName, spaceDeveloperRole.Name, spaceGUID)

		cfApp := &workloadsv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appGUID,
				Namespace: spaceGUID,
			},
			Spec: workloadsv1alpha1.CFAppSpec{
				Name:         "name-for-" + appGUID,
				DesiredState: "STOPPED",
				Lifecycle: workloadsv1alpha1.Lifecycle{
					Type: "buildpack",
					Data: workloadsv1alpha1.LifecycleData{Buildpacks: []string{"java"}},
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

		serviceInstance := &servicesv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceInstanceGUID,
				Namespace: spaceGUID,
			},
			Spec: servicesv1alpha1.CFServiceInstanceSpec{
				Name:       "service-instance-name",
				SecretName: secretName,
				Type:       "user-provided",
			},
		}
		Expect(
			k8sClient.Create(context.Background(), serviceInstance),
		).To(Succeed())

		appRepo := repositories.NewAppRepo(k8sClient, clientFactory, nsPermissions)
		serviceInstanceRepo := repositories.NewServiceInstanceRepo(clientFactory, nsPermissions, namespaceGetter)
		serviceBindingRepo := repositories.NewServiceBindingRepo(clientFactory)
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())
		// set up apiHandler, repos, etc.
		apiHandler := NewServiceBindingHandler(
			logf.Log.WithName("TestServiceBinding"),
			*serverURL,
			serviceBindingRepo,
			appRepo,
			serviceInstanceRepo,
			decoderValidator,
		)
		apiHandler.RegisterRoutes(router)
	})

	When("type is `app`", func() {
		It("is successful and creates a CFServiceBinding resource", func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", serverURI("/v3/service_credential_bindings"), strings.NewReader(fmt.Sprintf(`{
			"relationships": {
				"app": {
					"data": {
						"guid": %q
					}
				},
				"service_instance": {
					"data": {
						"guid": %q
					}
				}
			},
			"type": "app"
		}`, appGUID, serviceInstanceGUID)))
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("Content-type", "application/json")
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(201))
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))

			var response map[string]interface{}
			Expect(
				json.NewDecoder(rr.Body).Decode(&response),
			).To(Succeed())

			Expect(response).To(HaveKeyWithValue("guid", Not(BeEmpty())))
			guid := response["guid"].(string)
			Expect(response).To(HaveKeyWithValue("type", Equal("app")))
			Expect(response).To(HaveKeyWithValue("name", BeNil()))
			Expect(response).To(HaveKeyWithValue("created_at", Not(BeEmpty())))
			Expect(response).To(HaveKeyWithValue("updated_at", Not(BeEmpty())))
			Expect(response).To(HaveKeyWithValue("last_operation", MatchAllKeys(Keys{
				"description": BeNil(),
				"state":       Equal("succeeded"),
				"type":        Equal("create"),
				"created_at":  Not(BeEmpty()),
				"updated_at":  Not(BeEmpty()),
			})))
			Expect(response).To(HaveKeyWithValue("relationships", map[string]interface{}{
				"app": map[string]interface{}{
					"data": map[string]interface{}{
						"guid": appGUID,
					},
				},
				"service_instance": map[string]interface{}{
					"data": map[string]interface{}{
						"guid": serviceInstanceGUID,
					},
				},
			}))
			Expect(response).To(HaveKeyWithValue("metadata", map[string]interface{}{
				"annotations": map[string]interface{}{},
				"labels":      map[string]interface{}{},
			}))
			Expect(response).To(HaveKeyWithValue("links", MatchAllKeys(Keys{
				"app":              HaveKeyWithValue("href", HaveSuffix("/v3/apps/"+appGUID)),
				"service_instance": HaveKeyWithValue("href", HaveSuffix("/v3/service_instances/"+serviceInstanceGUID)),
				"self":             HaveKeyWithValue("href", HaveSuffix("/v3/service_credential_bindings/"+guid)),
				"details":          HaveKeyWithValue("href", HaveSuffix("/v3/service_credential_bindings/"+guid+"/details")),
			})))

			serviceBinding := new(servicesv1alpha1.CFServiceBinding)
			Expect(
				k8sClient.Get(context.Background(), types.NamespacedName{Namespace: spaceGUID, Name: guid}, serviceBinding),
			).To(Succeed())

			Expect(serviceBinding.Labels).To(HaveKeyWithValue("servicebinding.io/provisioned-service", "true"))
			Expect(serviceBinding.Spec.AppRef).To(Equal(corev1.LocalObjectReference{Name: appGUID}))
			Expect(serviceBinding.Spec.Service).To(Equal(
				corev1.ObjectReference{
					APIVersion: "services.cloudfoundry.org/v1alpha1",
					Kind:       "CFServiceInstance",
					Name:       serviceInstanceGUID,
				},
			))
		})
	})
})
