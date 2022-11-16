package v1alpha1_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	cfAppLabelKey    = "korifi.cloudfoundry.org/app-guid"
	cfAppRevisionKey = "korifi.cloudfoundry.org/app-rev"
)

var _ = Describe("CFAppMutatingWebhook", func() {
	var cfApp *korifiv1alpha1.CFApp

	BeforeEach(func() {
		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateGUID(),
				Namespace: namespace,
				Labels: map[string]string{
					"anotherLabel": "app-label",
				},
				Annotations: map[string]string{
					"someAnnotation": "blah",
				},
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  GenerateGUID(),
				DesiredState: "STARTED",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
	})

	It("adds a label matching metadata.name", func() {
		Expect(cfApp.Labels).To(HaveKeyWithValue(cfAppLabelKey, cfApp.Name))
	})

	It("adds an app revision annotation", func() {
		Expect(cfApp.Annotations).To(HaveKeyWithValue(cfAppRevisionKey, "0"))
	})

	It("preserves all other app labels and annotations", func() {
		Expect(cfApp.Labels).To(HaveKeyWithValue("anotherLabel", "app-label"))
		Expect(cfApp.Annotations).To(HaveKeyWithValue("someAnnotation", "blah"))
	})

	When("the app is being stopped", func() {
		JustBeforeEach(func() {
			Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
				cfApp.Spec.DesiredState = "STOPPED"
				cfApp.Status.ObservedDesiredState = "STARTED"
				// the values below are required
				cfApp.Status.Conditions = []metav1.Condition{}
				cfApp.Status.VCAPServicesSecretName = "foo"
			})).To(Succeed())
		})

		It("increments the app revision annotation", func() {
			Expect(cfApp.Annotations).To(HaveKeyWithValue(cfAppRevisionKey, "1"))
		})
	})
})
