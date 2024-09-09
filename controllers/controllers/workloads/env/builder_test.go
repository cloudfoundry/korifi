package env_test

import (
	"context"
	"slices"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("EnvBuilder", func() {
	var (
		vcapServicesSecret    *corev1.Secret
		vcapApplicationSecret *corev1.Secret
		appSecret             *corev1.Secret

		buildErr error

		envVars            []corev1.EnvVar
		appSecretEnv       types.GomegaMatcher
		vcapServicesEnv    types.GomegaMatcher
		vcapApplicationEnv types.GomegaMatcher
	)

	BeforeEach(func() {
		appSecretEnv = MatchFields(IgnoreExtras, Fields{
			"Name": Equal("app-secret"),
			"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
				"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfApp.Spec.EnvSecretName,
					},
					Key: "app-secret",
				})),
			})),
		})
		vcapServicesEnv = MatchFields(IgnoreExtras, Fields{
			"Name": Equal("VCAP_SERVICES"),
			"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
				"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfApp.Status.VCAPServicesSecretName,
					},
					Key: "VCAP_SERVICES",
				})),
			})),
		})

		vcapApplicationEnv = MatchFields(IgnoreExtras, Fields{
			"Name": Equal("VCAP_APPLICATION"),
			"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
				"SecretKeyRef": PointTo(Equal(corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfApp.Status.VCAPApplicationSecretName,
					},
					Key: "VCAP_APPLICATION",
				})),
			})),
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
		helpers.EnsureCreate(controllersClient, appSecret)

		vcapServicesSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-guid-vcap-services",
				Namespace: cfSpace.Status.GUID,
			},
			Data: map[string][]byte{"VCAP_SERVICES": []byte("{}")},
		}
		helpers.EnsureCreate(controllersClient, vcapServicesSecret)

		vcapApplicationSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-guid-vcap-application",
				Namespace: cfSpace.Status.GUID,
			},
			Data: map[string][]byte{"VCAP_APPLICATION": []byte(`{"foo":"bar"}`)},
		}
		helpers.EnsureCreate(controllersClient, vcapApplicationSecret)
	})

	Describe("AppEnvBuilder", func() {
		var builder *env.AppEnvBuilder

		BeforeEach(func() {
			builder = env.NewAppEnvBuilder(controllersClient)
		})

		JustBeforeEach(func() {
			envVars, buildErr = builder.Build(context.Background(), cfApp)
		})

		It("builds the user defined and VCAP_* env vars", func() {
			Expect(buildErr).NotTo(HaveOccurred())
			Expect(envVars).To(ConsistOf(
				appSecretEnv,
				vcapServicesEnv,
				vcapApplicationEnv,
			))
		})

		It("sorts the env vars by name", func() {
			Expect(buildErr).NotTo(HaveOccurred())
			envVarNames := []string{}
			for _, v := range envVars {
				envVarNames = append(envVarNames, v.Name)
			}

			Expect(slices.IsSorted(envVarNames)).To(BeTrue())
		})

		When("the app env secret does not exist", func() {
			BeforeEach(func() {
				helpers.EnsureDelete(controllersClient, appSecret)
			})

			It("errors", func() {
				Expect(buildErr).To(MatchError(ContainSubstring("error when trying to fetch app env Secret")))
			})
		})

		When("the app env secret is empty", func() {
			BeforeEach(func() {
				helpers.EnsurePatch(controllersClient, appSecret, func(s *corev1.Secret) {
					s.Data = map[string][]byte{}
				})
			})

			It("omits the app env", func() {
				Expect(envVars).To(ConsistOf(
					vcapServicesEnv,
					vcapApplicationEnv,
				))
			})
		})

		When("the app does not have an associated app env secret", func() {
			BeforeEach(func() {
				helpers.EnsurePatch(controllersClient, cfApp, func(a *korifiv1alpha1.CFApp) {
					a.Spec.EnvSecretName = ""
				})
			})

			It("succeeds", func() {
				Expect(buildErr).NotTo(HaveOccurred())
			})

			It("omits the app env", func() {
				Expect(buildErr).NotTo(HaveOccurred())
				Expect(envVars).To(ConsistOf(
					vcapServicesEnv,
					vcapApplicationEnv,
				))
			})
		})

		When("the app vcap services secret does not exist", func() {
			BeforeEach(func() {
				helpers.EnsureDelete(controllersClient, vcapServicesSecret)
			})

			It("errors", func() {
				Expect(buildErr).To(MatchError(ContainSubstring("error when trying to fetch vcap services secret")))
			})
		})

		When("the app vcap services secret is empty", func() {
			BeforeEach(func() {
				helpers.EnsurePatch(controllersClient, vcapServicesSecret, func(s *corev1.Secret) {
					s.Data = map[string][]byte{}
				})
			})

			It("omits the vcap services env", func() {
				Expect(envVars).To(ConsistOf(
					appSecretEnv,
					vcapApplicationEnv,
				))
			})
		})

		When("the app does not have an associated app vcap services secret", func() {
			BeforeEach(func() {
				helpers.EnsurePatch(controllersClient, cfApp, func(a *korifiv1alpha1.CFApp) {
					a.Status.VCAPServicesSecretName = ""
				})
			})

			It("succeeds", func() {
				Expect(buildErr).NotTo(HaveOccurred())
			})

			It("omits the vcap services env", func() {
				Expect(buildErr).NotTo(HaveOccurred())
				Expect(envVars).To(ConsistOf(
					appSecretEnv,
					vcapApplicationEnv,
				))
			})
		})

		When("the app vcap application secret does not exist", func() {
			BeforeEach(func() {
				helpers.EnsureDelete(controllersClient, vcapApplicationSecret)
			})

			It("errors", func() {
				Expect(buildErr).To(MatchError(ContainSubstring("error when trying to fetch vcap application secret")))
			})
		})

		When("the app vcap application secret is empty", func() {
			BeforeEach(func() {
				helpers.EnsurePatch(controllersClient, vcapApplicationSecret, func(secret *corev1.Secret) {
					secret.Data = nil
				})
			})

			It("omits the vcap application env", func() {
				Expect(envVars).To(ConsistOf(
					appSecretEnv,
					vcapServicesEnv,
				))
			})
		})

		When("the app does not have an associated app vcap application secret", func() {
			BeforeEach(func() {
				helpers.EnsurePatch(controllersClient, cfApp, func(a *korifiv1alpha1.CFApp) {
					a.Status.VCAPApplicationSecretName = ""
				})
			})

			It("omits the vcap application env", func() {
				Expect(buildErr).NotTo(HaveOccurred())
				Expect(envVars).To(ConsistOf(
					appSecretEnv,
					vcapServicesEnv,
				))
			})
		})
	})

	Describe("ProcessEnvBuilder", func() {
		var (
			builder   *env.ProcessEnvBuilder
			cfProcess *korifiv1alpha1.CFProcess
		)

		BeforeEach(func() {
			cfProcess = &korifiv1alpha1.CFProcess{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cfSpace.Status.GUID,
					Name:      "process-guid",
				},
				Spec: korifiv1alpha1.CFProcessSpec{
					MemoryMB:    789,
					ProcessType: "web",
				},
			}
			helpers.EnsureCreate(controllersClient, cfProcess)
			builder = env.NewProcessEnvBuilder(controllersClient)
		})

		JustBeforeEach(func() {
			envVars, buildErr = builder.Build(context.Background(), cfApp, cfProcess)
		})

		It("returns the process env vars", func() {
			Expect(buildErr).NotTo(HaveOccurred())
			Expect(envVars).To(ConsistOf(
				appSecretEnv,
				vcapServicesEnv,
				vcapApplicationEnv,
				MatchFields(IgnoreExtras, Fields{
					"Name":  Equal("VCAP_APP_HOST"),
					"Value": Equal("0.0.0.0"),
				}),
				MatchFields(IgnoreExtras, Fields{
					"Name":  Equal("MEMORY_LIMIT"),
					"Value": Equal("789M"),
				}),
			))
		})

		It("sorts the env vars by name", func() {
			Expect(buildErr).NotTo(HaveOccurred())
			envVarNames := []string{}
			for _, v := range envVars {
				envVarNames = append(envVarNames, v.Name)
			}

			Expect(slices.IsSorted(envVarNames)).To(BeTrue())
		})

		Describe("ports env vars", func() {
			var cfRoute *korifiv1alpha1.CFRoute

			BeforeEach(func() {
				destinations := []korifiv1alpha1.Destination{{
					GUID: "dest-guid",
					Port: tools.PtrTo[int32](1234),
					AppRef: corev1.LocalObjectReference{
						Name: cfApp.Name,
					},
					ProcessType: "web",
				}}

				cfRoute = &korifiv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: cfSpace.Status.GUID,
						Name:      "cf-route-guid",
					},
					Spec: korifiv1alpha1.CFRouteSpec{
						Destinations: destinations,
					},
				}
				helpers.EnsureCreate(controllersClient, cfRoute)

				helpers.EnsurePatch(controllersClient, cfRoute, func(cfRoute *korifiv1alpha1.CFRoute) {
					cfRoute.Status.Destinations = destinations
				})
			})

			It("builds the port env vars", func() {
				Expect(buildErr).NotTo(HaveOccurred())
				Expect(envVars).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"Name":  Equal("VCAP_APP_PORT"),
						"Value": Equal("1234"),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":  Equal("PORT"),
						"Value": Equal("1234"),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":  Equal("CF_INSTANCE_PORTS"),
						"Value": MatchJSON("[{\"internal\":1234}]"),
					}),
				))
			})
			When("the route does not have destinations", func() {
				BeforeEach(func() {
					helpers.EnsurePatch(controllersClient, cfRoute, func(cfRoute *korifiv1alpha1.CFRoute) {
						cfRoute.Status.Destinations = []korifiv1alpha1.Destination{}
					})
				})

				It("does not set port env vars", func() {
					Expect(envVars).NotTo(ContainElements(
						MatchFields(IgnoreExtras, Fields{"Name": Equal("VCAP_APP_PORT")}),
						MatchFields(IgnoreExtras, Fields{"Name": Equal("PORT")}),
					))
				})
			})
		})
	})
})
