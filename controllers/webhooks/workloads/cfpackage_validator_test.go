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
		Expect(adminClient.Create(context.Background(), cfApp)).To(Succeed())

		cfPackage = &korifiv1alpha1.CFPackage{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfApp.Namespace,
				Name:      testutils.PrefixedGUID("cfpackage"),
			},
			Spec: korifiv1alpha1.CFPackageSpec{
				AppRef: v1.LocalObjectReference{
					Name: cfApp.Name,
				},
				Type: "bits",
			},
		}
		Expect(adminClient.Create(context.Background(), cfPackage)).To(Succeed())
	})

	Describe("package type immutability", func() {
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
					cfPackage.Finalizers = append(cfPackage.Finalizers, "some-finalizer")
				})).To(Succeed())
				Expect(adminNonSyncClient.Delete(context.Background(), cfPackage)).To(Succeed())
			})

			It("allows it", func() {
				Expect(updateErr).NotTo(HaveOccurred())
			})
		})
	})
})
