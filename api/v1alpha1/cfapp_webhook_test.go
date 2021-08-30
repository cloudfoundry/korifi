package v1alpha1_test

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestAppWebhook(t *testing.T) {
	spec.Run(t, "CFApp Webhook", testCFAppWebhook, spec.Report(report.Terminal{}))

}

func testCFAppWebhook(t *testing.T, when spec.G, it spec.S) {
	Expect := NewWithT(t).Expect
	const (
		cfAppGUID     = "test-app-guid"
		namespace     = "default"
		cfAppLabelKey = "apps.cloudfoundry.org/appGuid"
	)
	when("there are no existing labels on the CFAPP record", func() {
		it("should add a new label matching metadata.name", func() {
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

	when("there are other existing labels on the CFAPP record", func() {
		it("should add a new label matching metadata.name and preserve the other labels", func() {
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
			Expect(cfApp.ObjectMeta.Labels).To(HaveKeyWithValue("anotherLabel", "app-label"))
		})
	})
}
