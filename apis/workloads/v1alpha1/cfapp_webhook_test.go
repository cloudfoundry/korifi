package v1alpha1_test

import (
	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFBuildAppWebhook Unit Tests", func() {
	const (
		cfAppGUID     = "test-app-guid"
		cfAppLabelKey = "workloads.cloudfoundry.org/app-guid"
		namespace     = "default"
	)

	When("there are no existing labels on the CFAPP record", func() {
		It("should add a new label matching metadata.name", func() {
			cfApp := &v1alpha1.CFApp{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFApp",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespace,
				},
				Spec: v1alpha1.CFAppSpec{
					Name:         "test-app",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}

			cfApp.Default()
			Expect(cfApp.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppLabelKey, cfAppGUID))
		})
	})

	When("there are other existing labels on the CFAPP record", func() {
		It("should add a new label matching metadata.name and preserve the other labels", func() {
			cfApp := &v1alpha1.CFApp{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFApp",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespace,
					Labels: map[string]string{
						"anotherLabel": "app-label",
					},
				},
				Spec: v1alpha1.CFAppSpec{
					Name:         "test-app",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}

			cfApp.Default()
			Expect(cfApp.ObjectMeta.Labels).To(HaveLen(2))
			Expect(cfApp.ObjectMeta.Labels).To(HaveKeyWithValue("anotherLabel", "app-label"))
		})
	})
})
