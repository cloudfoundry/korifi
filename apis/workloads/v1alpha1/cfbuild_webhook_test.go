package v1alpha1_test

import (
	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestBuildWebhook(t *testing.T) {
	spec.Run(t, "CFBuild Webhook", testCFBuildWebhook, spec.Report(report.Terminal{}))

}

func testCFBuildWebhook(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		cfBuildGUID           = "test-build-guid"
		cfAppGUID             = "test-app-guid"
		cfPackageGUID         = "test-package-guid"
		namespace             = "default"
		cfAppGUIDLabelKey     = "workloads.cloudfoundry.org/app-guid"
		cfPackageGUIDLabelKey = "workloads.cloudfoundry.org/package-guid"
	)

	when("there are no existing labels on the CFBuild record", func() {
		var cfBuild v1alpha1.CFBuild

		it.Before(func() {
			cfBuild = v1alpha1.CFBuild{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFBuild",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfBuildGUID,
					Namespace: namespace,
				},
				Spec: v1alpha1.CFBuildSpec{
					PackageRef: v1.LocalObjectReference{Name: cfPackageGUID},
					AppRef:     v1.LocalObjectReference{Name: cfAppGUID},
					Lifecycle:  v1alpha1.Lifecycle{},
				},
			}
			cfBuild.Default()
		})
		it("should have an app label matching spec.AppRef.name", func() {
			g.Expect(cfBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})
		it("should have an package label matching spec.PackageRef.name", func() {
			g.Expect(cfBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfPackageGUIDLabelKey, cfPackageGUID))
		})
	})

	when("there are other existing labels on the CFBuild record", func() {
		var cfBuild v1alpha1.CFBuild

		it.Before(func() {
			cfBuild = v1alpha1.CFBuild{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFBuild",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfBuildGUID,
					Namespace: namespace,
					Labels: map[string]string{
						"anotherLabel": "my-package-label",
					},
				},
				Spec: v1alpha1.CFBuildSpec{
					PackageRef: v1.LocalObjectReference{Name: cfPackageGUID},
					AppRef:     v1.LocalObjectReference{Name: cfAppGUID},
					Lifecycle:  v1alpha1.Lifecycle{},
				},
			}
			cfBuild.Default()
		})
		it("should preserve the existing labels", func() {
			g.Expect(cfBuild.ObjectMeta.Labels).To(HaveKeyWithValue("anotherLabel", "my-package-label"))
		})
		it("should have an app label matching spec.AppRef.name", func() {
			g.Expect(cfBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})
		it("should have an package label matching spec.PackageRef.name", func() {
			g.Expect(cfBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfPackageGUIDLabelKey, cfPackageGUID))
		})
	})
}
