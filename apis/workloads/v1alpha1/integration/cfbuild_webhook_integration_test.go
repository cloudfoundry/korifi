package integration_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CFBuildMutatingWebhook Integration Tests", func() {
	When("a CFBuild record is created", func() {
		const (
			namespace             = "default"
			cfAppGUIDLabelKey     = "workloads.cloudfoundry.org/app-guid"
			cfPackageGUIDLabelKey = "workloads.cloudfoundry.org/package-guid"
			lifeCycleType         = "buildpack"
		)

		var (
			cfBuild        *v1alpha1.CFBuild
			createdCFBuild *v1alpha1.CFBuild
			cfAppGUID      string
			cfPackageGUID  string
			cfBuildGUID    string
		)

		BeforeEach(func() {
			beforeCtx := context.Background()
			cfAppGUID = GenerateGUID()
			cfPackageGUID = GenerateGUID()
			cfBuildGUID = GenerateGUID()
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
			Expect(k8sClient.Create(beforeCtx, cfBuild)).To(Succeed())

			//Fetching the created CFBuild resource
			cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespace}
			createdCFBuild = new(v1alpha1.CFBuild)
			Eventually(func() map[string]string {
				err := k8sClient.Get(beforeCtx, cfBuildLookupKey, createdCFBuild)
				if err != nil {
					return nil
				}
				return createdCFBuild.ObjectMeta.Labels
			}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty())
		})

		AfterEach(func() {
			afterCtx := context.Background()
			//Cleaning up the created CFBuild resource
			k8sClient.Delete(afterCtx, cfBuild)
		})

		It("should have metadata labels for related resources", func() {
			Expect(createdCFBuild.ObjectMeta.Labels).ShouldNot(BeEmpty())
			Expect(createdCFBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
			Expect(createdCFBuild.ObjectMeta.Labels).To(HaveKeyWithValue(cfPackageGUIDLabelKey, cfPackageGUID))
		})
	})
})
