package env_test

import (
	"context"
	"encoding/json"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
				DisplayName: "my-service-instance-2",
				Tags:        []string{"t1", "t2"},
				Type:        "user-provided",
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

		appSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      "app-env-secret",
			},
			Data: map[string][]byte{
				"app-secret": []byte("top-secret"),
			},
		}
		ensureCreate(appSecret)

		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      "app-guid",
			},
			Spec: korifiv1alpha1.CFAppSpec{
				EnvSecretName: "app-env-secret",
				DisplayName:   "app-display-name",
				DesiredState:  korifiv1alpha1.StoppedState,
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}
		ensureCreate(cfApp)
		ensurePatch(cfApp, func(app *korifiv1alpha1.CFApp) {
			app.Status = korifiv1alpha1.CFAppStatus{
				Conditions:                []metav1.Condition{},
				VCAPServicesSecretName:    "app-guid-vcap-services",
				VCAPApplicationSecretName: "app-guid-vcap-application",
			}
			meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				Reason:             "testing",
				LastTransitionTime: metav1.Date(2023, 2, 15, 12, 0, 0, 0, time.FixedZone("", 0)),
			})
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
				ensureDelete(appSecret)
			})

			It("errors", func() {
				Expect(buildEnvErr).To(MatchError(ContainSubstring("error when trying to fetch app env Secret")))
			})
		})

		When("the app env secret is empty", func() {
			BeforeEach(func() {
				ensurePatch(appSecret, func(s *corev1.Secret) {
					s.Data = map[string][]byte{}
				})
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
				ensurePatch(cfApp, func(a *korifiv1alpha1.CFApp) {
					a.Spec.EnvSecretName = ""
				})
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
				ensureDelete(vcapServicesSecret)
			})

			It("errors", func() {
				Expect(buildEnvErr).To(MatchError(ContainSubstring("error when trying to fetch vcap services secret")))
			})
		})

		When("the app vcap services secret is empty", func() {
			BeforeEach(func() {
				ensurePatch(vcapServicesSecret, func(s *corev1.Secret) {
					s.Data = map[string][]byte{}
				})
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
				ensurePatch(cfApp, func(a *korifiv1alpha1.CFApp) {
					a.Status.VCAPServicesSecretName = ""
				})
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
				ensurePatch(serviceBinding, func(sb *korifiv1alpha1.CFServiceBinding) {
					sb.Spec.DisplayName = nil
				})
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
				ensurePatch(serviceInstance, func(si *korifiv1alpha1.CFServiceInstance) {
					si.Spec.Tags = nil
				})
			})

			It("sets an empty array to tags", func() {
				Expect(extractServiceInfo(vcapServicesString)).To(ContainElement(HaveKeyWithValue("tags", BeEmpty())))
			})
		})

		When("there are no service bindings for the app", func() {
			BeforeEach(func() {
				Expect(k8sClient.DeleteAllOf(ctx, &korifiv1alpha1.CFServiceBinding{}, client.InNamespace(cfSpace.Status.GUID))).To(Succeed())
				Eventually(func(g Gomega) {
					sbList := &korifiv1alpha1.CFServiceBindingList{}
					g.Expect(k8sClient.List(ctx, sbList, client.InNamespace(cfSpace.Status.GUID))).To(Succeed())
					g.Expect(sbList.Items).To(BeEmpty())
				}).Should(Succeed())
			})

			It("returns an empty JSON string", func() {
				Expect(vcapServicesString).To(MatchJSON(`{}`))
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

	Describe("BuildVCAPApplicationEnvValue", func() {
		var (
			vcapApplicationString           string
			buildVCAPApplicationEnvValueErr error
		)

		JustBeforeEach(func() {
			vcapApplicationString, buildVCAPApplicationEnvValueErr = builder.BuildVCAPApplicationEnvValue(context.Background(), cfApp)
		})

		It("sets the basic fields", func() {
			Expect(buildVCAPApplicationEnvValueErr).ToNot(HaveOccurred())
			appMap := map[string]string{}
			Expect(json.Unmarshal([]byte(vcapApplicationString), &appMap)).To(Succeed())
			Expect(appMap).To(HaveKeyWithValue("application_id", cfApp.Name))
			Expect(appMap).To(HaveKeyWithValue("application_name", cfApp.Spec.DisplayName))
			Expect(appMap).To(HaveKeyWithValue("name", cfApp.Spec.DisplayName))
			Expect(appMap).To(HaveKeyWithValue("cf_api", BeEmpty()))
			Expect(appMap).To(HaveKeyWithValue("space_id", cfSpace.Name))
			Expect(appMap).To(HaveKeyWithValue("space_name", cfSpace.Spec.DisplayName))
			Expect(appMap).To(HaveKeyWithValue("organization_id", cfOrg.Name))
			Expect(appMap).To(HaveKeyWithValue("organization_name", cfOrg.Spec.DisplayName))
		})
	})
})

func ensureCreate(obj client.Object) {
	Expect(k8sClient.Create(ctx, obj)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
	}).Should(Succeed())
}

func ensurePatch[T any, PT k8s.ObjectWithDeepCopy[T]](obj PT, modifyFunc func(PT)) {
	Expect(k8s.Patch(ctx, k8sClient, obj, func() {
		modifyFunc(obj)
	})).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		objCopy := obj.DeepCopy()
		modifyFunc(objCopy)
		g.Expect(equality.Semantic.DeepEqual(objCopy, obj)).To(BeTrue())
	}).Should(Succeed())
}

func ensureDelete(obj client.Object) {
	Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
	}).Should(Succeed())
}

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
