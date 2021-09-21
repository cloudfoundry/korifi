package v1alpha1_test

import (
	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

var _ = AddToTestSuite("CFBuildWebhook", integrationTestCFBuildWebhook)

func integrationTestCFBuildWebhook(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)
	when("a CFBuild record is created", func() {
		var cfBuild *v1alpha1.CFBuild
		var createdCFBuild *v1alpha1.CFBuild
		const (
			cfAppGUID             = "test-app-guid"
			cfPackageGUID         = "test-package-guid"
			cfBuildGUID           = "test-build-guid"
			namespace             = "default"
			cfAppGUIDLabelKey     = "workloads.cloudfoundry.org/app-guid"
			cfPackageGUIDLabelKey = "workloads.cloudfoundry.org/package-guid"
			lifeCycleType         = "buildpack"
		)
		it.Before(func() {
			cfBuild = &v1alpha1.CFBuild{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFBuild",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfBuildGUID,
					Namespace: namespace,
				},
				Spec: v1alpha1.CFBuildSpec{
					PackageRef: v1.LocalObjectReference{
						Name: cfPackageGUID,
					},
					AppRef: v1.LocalObjectReference{
						Name: cfAppGUID,
					},
					Lifecycle: v1alpha1.Lifecycle{
						Type: lifeCycleType,
						Data: v1alpha1.LifecycleData{
							Buildpacks: []string{"java-buildpack"},
							Stack:      "cflinuxfs3",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfBuild)).To(Succeed())

			//Fetching the created CFBuild resource
			cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespace}
			createdCFBuild = new(v1alpha1.CFBuild)
			g.Eventually(func() map[string]string {
				err := k8sClient.Get(ctx, cfBuildLookupKey, createdCFBuild)
				if err != nil {
					return nil
				}
				return createdCFBuild.ObjectMeta.Labels
			}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty())
		})

		it.After(func() {
			//Cleaning up the created CFBuild resource
			g.Expect(k8sClient.Delete(ctx, cfBuild)).To(Succeed())
		})

		it("should have metadata labels for related resources", func() {
			g.Expect(createdCFBuild.ObjectMeta.Labels).ShouldNot(BeEmpty())
			g.Expect(createdCFBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
			g.Expect(createdCFBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfPackageGUIDLabelKey, cfPackageGUID))
		})
	})

}
