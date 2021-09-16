package v1alpha1_test

import (
	v1 "k8s.io/api/core/v1"
	"testing"

	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPackageWebhook(t *testing.T) {
	spec.Run(t, "CFPackage Webhook", testCFPackageWebhook, spec.Report(report.Terminal{}))

}

func testCFPackageWebhook(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		cfAppGUID     = "test-app-guid"
		cfPackageGUID = "test-package-guid"
		namespace     = "default"
		cfPackageType = "bits"
		cfAppLabelKey = "workloads.cloudfoundry.org/app-guid"
	)

	when("there are no existing labels on the CFPackage record", func() {
		it("should add a new label matching spec.AppRef.name", func() {
			cfPackage := &v1alpha1.CFPackage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFPackage",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfPackageGUID,
					Namespace: namespace,
				},
				Spec: v1alpha1.CFPackageSpec{
					Type: cfPackageType,
					AppRef: v1.LocalObjectReference{
						Name: cfAppGUID,
					},
				},
			}

			cfPackage.Default()
			g.Expect(cfPackage.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppLabelKey, cfAppGUID))
		})
	})

	when("there are other existing labels on the CFPackage record", func() {
		it("should add a new label matching spec.AppRef.name and preserve the other labels", func() {
			cfPackage := &v1alpha1.CFPackage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFPackage",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfPackageGUID,
					Namespace: namespace,
					Labels: map[string]string{
						"anotherLabel": "app-label",
					},
				},
				Spec: v1alpha1.CFPackageSpec{
					Type: cfPackageType,
					AppRef: v1.LocalObjectReference{
						Name: cfAppGUID,
					},
				},
			}

			cfPackage.Default()
			g.Expect(cfPackage.ObjectMeta.Labels).To(HaveKeyWithValue("anotherLabel", "app-label"))
			g.Expect(cfPackage.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppLabelKey, cfAppGUID))
		})
	})
}
