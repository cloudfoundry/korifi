package env_test

import (
	"context"
	"encoding/json"
	"errors"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Builder", func() {
	var (
		cfClient                     *fake.CFClient
		listServiceBindingsError     error
		getServiceInstanceError      error
		getAppSecretError            error
		getServiceBindingSecretError error

		serviceBinding       korifiv1alpha1.CFServiceBinding
		serviceInstance      korifiv1alpha1.CFServiceInstance
		serviceBindingSecret corev1.Secret
		appSecret            corev1.Secret
		cfApp                *korifiv1alpha1.CFApp

		builder *env.Builder

		envMap      map[string]string
		buildEnvErr error
	)

	BeforeEach(func() {
		cfClient = new(fake.CFClient)
		builder = env.NewBuilder(cfClient)
		listServiceBindingsError = nil
		getServiceInstanceError = nil
		getAppSecretError = nil
		getServiceBindingSecretError = nil

		serviceBindingName := "my-service-binding"
		serviceBinding = korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "service-binding-ns",
				Name:      "my-service-binding-guid",
			},
			Spec: korifiv1alpha1.CFServiceBindingSpec{
				DisplayName: &serviceBindingName,
				Service: corev1.ObjectReference{
					Name: "bound-service",
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
			Data: map[string][]byte{
				"foo": []byte("bar"),
			},
		}
		appSecret = corev1.Secret{
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
				EnvSecretName: "app-env-secret",
			},
		}

		cfClient.ListStub = func(_ context.Context, objList client.ObjectList, _ ...client.ListOption) error {
			switch objList := objList.(type) {
			case *korifiv1alpha1.CFServiceBindingList:
				resultBinding := korifiv1alpha1.CFServiceBinding{}
				serviceBinding.DeepCopyInto(&resultBinding)
				objList.Items = []korifiv1alpha1.CFServiceBinding{resultBinding}
				return listServiceBindingsError
			default:
				panic("CfClient List provided a weird obj")
			}
		}

		cfClient.GetStub = func(_ context.Context, nsName types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFServiceInstance:
				serviceInstance.DeepCopyInto(obj)
				return getServiceInstanceError
			case *corev1.Secret:
				if nsName.Name == "app-env-secret" {
					appSecret.DeepCopyInto(obj)
					return getAppSecretError
				}

				serviceBindingSecret.DeepCopyInto(obj)
				return getServiceBindingSecretError
			default:
				panic("CfClient Get provided a weird obj")
			}
		}
	})

	JustBeforeEach(func() {
		envMap, buildEnvErr = builder.BuildEnv(context.Background(), cfApp)
	})

	It("succeeds", func() {
		Expect(buildEnvErr).NotTo(HaveOccurred())
	})

	It("gets the app env secret", func() {
		Expect(cfClient.GetCallCount()).To(Equal(3))
		_, actualNsName, _ := cfClient.GetArgsForCall(0)
		Expect(actualNsName.Namespace).To(Equal(cfApp.Namespace))
		Expect(actualNsName.Name).To(Equal(cfApp.Spec.EnvSecretName))
	})

	It("lists the service bindings for the app", func() {
		Expect(cfClient.ListCallCount()).To(Equal(1))
		_, _, actualListOpts := cfClient.ListArgsForCall(0)
		Expect(actualListOpts).To(HaveLen(2))
		Expect(actualListOpts[0]).To(Equal(client.InNamespace("app-ns")))
		Expect(actualListOpts[1]).To(Equal(client.MatchingFields{shared.IndexServiceBindingAppGUID: "app-guid"}))
	})

	It("gets the service instance for the binding", func() {
		Expect(cfClient.GetCallCount()).To(Equal(3))
		_, actualNsName, _ := cfClient.GetArgsForCall(1)
		Expect(actualNsName.Namespace).To(Equal("service-binding-ns"))
		Expect(actualNsName.Name).To(Equal("bound-service"))
	})

	It("gets the secret for the bound service", func() {
		Expect(cfClient.GetCallCount()).To(Equal(3))
		_, actualNsName, _ := cfClient.GetArgsForCall(2)
		Expect(actualNsName.Namespace).To(Equal("service-binding-ns"))
		Expect(actualNsName.Name).To(Equal("service-binding-secret"))
	})

	It("adds VCAP_SERVICES var to the app env secret", func() {
		Expect(cfClient.PatchCallCount()).To(Equal(1))
		_, patchedObject, patchType, _ := cfClient.PatchArgsForCall(0)

		patchedSecret, ok := patchedObject.(*corev1.Secret)
		Expect(ok).To(BeTrue())
		Expect(patchedSecret.Namespace).To(Equal(appSecret.Namespace))
		Expect(patchedSecret.Name).To(Equal(appSecret.Name))
		Expect(patchedSecret.Data).To(HaveKey("VCAP_SERVICES"))

		Expect(patchType.Type()).To(Equal(types.MergePatchType))
	})

	When("patching the app env secret fails", func() {
		BeforeEach(func() {
			cfClient.PatchReturns(errors.New("patch-err"))
		})

		It("returns an error", func() {
			Expect(buildEnvErr).To(MatchError(ContainSubstring("patch-err")))
		})
	})

	It("returns both the user defined env vars and the VCAP_SERVICES env var", func() {
		Expect(envMap).To(SatisfyAll(
			HaveLen(2),
			HaveKeyWithValue("app-secret", "top-secret"),
			HaveKey("VCAP_SERVICES"),
		))

		Expect(extractServiceInfo(envMap)).To(SatisfyAll(
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
			HaveKeyWithValue("volume_mounts", BeEmpty())),
		)
	})

	When("the service binding has no name", func() {
		BeforeEach(func() {
			serviceBinding.Spec.DisplayName = nil
		})

		It("uses the service instance name as name", func() {
			Expect(extractServiceInfo(envMap)).To(HaveKeyWithValue("name", serviceInstance.Spec.DisplayName))
		})

		It("sets the binding name to nil", func() {
			Expect(extractServiceInfo(envMap)).To(HaveKeyWithValue("binding_name", BeNil()))
		})
	})

	When("service instance tags are nil", func() {
		BeforeEach(func() {
			serviceInstance.Spec.Tags = nil
		})

		It("sets an empty array to tags", func() {
			Expect(extractServiceInfo(envMap)).To(HaveKeyWithValue("tags", BeEmpty()))
		})
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

		It("returns the VCAP_SERVICES env var only", func() {
			Expect(envMap).To(SatisfyAll(
				HaveLen(1),
				HaveKey("VCAP_SERVICES"),
			))
		})
	})

	When("the app env secret is nil", func() {
		BeforeEach(func() {
			appSecret.Data = nil
		})

		It("returns the VCAP_SERVICES env var only", func() {
			Expect(envMap).To(SatisfyAll(
				HaveLen(1),
				HaveKey("VCAP_SERVICES"),
			))
		})
	})

	When("the app does not have an associated app env secret", func() {
		BeforeEach(func() {
			cfApp.Spec.EnvSecretName = ""
		})

		It("succeeds", func() {
			Expect(buildEnvErr).NotTo(HaveOccurred())
		})

		It("returns an empty set of env vars", func() {
			Expect(envMap).To(BeEmpty())
		})
	})

	When("there are no service bindings for the app", func() {
		BeforeEach(func() {
			cfClient.ListReturns(nil)
		})

		It("still returns the user defined app env vars", func() {
			Expect(envMap).To(HaveKeyWithValue("app-secret", "top-secret"))
		})

		It("returns an empty json as the value of the VCAP_SERVICES var", func() {
			Expect(envMap).To(HaveKeyWithValue("VCAP_SERVICES", "{}"))
		})
	})

	When("listing service bindings fails", func() {
		BeforeEach(func() {
			listServiceBindingsError = errors.New("list-service-bindings-err")
		})

		It("returns an error", func() {
			Expect(buildEnvErr).To(MatchError(ContainSubstring("list-service-bindings-err")))
		})
	})

	When("getting the service instance fails", func() {
		BeforeEach(func() {
			getServiceInstanceError = errors.New("get-service-instance-err")
		})

		It("returns an error", func() {
			Expect(buildEnvErr).To(MatchError(ContainSubstring("get-service-instance-err")))
		})
	})

	When("getting the service binding secret fails", func() {
		BeforeEach(func() {
			getServiceBindingSecretError = errors.New("get-service-binding-secret-err")
		})

		It("returns an error", func() {
			Expect(buildEnvErr).To(MatchError(ContainSubstring("get-service-binding-secret-err")))
		})
	})
})

func extractServiceInfo(envMap map[string]string) map[string]interface{} {
	var vcapServices map[string]interface{}
	Expect(json.Unmarshal([]byte(envMap["VCAP_SERVICES"]), &vcapServices)).To(Succeed())

	Expect(vcapServices).To(HaveLen(1))
	Expect(vcapServices).To(HaveKey("user-provided"))

	serviceInfos, ok := vcapServices["user-provided"].([]interface{})
	Expect(ok).To(BeTrue())
	Expect(serviceInfos).To(HaveLen(1))

	info, ok := serviceInfos[0].(map[string]interface{})
	Expect(ok).To(BeTrue())

	return info
}
