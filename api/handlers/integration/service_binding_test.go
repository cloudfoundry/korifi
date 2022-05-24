package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ServiceBinding Handler", func() {
	const (
		secretName = "the-secret-name"
	)

	var (
		spaceGUID string
		appGUID   string
		cfApp     *v1alpha1.CFApp
	)

	BeforeEach(func() {
		appRepo := repositories.NewAppRepo(namespaceRetriever, clientFactory, nsPermissions)
		serviceInstanceRepo := repositories.NewServiceInstanceRepo(namespaceRetriever, clientFactory, nsPermissions)
		serviceBindingRepo := repositories.NewServiceBindingRepo(namespaceRetriever, clientFactory, nsPermissions)
		decoderValidator, err := NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler := NewServiceBindingHandler(
			*serverURL,
			serviceBindingRepo,
			appRepo,
			serviceInstanceRepo,
			decoderValidator,
		)
		apiHandler.RegisterRoutes(router)

		org := createOrgWithCleanup(ctx, generateGUID())
		space := createSpaceWithCleanup(ctx, org.Name, "spacename-"+generateGUID())
		spaceGUID = space.Name

		appGUID = generateGUID()
		cfApp = &v1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appGUID,
				Namespace: spaceGUID,
			},
			Spec: v1alpha1.CFAppSpec{
				DisplayName:  "name-for-" + appGUID,
				DesiredState: "STOPPED",
				Lifecycle: v1alpha1.Lifecycle{
					Type: "buildpack",
					Data: v1alpha1.LifecycleData{Buildpacks: []string{"java"}},
				},
			},
		}
		Expect(
			k8sClient.Create(context.Background(), cfApp),
		).To(Succeed())
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("POST /v3/service_credential_bindings", func() {
		var (
			serviceInstanceGUID string
			err                 error
		)
		BeforeEach(func() {
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, spaceGUID)

			serviceInstanceGUID = generateGUID()
			_ = createServiceInstance(context.Background(), k8sClient, serviceInstanceGUID, spaceGUID, "service-instance-name", secretName)
			req, err = http.NewRequestWithContext(
				ctx,
				"POST",
				serverURI("/v3/service_credential_bindings"),
				strings.NewReader(fmt.Sprintf(`{
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
				}`, appGUID, serviceInstanceGUID)),
			)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Add("Content-type", "application/json")
		})

		It("successfully returns the created service credential binding", func() {
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
		})

		It("successfully creates a CFServiceBinding", func() {
			guid := getServiceBindingGuidFromResponseBody(rr.Body)
			serviceBinding := new(v1alpha1.CFServiceBinding)
			Expect(
				k8sClient.Get(context.Background(), types.NamespacedName{Namespace: spaceGUID, Name: guid}, serviceBinding),
			).To(Succeed())

			Expect(serviceBinding.Labels).To(HaveKeyWithValue("servicebinding.io/provisioned-service", "true"))
			Expect(serviceBinding.Spec.AppRef).To(Equal(corev1.LocalObjectReference{Name: appGUID}))
			Expect(serviceBinding.Spec.Service).To(Equal(
				corev1.ObjectReference{
					APIVersion: "korifi.cloudfoundry.org/v1alpha1",
					Kind:       "CFServiceInstance",
					Name:       serviceInstanceGUID,
				},
			))
		})
	})

	Describe("GET /v3/service_credential_bindings", func() {
		var (
			serviceInstance1GUID, serviceBinding1GUID string
			serviceInstance2GUID, serviceBinding2GUID string
		)

		BeforeEach(func() {
			serviceInstance1GUID = generateGUID()
			serviceInstance1 := createServiceInstance(ctx, k8sClient, serviceInstance1GUID, spaceGUID, "service-instance-1-name", secretName)

			serviceBinding1GUID = generateGUID()
			serviceBindingName := "service-binding-1-name"
			_ = createServiceBinding(ctx, k8sClient, serviceBinding1GUID, spaceGUID, &serviceBindingName, serviceInstance1.Name, cfApp.Name)

			serviceInstance2GUID = generateGUID()
			serviceInstance2 := createServiceInstance(ctx, k8sClient, serviceInstance2GUID, spaceGUID, "service-instance-2-name", secretName)

			serviceBinding2GUID = generateGUID()
			_ = createServiceBinding(ctx, k8sClient, serviceBinding2GUID, spaceGUID, nil, serviceInstance2.Name, cfApp.Name)

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", serverURI("/v3/service_credential_bindings"), strings.NewReader(""))
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user has permission to view ServiceBindings", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, spaceGUID)
			})

			It("returns the list of visible ServiceBindings", func() {
				Expect(rr.Code).To(Equal(200))
				Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))
				response := map[string]interface{}{}
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response).To(HaveKeyWithValue("resources", HaveLen(2)))
			})

			When("the request includes a service instance GUID filter", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", serverURI("/v3/service_credential_bindings?service_instance_guids="+serviceInstance1GUID), strings.NewReader(""))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns the filtered list", func() {
					Expect(rr.Code).To(Equal(200))
					Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))
					response := map[string]interface{}{}
					err := json.Unmarshal(rr.Body.Bytes(), &response)
					Expect(err).NotTo(HaveOccurred())
					Expect(response).To(HaveKeyWithValue("resources", HaveLen(1)))
				})
			})
		})

		When("the user does not have permission to view ServiceBindings", func() {
			It("returns an empty list", func() {
				Expect(rr.Code).To(Equal(200))
				Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))
				response := map[string]interface{}{}
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response).To(HaveKeyWithValue("resources", HaveLen(0)))
			})
		})
	})
})

func getServiceBindingGuidFromResponseBody(body io.Reader) string {
	var response map[string]interface{}
	Expect(
		json.NewDecoder(body).Decode(&response),
	).To(Succeed())
	Expect(response).To(HaveKeyWithValue("guid", Not(BeEmpty())))
	return response["guid"].(string)
}

func createServiceInstance(ctx context.Context, k8sClient client.Client, serviceInstanceGUID, spaceGUID, name, secretName string) *v1alpha1.CFServiceInstance {
	toReturn := &v1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceInstanceGUID,
			Namespace: spaceGUID,
		},
		Spec: v1alpha1.CFServiceInstanceSpec{
			DisplayName: name,
			SecretName:  secretName,
			Type:        "user-provided",
		},
	}
	Expect(
		k8sClient.Create(ctx, toReturn),
	).To(Succeed())
	return toReturn
}

func createServiceBinding(ctx context.Context, k8sClient client.Client, serviceBindingGUID, spaceGUID string, name *string, serviceInstanceName, appName string) *v1alpha1.CFServiceBinding {
	toReturn := &v1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceBindingGUID,
			Namespace: spaceGUID,
		},
		Spec: v1alpha1.CFServiceBindingSpec{
			DisplayName: name,
			Service: corev1.ObjectReference{
				Kind:       "ServiceInstance",
				Name:       serviceInstanceName,
				APIVersion: "korifi.cloudfoundry.org/v1alpha1",
			},
			AppRef: corev1.LocalObjectReference{
				Name: appName,
			},
		},
	}
	Expect(
		k8sClient.Create(ctx, toReturn),
	).To(Succeed())
	return toReturn
}
