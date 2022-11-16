package v1alpha1_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFPackageMutatingWebhook", func() {
	var (
		cfAppGUID string
		cfPackage *korifiv1alpha1.CFPackage
	)

	BeforeEach(func() {
		cfAppGUID = GenerateGUID()

		cfPackage = &korifiv1alpha1.CFPackage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateGUID(),
				Namespace: namespace,
				Labels:    map[string]string{"foo": "bar"},
			},
			Spec: korifiv1alpha1.CFPackageSpec{
				Type: "bits",
				AppRef: v1.LocalObjectReference{
					Name: cfAppGUID,
				},
			},
		}
	})

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())
	})

	It("sets a label with the app guid", func() {
		Expect(cfPackage.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
	})

	It("preserves other labels", func() {
		Expect(cfPackage.Labels).To(HaveKeyWithValue("foo", "bar"))
	})
})
