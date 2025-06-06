package v1alpha1_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFAppMutatingWebhook", func() {
	var cfApp *korifiv1alpha1.CFApp

	BeforeEach(func() {
		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace,
				Labels: map[string]string{
					"anotherLabel": "app-label",
				},
				Annotations: map[string]string{
					"someAnnotation": "blah",
				},
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  uuid.NewString(),
				DesiredState: "STARTED",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(ctx, cfApp)).To(Succeed())
	})

	It("adds an app revision annotation", func() {
		Expect(cfApp.Annotations).To(HaveKeyWithValue(korifiv1alpha1.CFAppRevisionKey, "0"))
	})

	It("preserves all other app labels and annotations", func() {
		Expect(cfApp.Labels).To(HaveKeyWithValue("anotherLabel", "app-label"))
		Expect(cfApp.Annotations).To(HaveKeyWithValue("someAnnotation", "blah"))
	})

	When("the app does not have any annotations", func() {
		BeforeEach(func() {
			cfApp.Annotations = nil
		})

		It("adds an app revision annotation with a default value", func() {
			Expect(cfApp.Annotations).To(HaveKeyWithValue(korifiv1alpha1.CFAppRevisionKey, "0"))
		})
	})
})
