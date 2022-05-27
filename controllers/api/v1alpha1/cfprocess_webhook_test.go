package v1alpha1_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFProcessMutatingWebhook Unit Tests", func() {
	const (
		cfAppGUID             = "test-app-guid"
		cfAppGUIDLabelKey     = "korifi.cloudfoundry.org/app-guid"
		cfProcessGUID         = "test-process-guid"
		cfProcessGUIDLabelKey = "korifi.cloudfoundry.org/process-guid"
		cfProcessType         = "test-process-type"
		cfProcessTypeLabelKey = "korifi.cloudfoundry.org/process-type"
		namespace             = "default"
	)

	var cfProcess *korifiv1alpha1.CFProcess

	When("there are no existing labels on the CFProcess record", func() {
		BeforeEach(func() {
			cfProcess = &korifiv1alpha1.CFProcess{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFProcess",
					APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfProcessGUID,
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFProcessSpec{
					AppRef: v1.LocalObjectReference{
						Name: cfAppGUID,
					},
					ProcessType: cfProcessType,
				},
			}
		})

		It("should add the appropriate labels", func() {
			cfProcess.Default()

			Expect(cfProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfProcessGUIDLabelKey, cfProcessGUID))
			Expect(cfProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfProcessTypeLabelKey, cfProcessType))
			Expect(cfProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})
	})

	When("there are other existing labels on the CFProcess record", func() {
		BeforeEach(func() {
			cfProcess = &korifiv1alpha1.CFProcess{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFProcess",
					APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfProcessGUID,
					Namespace: namespace,
					Labels: map[string]string{
						"anotherLabel": "process-label",
					},
				},
				Spec: korifiv1alpha1.CFProcessSpec{},
			}
		})

		It("should preserve the other labels", func() {
			cfProcess.Default()

			Expect(cfProcess.ObjectMeta.Labels).To(HaveLen(4), "CFProcess resource should have 4 labels")
			Expect(cfProcess.ObjectMeta.Labels).To(HaveKeyWithValue("anotherLabel", "process-label"))
		})
	})
})
