package env_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Builder", func() {
	var (
		vcapServicesSecret    *corev1.Secret
		vcapApplicationSecret *corev1.Secret
		appSecret             *corev1.Secret

		builder *env.WorkloadEnvBuilder

		envVars     []corev1.EnvVar
		buildEnvErr error
	)

	BeforeEach(func() {
		builder = env.NewWorkloadEnvBuilder(k8sClient)

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

		vcapServicesSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-guid-vcap-services",
				Namespace: cfSpace.Status.GUID,
			},
			Data: map[string][]byte{"VCAP_SERVICES": []byte("{}")},
		}
		ensureCreate(vcapServicesSecret)

		vcapApplicationSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-guid-vcap-application",
				Namespace: cfSpace.Status.GUID,
			},
			Data: map[string][]byte{"VCAP_APPLICATION": []byte(`{"foo":"bar"}`)},
		}
		ensureCreate(vcapApplicationSecret)
	})

	Describe("BuildEnv", func() {
		var (
			appSecretEnv       types.GomegaMatcher
			vcapServicesEnv    types.GomegaMatcher
			vcapApplicationEnv types.GomegaMatcher
		)

		JustBeforeEach(func() {
			envVars, buildEnvErr = builder.BuildEnv(context.Background(), cfApp)
		})

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
		})

		It("returns the user defined and VCAP_* env vars", func() {
			Expect(buildEnvErr).NotTo(HaveOccurred())
			Expect(envVars).To(ConsistOf(
				appSecretEnv,
				vcapServicesEnv,
				vcapApplicationEnv,
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

			It("omits the app env", func() {
				Expect(envVars).To(ConsistOf(
					vcapServicesEnv,
					vcapApplicationEnv,
				))
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

			It("omits the app env", func() {
				Expect(buildEnvErr).NotTo(HaveOccurred())
				Expect(envVars).To(ConsistOf(
					vcapServicesEnv,
					vcapApplicationEnv,
				))
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

			It("omits the vcap services env", func() {
				Expect(envVars).To(ConsistOf(
					appSecretEnv,
					vcapApplicationEnv,
				))
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

			It("omits the vcap services env", func() {
				Expect(buildEnvErr).NotTo(HaveOccurred())
				Expect(envVars).To(ConsistOf(
					appSecretEnv,
					vcapApplicationEnv,
				))
			})
		})

		When("the app vcap application secret does not exist", func() {
			BeforeEach(func() {
				ensureDelete(vcapApplicationSecret)
			})

			It("errors", func() {
				Expect(buildEnvErr).To(MatchError(ContainSubstring("error when trying to fetch vcap application secret")))
			})
		})

		When("the app vcap application secret is empty", func() {
			BeforeEach(func() {
				ensurePatch(vcapApplicationSecret, func(secret *corev1.Secret) {
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
				ensurePatch(cfApp, func(a *korifiv1alpha1.CFApp) {
					a.Status.VCAPApplicationSecretName = ""
				})
			})

			It("omits the vcap application env", func() {
				Expect(buildEnvErr).NotTo(HaveOccurred())
				Expect(envVars).To(ConsistOf(
					appSecretEnv,
					vcapServicesEnv,
				))
			})
		})
	})
})
