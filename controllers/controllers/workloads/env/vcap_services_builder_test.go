package env_test

import (
	"encoding/json"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tools"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Builder", func() {
	var (
		serviceBinding     *korifiv1alpha1.CFServiceBinding
		serviceInstance    *korifiv1alpha1.CFServiceInstance
		credentialsSecret  *corev1.Secret
		vcapServicesSecret *corev1.Secret
		builder            *env.VCAPServicesEnvValueBuilder
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
		helpers.EnsureCreate(controllersClient, serviceInstance)

		credentialsData, err := json.Marshal(map[string]any{
			"foo": "bar",
		})
		Expect(err).NotTo(HaveOccurred())

		credentialsSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      uuid.NewString(),
			},
			Data: map[string][]byte{
				tools.CredentialsSecretKey: credentialsData,
			},
		}
		helpers.EnsureCreate(controllersClient, credentialsSecret)

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
		helpers.EnsureCreate(controllersClient, serviceBinding)
		helpers.EnsurePatch(controllersClient, serviceBinding, func(sb *korifiv1alpha1.CFServiceBinding) {
			sb.Status = korifiv1alpha1.CFServiceBindingStatus{
				Credentials: corev1.LocalObjectReference{
					Name: credentialsSecret.Name,
				},
			}
		})

		helpers.EnsureCreate(controllersClient, &korifiv1alpha1.CFServiceInstance{
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
		helpers.EnsureCreate(controllersClient, serviceBinding2)
		helpers.EnsurePatch(controllersClient, serviceBinding2, func(sb *korifiv1alpha1.CFServiceBinding) {
			sb.Status = korifiv1alpha1.CFServiceBindingStatus{
				Credentials: corev1.LocalObjectReference{
					Name: credentialsSecret.Name,
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
		helpers.EnsureCreate(controllersClient, vcapServicesSecret)
	})

	Describe("BuildVCAPServicesEnvValue", func() {
		var (
			vcapServices                 map[string][]byte
			buildVCAPServicesEnvValueErr error
		)

		JustBeforeEach(func() {
			vcapServices, buildVCAPServicesEnvValueErr = builder.BuildEnvValue(ctx, cfApp)
		})

		It("returns the service info", func() {
			Expect(buildVCAPServicesEnvValueErr).NotTo(HaveOccurred())

			Expect(parseVcapServices(vcapServices)).To(MatchAllKeys(Keys{
				"user-provided": ConsistOf(MatchAllKeys(Keys{
					"label":         Equal("user-provided"),
					"name":          Equal("my-service-binding"),
					"tags":          ConsistOf("t1", "t2"),
					"instance_guid": Equal("my-service-instance-guid"),
					"instance_name": Equal("my-service-instance"),
					"binding_guid":  Equal("my-service-binding-guid"),
					"binding_name":  Equal("my-service-binding"),
					"credentials": MatchAllKeys(Keys{
						"foo": Equal("bar"),
					}),
					"syslog_drain_url": BeNil(),
					"volume_mounts":    BeEmpty(),
				})),
				"custom-service-2": ConsistOf(MatchAllKeys(Keys{
					"label":         Equal("custom-service-2"),
					"name":          Equal("my-service-binding-2"),
					"tags":          ConsistOf("t1", "t2"),
					"instance_guid": Equal("my-service-instance-guid-2"),
					"instance_name": Equal("my-service-instance-2"),
					"binding_guid":  Equal("my-service-binding-guid-2"),
					"binding_name":  Equal("my-service-binding-2"),
					"credentials": MatchAllKeys(Keys{
						"foo": Equal("bar"),
					}),
					"syslog_drain_url": BeNil(),
					"volume_mounts":    BeEmpty(),
				})),
			}))
		})

		When("the service binding has no name", func() {
			BeforeEach(func() {
				helpers.EnsurePatch(controllersClient, serviceBinding, func(s *korifiv1alpha1.CFServiceBinding) {
					s.Spec.DisplayName = nil
				})
			})

			It("uses the service instance name as name", func() {
				Expect(parseVcapServices(vcapServices)).To(MatchKeys(IgnoreExtras, Keys{
					"user-provided": ConsistOf(MatchKeys(IgnoreExtras, Keys{
						"name": Equal(serviceInstance.Spec.DisplayName),
					})),
				}))
			})

			It("sets the binding name to nil", func() {
				Expect(parseVcapServices(vcapServices)).To(MatchKeys(IgnoreExtras, Keys{
					"user-provided": ConsistOf(MatchKeys(IgnoreExtras, Keys{
						"binding_name": BeNil(),
					})),
				}))
			})
		})

		When("service instance tags are nil", func() {
			BeforeEach(func() {
				helpers.EnsurePatch(controllersClient, serviceInstance, func(s *korifiv1alpha1.CFServiceInstance) {
					s.Spec.Tags = nil
				})
			})

			It("sets an empty array to tags", func() {
				Expect(parseVcapServices(vcapServices)).To(MatchKeys(IgnoreExtras, Keys{
					"user-provided": ConsistOf(MatchKeys(IgnoreExtras, Keys{
						"tags": BeEmpty(),
					})),
				}))
			})
		})

		When("serviceLabel is set but blank", func() {
			BeforeEach(func() {
				helpers.EnsurePatch(controllersClient, serviceInstance, func(s *korifiv1alpha1.CFServiceInstance) {
					s.Spec.ServiceLabel = tools.PtrTo("")
				})
			})

			It("defaults the label to user-provided", func() {
				Expect(parseVcapServices(vcapServices)).To(MatchKeys(IgnoreExtras, Keys{
					"user-provided": Not(BeEmpty()),
				}))
			})
		})

		When("both services use the same serviceLabel", func() {
			BeforeEach(func() {
				helpers.EnsurePatch(controllersClient, serviceInstance, func(s *korifiv1alpha1.CFServiceInstance) {
					s.Spec.ServiceLabel = tools.PtrTo("custom-service-2")
				})
			})

			It("defaults the label to user-provided", func() {
				Expect(parseVcapServices(vcapServices)).To(MatchKeys(IgnoreExtras, Keys{
					"custom-service-2": Not(BeEmpty()),
				}))
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
				Expect(vcapServices).To(HaveKeyWithValue("VCAP_SERVICES", BeEquivalentTo("{}")))
			})
		})

		When("getting the service binding secret fails", func() {
			BeforeEach(func() {
				helpers.EnsureDelete(controllersClient, credentialsSecret)
			})

			It("returns an error", func() {
				Expect(buildVCAPServicesEnvValueErr).To(MatchError(ContainSubstring("error fetching CFServiceBinding Secret")))
			})
		})
	})
})

func parseVcapServices(vcapServicesData map[string][]byte) map[string]any {
	Expect(vcapServicesData).To(HaveKey("VCAP_SERVICES"))
	var vcapServices map[string]any
	Expect(json.Unmarshal([]byte(vcapServicesData["VCAP_SERVICES"]), &vcapServices)).To(Succeed())

	return vcapServices
}
