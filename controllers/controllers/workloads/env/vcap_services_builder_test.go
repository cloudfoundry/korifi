package env_test

import (
	"encoding/json"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/tools"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Builder", func() {
	var (
		serviceBinding       *korifiv1alpha1.CFServiceBinding
		serviceInstance      *korifiv1alpha1.CFServiceInstance
		serviceBindingSecret *corev1.Secret
		vcapServicesSecret   *corev1.Secret
		builder              *env.VCAPServicesEnvValueBuilder
	)

	BeforeEach(func() {
		builder = env.NewVCAPServicesEnvValueBuilder(controllersClient)

		serviceInstance = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      "my-service-instance-guid",
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "my-service-instance",
				Tags:        []string{"t1", "t2"},
				Type:        "user-provided",
			},
		}
		ensureCreate(serviceInstance)

		serviceBindingSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      "service-binding-secret",
			},
			Data: map[string][]byte{
				"foo": []byte("bar"),
			},
		}
		ensureCreate(serviceBindingSecret)

		serviceBindingName := "my-service-binding"
		serviceBinding = &korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      "my-service-binding-guid",
			},
			Spec: korifiv1alpha1.CFServiceBindingSpec{
				DisplayName: &serviceBindingName,
				Service: corev1.ObjectReference{
					Name: "my-service-instance-guid",
				},
				AppRef: corev1.LocalObjectReference{
					Name: "app-guid",
				},
			},
		}
		ensureCreate(serviceBinding)
		ensurePatch(serviceBinding, func(sb *korifiv1alpha1.CFServiceBinding) {
			sb.Status = korifiv1alpha1.CFServiceBindingStatus{
				Conditions: []metav1.Condition{},
				Binding: corev1.LocalObjectReference{
					Name: "service-binding-secret",
				},
			}
		})

		ensureCreate(&korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      "my-service-instance-guid-2",
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName:  "my-service-instance-2",
				Tags:         []string{"t1", "t2"},
				Type:         "user-provided",
				ServiceLabel: tools.PtrTo("custom-service-2"),
			},
		})

		ensureCreate(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      "service-binding-secret-2",
			},
			Data: map[string][]byte{
				"bar": []byte("foo"),
			},
		})

		serviceBindingName2 := "my-service-binding-2"
		serviceBinding2 := &korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      "my-service-binding-guid-2",
			},
			Spec: korifiv1alpha1.CFServiceBindingSpec{
				DisplayName: &serviceBindingName2,
				Service: corev1.ObjectReference{
					Name: "my-service-instance-guid-2",
				},
				AppRef: corev1.LocalObjectReference{
					Name: "app-guid",
				},
			},
		}
		ensureCreate(serviceBinding2)
		ensurePatch(serviceBinding2, func(sb *korifiv1alpha1.CFServiceBinding) {
			sb.Status = korifiv1alpha1.CFServiceBindingStatus{
				Conditions: []metav1.Condition{},
				Binding: corev1.LocalObjectReference{
					Name: "service-binding-secret-2",
				},
			}
		})

		vcapServicesSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-guid-vcap-services",
				Namespace: cfSpace.Status.GUID,
			},
			Data: map[string][]byte{"VCAP_SERVICES": []byte("{}")},
		}
		ensureCreate(vcapServicesSecret)
	})

	Describe("BuildVCAPServicesEnvValue", func() {
		var (
			vcapServices                 map[string]string
			buildVCAPServicesEnvValueErr error
		)

		JustBeforeEach(func() {
			vcapServices, buildVCAPServicesEnvValueErr = builder.BuildEnvValue(ctx, cfApp)
		})

		It("returns the service info", func() {
			Expect(buildVCAPServicesEnvValueErr).NotTo(HaveOccurred())

			Expect(extractServiceInfo(vcapServices, "user-provided", 1)).To(ContainElements(
				SatisfyAll(
					HaveLen(10),
					HaveKeyWithValue("label", "user-provided"),
					HaveKeyWithValue("name", "my-service-binding"),
					HaveKeyWithValue("tags", ConsistOf("t1", "t2")),
					HaveKeyWithValue("instance_guid", "my-service-instance-guid"),
					HaveKeyWithValue("instance_name", "my-service-instance"),
					HaveKeyWithValue("binding_guid", "my-service-binding-guid"),
					HaveKeyWithValue("binding_name", Equal("my-service-binding")),
					HaveKeyWithValue("credentials", SatisfyAll(HaveKeyWithValue("foo", "bar"), HaveLen(1))),
					HaveKeyWithValue("syslog_drain_url", BeNil()),
					HaveKeyWithValue("volume_mounts", BeEmpty()),
				),
			))
			Expect(extractServiceInfo(vcapServices, "custom-service-2", 1)).To(ContainElements(
				SatisfyAll(
					HaveLen(10),
					HaveKeyWithValue("label", "custom-service-2"),
					HaveKeyWithValue("name", "my-service-binding-2"),
					HaveKeyWithValue("tags", ConsistOf("t1", "t2")),
					HaveKeyWithValue("instance_guid", "my-service-instance-guid-2"),
					HaveKeyWithValue("instance_name", "my-service-instance-2"),
					HaveKeyWithValue("binding_guid", "my-service-binding-guid-2"),
					HaveKeyWithValue("binding_name", Equal("my-service-binding-2")),
					HaveKeyWithValue("credentials", SatisfyAll(HaveKeyWithValue("bar", "foo"), HaveLen(1))),
					HaveKeyWithValue("syslog_drain_url", BeNil()),
					HaveKeyWithValue("volume_mounts", BeEmpty()),
				),
			))
		})

		When("the service binding has no name", func() {
			BeforeEach(func() {
				ensurePatch(serviceBinding, func(s *korifiv1alpha1.CFServiceBinding) {
					s.Spec.DisplayName = nil
				})
			})

			It("uses the service instance name as name", func() {
				Expect(extractServiceInfo(vcapServices, "user-provided", 1)).To(ContainElement(HaveKeyWithValue("name", serviceInstance.Spec.DisplayName)))
			})

			It("sets the binding name to nil", func() {
				Expect(extractServiceInfo(vcapServices, "user-provided", 1)).To(ContainElement(HaveKeyWithValue("binding_name", BeNil())))
			})
		})

		When("service instance tags are nil", func() {
			BeforeEach(func() {
				ensurePatch(serviceInstance, func(s *korifiv1alpha1.CFServiceInstance) {
					s.Spec.Tags = nil
				})
			})

			It("sets an empty array to tags", func() {
				Expect(extractServiceInfo(vcapServices, "user-provided", 1)).To(ContainElement(HaveKeyWithValue("tags", BeEmpty())))
			})
		})

		When("serviceLabel is set but blank", func() {
			BeforeEach(func() {
				ensurePatch(serviceInstance, func(s *korifiv1alpha1.CFServiceInstance) {
					s.Spec.ServiceLabel = tools.PtrTo("")
				})
			})

			It("defaults the label to user-provided", func() {
				Expect(extractServiceInfo(vcapServices, "user-provided", 1)).NotTo(BeEmpty())
			})
		})

		When("both services use the same serviceLabel", func() {
			BeforeEach(func() {
				ensurePatch(serviceInstance, func(s *korifiv1alpha1.CFServiceInstance) {
					s.Spec.ServiceLabel = tools.PtrTo("custom-service-2")
				})
			})

			It("defaults the label to user-provided", func() {
				Expect(extractServiceInfo(vcapServices, "custom-service-2", 2)).NotTo(BeEmpty())
			})
		})

		When("there are no service bindings for the app", func() {
			BeforeEach(func() {
				Expect(adminClient.DeleteAllOf(ctx, &korifiv1alpha1.CFServiceBinding{}, client.InNamespace(cfSpace.Status.GUID))).To(Succeed())
				Eventually(func(g Gomega) {
					sbList := &korifiv1alpha1.CFServiceBindingList{}
					g.Expect(controllersClient.List(ctx, sbList, client.InNamespace(cfSpace.Status.GUID))).To(Succeed())
					g.Expect(sbList.Items).To(BeEmpty())
				}).Should(Succeed())
			})

			It("returns an empty JSON string", func() {
				Expect(vcapServices).To(HaveKeyWithValue("VCAP_SERVICES", "{}"))
			})
		})

		When("getting the service binding secret fails", func() {
			BeforeEach(func() {
				ensureDelete(serviceBindingSecret)
			})

			It("returns an error", func() {
				Expect(buildVCAPServicesEnvValueErr).To(MatchError(ContainSubstring("error fetching CFServiceBinding Secret")))
			})
		})
	})
})

func extractServiceInfo(vcapServicesData map[string]string, serviceKey string, expectedElements int) []map[string]interface{} {
	Expect(vcapServicesData).To(HaveKey("VCAP_SERVICES"))
	var vcapServices map[string]interface{}
	Expect(json.Unmarshal([]byte(vcapServicesData["VCAP_SERVICES"]), &vcapServices)).To(Succeed())

	Expect(vcapServices).To(HaveKey(serviceKey))

	serviceInfos, ok := vcapServices[serviceKey].([]interface{})
	Expect(ok).To(BeTrue())
	Expect(serviceInfos).To(HaveLen(expectedElements))

	infos := make([]map[string]interface{}, 0, expectedElements)
	for i := range serviceInfos {
		info, ok := serviceInfos[i].(map[string]interface{})
		Expect(ok).To(BeTrue())
		infos = append(infos, info)
	}

	return infos
}
