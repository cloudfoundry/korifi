package env_test

import (
	"context"
	"encoding/json"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tools"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Builder", func() {
	var (
		serviceBinding        *korifiv1alpha1.CFServiceBinding
		serviceBinding2       *korifiv1alpha1.CFServiceBinding
		serviceInstance       *korifiv1alpha1.CFServiceInstance
		serviceInstance2      *korifiv1alpha1.CFServiceInstance
		serviceBindingSecret  *corev1.Secret
		serviceBindingSecret2 *corev1.Secret
		vcapServicesSecret    *corev1.Secret
		appEnvSecret          *corev1.Secret
		cfApp                 *korifiv1alpha1.CFApp

		builder *env.Builder

		envVars     []corev1.EnvVar
		buildEnvErr error
	)

	BeforeEach(func() {
		builder = env.NewBuilder(k8sClient)

		appEnvSecret = helpers.RegisterObject(fixture, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "app-env-secret",
			},
			Data: map[string][]byte{
				"app-secret": []byte("top-secret"),
			},
		})

		vcapServicesSecret = helpers.RegisterObject(fixture, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "app-guid-vcap-services",
			},
			Data: map[string][]byte{"VCAP_SERVICES": []byte("{}")},
		})

		cfApp = helpers.RegisterObject(fixture, &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "app-guid",
			},
			Spec: korifiv1alpha1.CFAppSpec{
				EnvSecretName: appEnvSecret.Name,
				DesiredState:  korifiv1alpha1.DesiredState("STOPPED"),
				DisplayName:   "my-app",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
			Status: korifiv1alpha1.CFAppStatus{
				ObservedDesiredState:   korifiv1alpha1.StoppedState,
				VCAPServicesSecretName: vcapServicesSecret.Name,
			},
		})
		meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "testing",
		})

		serviceInstance = helpers.RegisterObject(fixture, &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "my-service-instance-guid",
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "my-service-instance",
				Tags:        []string{"t1", "t2"},
				Type:        "user-provided",
			},
		})

		serviceBindingSecret = helpers.RegisterObject(fixture, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "service-binding-secret",
			},
			Data: map[string][]byte{
				"foo": []byte("bar"),
			},
		})

		serviceBinding = helpers.RegisterObject(fixture, &korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "my-service-binding-guid",
			},
			Spec: korifiv1alpha1.CFServiceBindingSpec{
				DisplayName: tools.PtrTo("my-service-binding"),
				Service: corev1.ObjectReference{
					Name: serviceInstance.Name,
				},
				AppRef: corev1.LocalObjectReference{
					Name: cfApp.Name,
				},
			},
			Status: korifiv1alpha1.CFServiceBindingStatus{
				Conditions: []metav1.Condition{},
				Binding: corev1.LocalObjectReference{
					Name: serviceBindingSecret.Name,
				},
			},
		})

		serviceInstance2 = helpers.RegisterObject(fixture, &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "my-service-instance-guid-2",
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "my-service-instance-2",
				Tags:        []string{"t1", "t2"},
				Type:        "user-provided",
			},
		})

		serviceBindingSecret2 = helpers.RegisterObject(fixture, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "service-binding-secret-2",
			},
			Data: map[string][]byte{
				"bar": []byte("foo"),
			},
		})

		serviceBinding2 = helpers.RegisterObject(fixture, &korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "my-service-binding-guid-2",
			},
			Spec: korifiv1alpha1.CFServiceBindingSpec{
				DisplayName: tools.PtrTo("my-service-binding-2"),
				Service: corev1.ObjectReference{
					Name: serviceInstance2.Name,
				},
				AppRef: corev1.LocalObjectReference{
					Name: cfApp.Name,
				},
			},
			Status: korifiv1alpha1.CFServiceBindingStatus{
				Conditions: []metav1.Condition{},
				Binding: corev1.LocalObjectReference{
					Name: serviceBindingSecret2.Name,
				},
			},
		})
	})

	JustBeforeEach(func() {
		fixture.CreateAllRegisteredObjects()
	})

	Describe("BuildEnv", func() {
		JustBeforeEach(func() {
			envVars, buildEnvErr = builder.BuildEnv(context.Background(), cfApp)
		})

		It("succeeds", func() {
			Expect(buildEnvErr).NotTo(HaveOccurred())
		})

		It("returns the user defined and vcap services env vars", func() {
			Expect(envVars).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal("VCAP_SERVICES"),
					"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
						"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cfApp.Status.VCAPServicesSecretName,
							},
							Key: "VCAP_SERVICES",
						})),
					})),
				}),
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal("app-secret"),
					"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
						"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cfApp.Spec.EnvSecretName,
							},
							Key: "app-secret",
						})),
					})),
				}),
			))
		})

		When("the app env secret does not exist", func() {
			BeforeEach(func() {
				fixture.DeregisterObject(appEnvSecret)
			})

			It("errors", func() {
				Expect(buildEnvErr).To(MatchError(ContainSubstring("fetch app env Secret")))
			})
		})

		When("the app env secret is empty", func() {
			BeforeEach(func() {
				appEnvSecret.Data = map[string][]byte{}
			})

			It("returns only vcap services env var", func() {
				actualSecret := &corev1.Secret{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vcapServicesSecret), actualSecret)).To(Succeed())

				Expect(buildEnvErr).NotTo(HaveOccurred())
				Expect(envVars).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Name": Equal("VCAP_SERVICES"),
					"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
						"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cfApp.Status.VCAPServicesSecretName,
							},
							Key: "VCAP_SERVICES",
						})),
					})),
				})))
			})
		})

		When("the app env secret has no data", func() {
			BeforeEach(func() {
				appEnvSecret.Data = nil
			})

			It("returns only the vcap services env var", func() {
				Expect(envVars).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Name": Equal("VCAP_SERVICES"),
					"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
						"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cfApp.Status.VCAPServicesSecretName,
							},
							Key: "VCAP_SERVICES",
						})),
					})),
				})))
			})
		})

		When("the app does not have an associated app env secret", func() {
			BeforeEach(func() {
				cfApp.Spec.EnvSecretName = ""
			})

			It("succeeds", func() {
				Expect(buildEnvErr).NotTo(HaveOccurred())
			})

			It("returns only app vcap services env var", func() {
				Expect(buildEnvErr).NotTo(HaveOccurred())
				Expect(envVars).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Name": Equal("VCAP_SERVICES"),
					"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
						"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cfApp.Status.VCAPServicesSecretName,
							},
							Key: "VCAP_SERVICES",
						})),
					})),
				})))
			})
		})

		When("the app vcap services secret does not exist", func() {
			BeforeEach(func() {
				fixture.DeregisterObject(vcapServicesSecret)
			})

			It("errors", func() {
				Expect(buildEnvErr).To(MatchError(ContainSubstring("not found")))
			})
		})

		// This test block drives out error handling code, but the system should not reach these states
		When("the app vcap services secret data is malformed", func() {
			When("the app vcap services secret is empty", func() {
				BeforeEach(func() {
					vcapServicesSecret.Data = map[string][]byte{}
				})

				It("returns only app env vars", func() {
					Expect(envVars).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
						"Name": Equal("app-secret"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cfApp.Spec.EnvSecretName,
								},
								Key: "app-secret",
							})),
						})),
					})))
				})
			})

			When("the app vcap services secret has no data", func() {
				BeforeEach(func() {
					vcapServicesSecret.Data = nil
				})

				It("returns only the app env var", func() {
					Expect(envVars).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
						"Name": Equal("app-secret"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cfApp.Spec.EnvSecretName,
								},
								Key: "app-secret",
							})),
						})),
					})))
				})
			})

			When("the app does not have an associated app vcap services secret yet", func() {
				BeforeEach(func() {
					cfApp.Status = korifiv1alpha1.CFAppStatus{}
				})

				It("succeeds", func() {
					Expect(buildEnvErr).NotTo(HaveOccurred())
				})

				It("returns only the app env vars", func() {
					Expect(envVars).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
						"Name": Equal("app-secret"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cfApp.Spec.EnvSecretName,
								},
								Key: "app-secret",
							})),
						})),
					})))
				})
			})
		})
	})

	Describe("BuildVCAPServicesEnvValue", func() {
		var (
			vcapServicesString           string
			buildVCAPServicesEnvValueErr error
		)

		JustBeforeEach(func() {
			vcapServicesString, buildVCAPServicesEnvValueErr = builder.BuildVCAPServicesEnvValue(context.Background(), cfApp)
		})

		It("returns the service info", func() {
			Expect(extractServiceInfo(vcapServicesString)).To(ContainElements(
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
				SatisfyAll(
					HaveLen(10),
					HaveKeyWithValue("label", "user-provided"),
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
				serviceBinding.Spec.DisplayName = nil
			})

			It("uses the service instance name as name", func() {
				Expect(extractServiceInfo(vcapServicesString)).To(ContainElement(HaveKeyWithValue("name", serviceInstance.Spec.DisplayName)))
			})

			It("sets the binding name to nil", func() {
				Expect(extractServiceInfo(vcapServicesString)).To(ContainElement(HaveKeyWithValue("binding_name", BeNil())))
			})
		})

		When("service instance tags are nil", func() {
			BeforeEach(func() {
				serviceInstance.Spec.Tags = nil
			})

			It("sets an empty array to tags", func() {
				Expect(extractServiceInfo(vcapServicesString)).To(ContainElement(HaveKeyWithValue("tags", BeEmpty())))
			})
		})

		When("there are no service bindings for the app", func() {
			BeforeEach(func() {
				fixture.DeregisterObject(serviceBinding, serviceBinding2)
			})

			It("returns an empty JSON string", func() {
				Expect(vcapServicesString).To(MatchJSON(`{}`))
			})
		})

		When("the service referenced by the binding cannot be looked up", func() {
			BeforeEach(func() {
				fixture.DeregisterObject(serviceInstance)
			})

			It("returns an error", func() {
				Expect(buildVCAPServicesEnvValueErr).To(MatchError(ContainSubstring("not found")))
			})
		})

		When("the service binding secret cannot be looked up", func() {
			BeforeEach(func() {
				fixture.DeregisterObject(serviceBindingSecret)
			})

			It("returns an error", func() {
				Expect(buildVCAPServicesEnvValueErr).To(MatchError(ContainSubstring("not found")))
			})
		})
	})
})

func extractServiceInfo(vcapServicesData string) []map[string]interface{} {
	var vcapServices map[string]interface{}
	Expect(json.Unmarshal([]byte(vcapServicesData), &vcapServices)).To(Succeed())

	Expect(vcapServices).To(HaveLen(1))
	Expect(vcapServices).To(HaveKey("user-provided"))

	serviceInfos, ok := vcapServices["user-provided"].([]interface{})
	Expect(ok).To(BeTrue())
	Expect(serviceInfos).To(HaveLen(2))

	infos := make([]map[string]interface{}, 0, 2)
	for i := range serviceInfos {
		info, ok := serviceInfos[i].(map[string]interface{})
		Expect(ok).To(BeTrue())
		infos = append(infos, info)
	}

	return infos
}
