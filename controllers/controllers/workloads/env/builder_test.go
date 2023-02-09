package env_test

import (
	"context"
	"encoding/json"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Builder", func() {
	const (
		vcapServicesData       = "{}"
		vcapServicesKey        = "VCAP_SERVICES"
		envSecretName          = "app-env-secret"
		vcapServicesSecretName = "app-guid-vcap-services"
		cfAPIURL               = "https://foo.bar"
	)

	var (
		serviceBinding       *korifiv1alpha1.CFServiceBinding
		serviceInstance      *korifiv1alpha1.CFServiceInstance
		serviceBindingSecret *corev1.Secret
		vcapServicesSecret   *corev1.Secret
		appSecret            *corev1.Secret
		cfApp                *korifiv1alpha1.CFApp

		builder *env.Builder

		envVars     []corev1.EnvVar
		buildEnvErr error
	)

	BeforeEach(func() {
		builder = env.NewBuilder(k8sClient)

		ctx = context.Background()
		serviceInstance = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      "my-service-instance-guid",
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "my-service-instance",
				Tags:        []string{"t1", "t2"},
				Type:        "user-provided",
			},
		}
		Expect(k8sClient.Create(ctx, serviceInstance)).To(Succeed())

		serviceBindingSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      "service-binding-secret",
			},
			Data: map[string][]byte{
				"foo": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, serviceBindingSecret)).To(Succeed())

		serviceBindingName := "my-service-binding"
		serviceBinding = createWithStatus(&korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
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
		}, func(sb *korifiv1alpha1.CFServiceBinding) {
			sb.Status = korifiv1alpha1.CFServiceBindingStatus{
				Conditions: []metav1.Condition{},
				Binding: corev1.LocalObjectReference{
					Name: "service-binding-secret",
				},
			}
		})

		Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      "my-service-instance-guid-2",
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "my-service-instance-2",
				Tags:        []string{"t1", "t2"},
				Type:        "user-provided",
			},
		})).To(Succeed())

		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      "service-binding-secret-2",
			},
			Data: map[string][]byte{
				"bar": []byte("foo"),
			},
		})).To(Succeed())

		serviceBindingName2 := "my-service-binding-2"
		createWithStatus(&korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
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
		}, func(sb *korifiv1alpha1.CFServiceBinding) {
			sb.Status = korifiv1alpha1.CFServiceBindingStatus{
				Conditions: []metav1.Condition{},
				Binding: corev1.LocalObjectReference{
					Name: "service-binding-secret-2",
				},
			}
		})

		appSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      envSecretName,
			},
			Data: map[string][]byte{
				"app-secret": []byte("top-secret"),
			},
		}
		Expect(k8sClient.Create(ctx, appSecret)).To(Succeed())

		cfApp = createWithStatus(&korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      "app-guid",
			},
			Spec: korifiv1alpha1.CFAppSpec{
				EnvSecretName: envSecretName,
				DisplayName:   "app-display-name",
				DesiredState:  korifiv1alpha1.StoppedState,
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}, func(app *korifiv1alpha1.CFApp) {
			app.Status = korifiv1alpha1.CFAppStatus{
				Conditions:             []metav1.Condition{},
				VCAPServicesSecretName: vcapServicesSecretName,
			}
			meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
				Reason: "testing",
			})
		})

		vcapServicesSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vcapServicesSecretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{vcapServicesKey: []byte(vcapServicesData)},
		}
		Expect(k8sClient.Create(ctx, vcapServicesSecret)).To(Succeed())
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
				Expect(k8sClient.Delete(ctx, appSecret)).To(Succeed())
			})

			It("errors", func() {
				Expect(buildEnvErr).To(MatchError(ContainSubstring("error when trying to fetch app env Secret")))
			})
		})

		When("the app env secret is empty", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, appSecret, func() {
					appSecret.Data = map[string][]byte{}
				})).To(Succeed())
			})

			It("returns only vcap services env var", func() {
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
				Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
					cfApp.Spec.EnvSecretName = ""
				})).To(Succeed())
			})

			It("succeeds", func() {
				Expect(buildEnvErr).NotTo(HaveOccurred())
			})

			It("returns only app vcap services env var", func() {
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
				Expect(k8sClient.Delete(ctx, vcapServicesSecret)).To(Succeed())
			})

			It("errors", func() {
				Expect(buildEnvErr).To(MatchError(ContainSubstring("error when trying to fetch vcap services secret")))
			})
		})

		When("the app vcap services secret is empty", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, vcapServicesSecret, func() {
					vcapServicesSecret.Data = map[string][]byte{}
				})).To(Succeed())
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

		When("the app does not have an associated app vcap services secret", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
					cfApp.Status.VCAPServicesSecretName = ""
				})).To(Succeed())
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
				Expect(k8s.Patch(ctx, k8sClient, serviceBinding, func() {
					serviceBinding.Spec.DisplayName = nil
				})).To(Succeed())
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
				Expect(k8s.Patch(ctx, k8sClient, serviceInstance, func() {
					serviceInstance.Spec.Tags = nil
				})).To(Succeed())
			})

			It("sets an empty array to tags", func() {
				Expect(extractServiceInfo(vcapServicesString)).To(ContainElement(HaveKeyWithValue("tags", BeEmpty())))
			})
		})

		When("there are no service bindings for the app", func() {
			BeforeEach(func() {
				Expect(k8sClient.DeleteAllOf(ctx, &korifiv1alpha1.CFServiceBinding{}, client.InNamespace(testNamespace))).To(Succeed())
				Eventually(func(g Gomega) {
					sbList := &korifiv1alpha1.CFServiceBindingList{}
					g.Expect(k8sClient.List(ctx, sbList, client.InNamespace(testNamespace))).To(Succeed())
					g.Expect(sbList.Items).To(BeEmpty())
				}).Should(Succeed())
			})

			It("returns an empty JSON string", func() {
				Expect(vcapServicesString).To(MatchJSON(`{}`))
			})
		})

		When("getting the service binding secret fails", func() {
			BeforeEach(func() {
				Expect(k8sClient.Delete(ctx, serviceBindingSecret)).To(Succeed())
			})

			It("returns an error", func() {
				Expect(buildVCAPServicesEnvValueErr).To(MatchError(ContainSubstring("error fetching CFServiceBinding Secret")))
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

func createWithStatus[T any, PT k8s.ObjectWithDeepCopy[T]](obj PT, setStatus func(PT)) PT {
	Expect(k8sClient.Create(ctx, obj)).To(Succeed())
	Expect(k8s.Patch(ctx, k8sClient, obj, func() {
		setStatus(obj)
	})).To(Succeed())
	return obj
}
