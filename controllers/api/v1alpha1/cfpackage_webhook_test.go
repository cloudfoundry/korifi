package v1alpha1_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFPackageMutatingWebhook Unit Tests", func() {
	const (
		cfAppGUID         = "test-app-guid"
		cfAppGUIDLabelKey = "korifi.cloudfoundry.org/app-guid"
		cfPackageGUID     = "test-package-guid"
		cfPackageType     = "bits"
		namespace         = "default"
	)

	When("there are no existing labels on the CFPackage record", func() {
		It("should add a new label matching spec.AppRef.name", func() {
			cfPackage := &korifiv1alpha1.CFPackage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFPackage",
					APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfPackageGUID,
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFPackageSpec{
					Type: cfPackageType,
					AppRef: v1.LocalObjectReference{
						Name: cfAppGUID,
					},
				},
			}

			cfPackage.Default()
			Expect(cfPackage.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})
	})

	When("there are other existing labels on the CFPackage record", func() {
		It("should add a new label matching spec.AppRef.name and preserve the other labels", func() {
			cfPackage := &korifiv1alpha1.CFPackage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFPackage",
					APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfPackageGUID,
					Namespace: namespace,
					Labels: map[string]string{
						"anotherLabel": "app-label",
					},
				},
				Spec: korifiv1alpha1.CFPackageSpec{
					Type: cfPackageType,
					AppRef: v1.LocalObjectReference{
						Name: cfAppGUID,
					},
				},
			}

			cfPackage.Default()
			Expect(cfPackage.ObjectMeta.Labels).To(HaveLen(2))
			Expect(cfPackage.ObjectMeta.Labels).To(HaveKeyWithValue("anotherLabel", "app-label"))
		})
	})
})
