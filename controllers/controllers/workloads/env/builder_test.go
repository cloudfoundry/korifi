package env_test

import (
	"context"
	"encoding/json"
	"errors"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/controllers/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Builder", func() {
	const (
		vcapServicesData       = "{}"
		vcapServicesKey        = "VCAP_SERVICES"
		envSecretName          = "app-env-secret"
		vcapServicesSecretName = "app-guid-vcap-services"
	)

	var (
		cfClient                     *fake.Client
		listServiceBindingsError     error
		getServiceInstanceError      error
		getAppSecretError            error
		getServiceBindingSecretError error
		getVCAPServicesSecretError   error

		serviceBinding        korifiv1alpha1.CFServiceBinding
		serviceBinding2       korifiv1alpha1.CFServiceBinding
		serviceInstance       korifiv1alpha1.CFServiceInstance
		serviceInstance2      korifiv1alpha1.CFServiceInstance
		serviceBindingSecret  corev1.Secret
		serviceBindingSecret2 corev1.Secret
		vcapServicesSecret    corev1.Secret
		appSecret             corev1.Secret
		cfApp                 *korifiv1alpha1.CFApp

		builder *env.Builder

		envVars     []corev1.EnvVar
		buildEnvErr error
	)

	BeforeEach(func() {
		cfClient = new(fake.Client)
		builder = env.NewBuilder(cfClient)
		listServiceBindingsError = nil
		getServiceInstanceError = nil
		getAppSecretError = nil
		getServiceBindingSecretError = nil
		getVCAPServicesSecretError = nil

		serviceBindingName := "my-service-binding"
		serviceBinding = korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "service-binding-ns",
				Name:      "my-service-binding-guid",
			},
			Spec: korifiv1alpha1.CFServiceBindingSpec{
				DisplayName: &serviceBindingName,
				Service: corev1.ObjectReference{
					Name: "my-service-instance-guid",
				},
			},
			Status: korifiv1alpha1.CFServiceBindingStatus{
				Binding: corev1.LocalObjectReference{
					Name: "service-binding-secret",
				},
			},
		}
		serviceInstance = korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-service-instance-guid",
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "my-service-instance",
				Tags:        []string{"t1", "t2"},
			},
		}
		serviceBindingSecret = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "service-binding-secret"},
			Data: map[string][]byte{
				"foo": []byte("bar"),
			},
		}

		serviceBindingName2 := "my-service-binding-2"
		serviceBinding2 = korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "service-binding-ns",
				Name:      "my-service-binding-guid-2",
			},
			Spec: korifiv1alpha1.CFServiceBindingSpec{
				DisplayName: &serviceBindingName2,
				Service: corev1.ObjectReference{
					Name: "my-service-instance-guid-2",
				},
			},
			Status: korifiv1alpha1.CFServiceBindingStatus{
				Binding: corev1.LocalObjectReference{
					Name: "service-binding-secret-2",
				},
			},
		}
		serviceInstance2 = korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-service-instance-guid-2",
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "my-service-instance-2",
				Tags:        []string{"t1", "t2"},
			},
		}
		serviceBindingSecret2 = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "service-binding-secret-2"},
			Data: map[string][]byte{
				"bar": []byte("foo"),
			},
		}

		appSecret = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "app-ns",
				Name:      envSecretName,
			},
			Data: map[string][]byte{
				"app-secret": []byte("top-secret"),
			},
		}
		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "app-ns",
				Name:      "app-guid",
			},
			Spec: korifiv1alpha1.CFAppSpec{
				EnvSecretName: envSecretName,
			},
			Status: korifiv1alpha1.CFAppStatus{
				Conditions:             nil,
				ObservedDesiredState:   korifiv1alpha1.StoppedState,
				VCAPServicesSecretName: vcapServicesSecretName,
			},
		}

		meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "testing",
		})

		cfClient.ListStub = func(_ context.Context, objList client.ObjectList, _ ...client.ListOption) error {
			switch objList := objList.(type) {
			case *korifiv1alpha1.CFServiceBindingList:
				resultBinding := korifiv1alpha1.CFServiceBinding{}
				serviceBinding.DeepCopyInto(&resultBinding)
				resultBinding2 := korifiv1alpha1.CFServiceBinding{}
				serviceBinding2.DeepCopyInto(&resultBinding2)
				objList.Items = []korifiv1alpha1.CFServiceBinding{resultBinding, resultBinding2}
				return listServiceBindingsError
			default:
				panic("CfClient List provided a weird obj")
			}
		}

		vcapServicesSecret = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vcapServicesSecretName,
				Namespace: "app-ns",
			},
			Data: map[string][]byte{vcapServicesKey: []byte(vcapServicesData)},
		}

		cfClient.GetStub = func(_ context.Context, nsName types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFServiceInstance:
				if nsName.Name == serviceInstance.Name {
					serviceInstance.DeepCopyInto(obj)
				}
				if nsName.Name == serviceInstance2.Name {
					serviceInstance2.DeepCopyInto(obj)
				}
				return getServiceInstanceError
			case *corev1.Secret:
				if nsName.Name == envSecretName {
					appSecret.DeepCopyInto(obj)
					return getAppSecretError
				}
				if nsName.Name == vcapServicesSecretName {
					vcapServicesSecret.DeepCopyInto(obj)
					return getVCAPServicesSecretError
				}
				if nsName.Name == serviceBindingSecret.Name {
					serviceBindingSecret.DeepCopyInto(obj)
				}
				if nsName.Name == serviceBindingSecret2.Name {
					serviceBindingSecret2.DeepCopyInto(obj)
				}
				return getServiceBindingSecretError
			default:
				panic("CfClient Get provided a weird obj")
			}
		}
	})

	Describe("BuildEnv", func() {
		JustBeforeEach(func() {
			envVars, buildEnvErr = builder.BuildEnv(context.Background(), cfApp)
		})
		It("succeeds", func() {
			Expect(buildEnvErr).NotTo(HaveOccurred())
		})

		It("gets the app secrets (env and vcap services)", func() {
			Expect(cfClient.GetCallCount()).To(Equal(2))
			_, actualNsName, _, _ := cfClient.GetArgsForCall(0)
			Expect(actualNsName.Namespace).To(Equal(cfApp.Namespace))
			Expect(actualNsName.Name).To(Equal(cfApp.Spec.EnvSecretName))
			_, actualNsName, _, _ = cfClient.GetArgsForCall(1)
			Expect(actualNsName.Namespace).To(Equal(cfApp.Namespace))
			Expect(actualNsName.Name).To(Equal(cfApp.Status.VCAPServicesSecretName))
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
				getAppSecretError = apierrors.NewNotFound(schema.GroupResource{}, "boom")
			})

			It("errors", func() {
				Expect(buildEnvErr).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("getting the app env secret fails", func() {
			BeforeEach(func() {
				getAppSecretError = errors.New("get-app-secret-err")
			})

			It("returns an error", func() {
				Expect(buildEnvErr).To(MatchError(ContainSubstring("get-app-secret-err")))
			})
		})

		When("the app env secret is empty", func() {
			BeforeEach(func() {
				appSecret.Data = map[string][]byte{}
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

		When("the app env secret has no data", func() {
			BeforeEach(func() {
				appSecret.Data = nil
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
				getVCAPServicesSecretError = apierrors.NewNotFound(schema.GroupResource{}, "boom")
			})

			It("errors", func() {
				Expect(buildEnvErr).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("getting the app vcap services secret fails", func() {
			BeforeEach(func() {
				getVCAPServicesSecretError = errors.New("get-app-secret-err")
			})

			It("returns an error", func() {
				Expect(buildEnvErr).To(MatchError(ContainSubstring("get-app-secret-err")))
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

			When("the app does not have an associated app vcap services secret", func() {
				BeforeEach(func() {
					cfApp.Status.VCAPServicesSecretName = ""
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
			BuildVCAPServicesEnvValueErr error
		)

		JustBeforeEach(func() {
			vcapServicesString, BuildVCAPServicesEnvValueErr = builder.BuildVCAPServicesEnvValue(context.Background(), cfApp)
		})

		It("lists the service bindings for the app", func() {
			Expect(cfClient.ListCallCount()).To(Equal(1))
			_, _, actualListOpts := cfClient.ListArgsForCall(0)
			Expect(actualListOpts).To(HaveLen(2))
			Expect(actualListOpts[0]).To(Equal(client.InNamespace("app-ns")))
			Expect(actualListOpts[1]).To(Equal(client.MatchingFields{shared.IndexServiceBindingAppGUID: "app-guid"}))
		})

		It("gets the service instance for the binding", func() {
			Expect(cfClient.GetCallCount()).To(Equal(4))
			_, actualNsName, _, _ := cfClient.GetArgsForCall(0)
			Expect(actualNsName.Namespace).To(Equal("service-binding-ns"))
			Expect(actualNsName.Name).To(Equal("my-service-instance-guid"))
		})

		It("gets the secret for the bound service", func() {
			Expect(cfClient.GetCallCount()).To(Equal(4))
			_, actualNsName, _, _ := cfClient.GetArgsForCall(1)
			Expect(actualNsName.Namespace).To(Equal("service-binding-ns"))
			Expect(actualNsName.Name).To(Equal("service-binding-secret"))
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
				cfClient.ListReturns(nil)
			})

			It("returns an empty JSON string", func() {
				Expect(vcapServicesString).To(MatchJSON(`{}`))
			})
		})

		When("listing service bindings fails", func() {
			BeforeEach(func() {
				listServiceBindingsError = errors.New("list-service-bindings-err")
			})

			It("returns an error", func() {
				Expect(BuildVCAPServicesEnvValueErr).To(MatchError(ContainSubstring("list-service-bindings-err")))
			})
		})

		When("getting the service instance fails", func() {
			BeforeEach(func() {
				getServiceInstanceError = errors.New("get-service-instance-err")
			})

			It("returns an error", func() {
				Expect(BuildVCAPServicesEnvValueErr).To(MatchError(ContainSubstring("get-service-instance-err")))
			})
		})

		When("getting the service binding secret fails", func() {
			BeforeEach(func() {
				getServiceBindingSecretError = errors.New("get-service-binding-secret-err")
			})

			It("returns an error", func() {
				Expect(BuildVCAPServicesEnvValueErr).To(MatchError(ContainSubstring("get-service-binding-secret-err")))
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
