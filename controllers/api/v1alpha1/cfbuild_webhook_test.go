package v1alpha1_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFBuildMutatingWebhook Unit Tests", func() {
	const (
		cfBuildGUID           = "test-build-guid"
		cfAppGUID             = "test-app-guid"
		cfPackageGUID         = "test-package-guid"
		namespace             = "default"
		cfAppGUIDLabelKey     = "korifi.cloudfoundry.org/app-guid"
		cfPackageGUIDLabelKey = "korifi.cloudfoundry.org/package-guid"
	)

	When("there are no existing labels on the CFBuild record", func() {
		var cfBuild korifiv1alpha1.CFBuild

		BeforeEach(func() {
			cfBuild = korifiv1alpha1.CFBuild{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFBuild",
					APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfBuildGUID,
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFBuildSpec{
					PackageRef: v1.LocalObjectReference{Name: cfPackageGUID},
					AppRef:     v1.LocalObjectReference{Name: cfAppGUID},
					Lifecycle:  korifiv1alpha1.Lifecycle{},
				},
			}
			cfBuild.Default()
		})
		It("should have an app label matching spec.AppRef.name", func() {
			Expect(cfBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})
		It("should have an package label matching spec.PackageRef.name", func() {
			Expect(cfBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfPackageGUIDLabelKey, cfPackageGUID))
		})
	})

	When("there are other existing labels on the CFBuild record", func() {
		var cfBuild korifiv1alpha1.CFBuild

		BeforeEach(func() {
			cfBuild = korifiv1alpha1.CFBuild{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFBuild",
					APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfBuildGUID,
					Namespace: namespace,
					Labels: map[string]string{
						"anotherLabel": "my-package-label",
					},
				},
				Spec: korifiv1alpha1.CFBuildSpec{
					PackageRef: v1.LocalObjectReference{Name: cfPackageGUID},
					AppRef:     v1.LocalObjectReference{Name: cfAppGUID},
					Lifecycle:  korifiv1alpha1.Lifecycle{},
				},
			}
			cfBuild.Default()
		})
		It("should preserve the existing labels", func() {
			Expect(cfBuild.ObjectMeta.Labels).To(HaveKeyWithValue("anotherLabel", "my-package-label"))
		})
		It("should have an app label matching spec.AppRef.name", func() {
			Expect(cfBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})
		It("should have an package label matching spec.PackageRef.name", func() {
			Expect(cfBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfPackageGUIDLabelKey, cfPackageGUID))
		})
	})
})
