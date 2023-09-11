package workloads_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFPackage Validation", func() {
	var (
		cfApp     *korifiv1alpha1.CFApp
		cfPackage *korifiv1alpha1.CFPackage
	)

	BeforeEach(func() {
		cfApp = makeCFApp(testutils.PrefixedGUID("cfapp"), rootNamespace, testutils.PrefixedGUID("appName"))
		cfPackage = &korifiv1alpha1.CFPackage{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfApp.Namespace,
				Name:      testutils.PrefixedGUID("cfpackage"),
			},
			Spec: korifiv1alpha1.CFPackageSpec{
				AppRef: v1.LocalObjectReference{
					Name: cfApp.Name,
				},
			},
		}
	})

	Describe("create", func() {
		var creationErr error

		JustBeforeEach(func() {
			Expect(adminClient.Create(context.Background(), cfApp)).To(Succeed())
			creationErr = adminClient.Create(context.Background(), cfPackage)
		})

		When("the app does not exist", func() {
			BeforeEach(func() {
				cfPackage.Spec.Type = korifiv1alpha1.PackageType("bits")
				cfPackage.Spec.AppRef.Name = "not-existing"
			})

			It("returns a validation error", func() {
				Expect(creationErr).To(MatchError(ContainSubstring("does not exist")))
			})
		})

		Describe("buildpack apps", func() {
			BeforeEach(func() {
				cfApp.Spec.Lifecycle.Type = "buildpack"
				cfPackage.Spec.Type = korifiv1alpha1.PackageType("bits")
			})

			It("succeeds", func() {
				Expect(creationErr).NotTo(HaveOccurred())
			})

			When("the package type is not bits", func() {
				BeforeEach(func() {
					cfPackage.Spec.Type = korifiv1alpha1.PackageType("docker")
				})

				It("returns a validation error", func() {
					Expect(creationErr).To(MatchError(ContainSubstring("cannot create docker package for a buildpack app")))
				})
			})
		})

		Describe("docker apps", func() {
			BeforeEach(func() {
				cfApp.Spec.Lifecycle.Type = "docker"
				cfPackage.Spec.Type = korifiv1alpha1.PackageType("docker")
			})

			It("succeeds", func() {
				Expect(creationErr).NotTo(HaveOccurred())
			})

			When("the package type is not docker", func() {
				BeforeEach(func() {
					cfPackage.Spec.Type = korifiv1alpha1.PackageType("bits")
				})

				It("returns a validation error", func() {
					Expect(creationErr).To(MatchError(ContainSubstring("cannot create bits package for a docker app")))
				})
			})
		})
	})

	Describe("update", func() {
		BeforeEach(func() {
			Expect(adminClient.Create(context.Background(), cfApp)).To(Succeed())
			cfPackage.Spec.Type = korifiv1alpha1.PackageType("bits")
			Expect(adminClient.Create(context.Background(), cfPackage)).To(Succeed())
		})

		Describe("package type", func() {
			var updateErr error

			JustBeforeEach(func() {
				updateErr = k8s.Patch(context.Background(), adminClient, cfPackage, func() {
					cfPackage.Spec.Type = "docker"
				})
			})

			It("does not allow changing the package type", func() {
				Expect(updateErr).To(MatchError(ContainSubstring("immutable")))
			})

			When("the package is being deleted", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(context.Background(), adminClient, cfPackage, func() {
						cfPackage.Finalizers = append(cfPackage.Finalizers, "dummy")
					})).To(Succeed())
					Expect(adminClient.Delete(context.Background(), cfPackage)).To(Succeed())
				})

				It("allows it", func() {
					Expect(updateErr).NotTo(HaveOccurred())
				})
			})
		})
	})
})
