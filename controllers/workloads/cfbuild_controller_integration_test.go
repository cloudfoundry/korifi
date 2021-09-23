package workloads_test

import (
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/testutils"
	"context"
	buildv1alpha1 "github.com/pivotal/kpack/pkg/apis/build/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = AddToTestSuite("CFBuildReconciler", testCFBuildReconcilerIntegration)

func testCFBuildReconcilerIntegration(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	when("CFBuild status conditions are missing or unknown", func() {
		const (
			defaultNamespace = "default"
		)
		var (
			cfAppGUID        string
			cfPackageGUID    string
			cfBuildGUID      string
			desiredCFApp     *workloadsv1alpha1.CFApp
			desiredCFPackage *workloadsv1alpha1.CFPackage
			desiredCFBuild   *workloadsv1alpha1.CFBuild
		)
		it.Before(func() {
			cfAppGUID = GenerateGUID()
			cfPackageGUID = GenerateGUID()
			cfBuildGUID = GenerateGUID()

			beforeCtx := context.Background()

			desiredCFApp = InitializeAppCR(cfAppGUID, defaultNamespace)
			g.Expect(k8sClient.Create(beforeCtx, desiredCFApp)).To(Succeed())

			desiredCFPackage = InitializePackageCR(cfPackageGUID, defaultNamespace, cfAppGUID)
			g.Expect(k8sClient.Create(beforeCtx, desiredCFPackage)).To(Succeed())

			desiredCFBuild = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfBuildGUID,
					Namespace: defaultNamespace,
				},
				Spec: workloadsv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: cfPackageGUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: cfAppGUID,
					},
					StagingMemoryMB: 1024,
					StagingDiskMB:   1024,
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: nil,
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(beforeCtx, desiredCFBuild)).To(Succeed())
		})
		it.After(func() {
			afterCtx := context.Background()
			g.Expect(k8sClient.Delete(afterCtx, desiredCFApp)).To(Succeed())
			g.Expect(k8sClient.Delete(afterCtx, desiredCFPackage)).To(Succeed())
			g.Expect(k8sClient.Delete(afterCtx, desiredCFBuild)).To(Succeed())
		})

		when("on the happy path", func() {
			when("kpack image with CFBuild GUID doesn't exist", func() {
				it("should eventually create a Kpack Image", func() {
					testCtx := context.Background()
					kpackImageLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: defaultNamespace}
					createdKpackImage := new(buildv1alpha1.Image)
					g.Eventually(func() bool {
						err := k8sClient.Get(testCtx, kpackImageLookupKey, createdKpackImage)
						return err == nil
					}, 10*time.Second, 250*time.Millisecond).Should(BeTrue(), "could not retrieve the kpack image")
					kpackImageTag := "image/registry/tag/" + defaultNamespace + "/" + cfBuildGUID
					g.Expect(createdKpackImage.Spec.Tag).To(Equal(kpackImageTag))
					g.Expect(k8sClient.Delete(testCtx, createdKpackImage)).To(Succeed())
				})
				it("eventually sets the status conditions on CFBuild", func() {
					testCtx := context.Background()
					cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: defaultNamespace}
					createdCFBuild := new(workloadsv1alpha1.CFBuild)
					g.Eventually(func() []metav1.Condition {
						err := k8sClient.Get(testCtx, cfBuildLookupKey, createdCFBuild)
						if err != nil {
							return nil
						}
						return createdCFBuild.Status.Conditions
					}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "CFBuild status conditions were empty")
				})
			})
			when("kpack image with CFBuild GUID already exists", func() {
				var (
					newCFBuildGUID     string
					existingKpackImage *buildv1alpha1.Image
					newCFBuild         *workloadsv1alpha1.CFBuild
				)
				it.Before(func() {
					beforeCtx := context.Background()
					newCFBuildGUID = GenerateGUID()
					existingKpackImage = &buildv1alpha1.Image{
						ObjectMeta: metav1.ObjectMeta{
							Name:      newCFBuildGUID,
							Namespace: defaultNamespace,
						},
						Spec: buildv1alpha1.ImageSpec{
							Tag: "my-tag-string",
							Builder: corev1.ObjectReference{
								Name: "my-builder",
							},
							ServiceAccount: "my-service-account",
							Source: buildv1alpha1.SourceConfig{
								Registry: &buildv1alpha1.Registry{
									Image:            "not-an-image",
									ImagePullSecrets: nil,
								},
							},
						},
					}
					g.Expect(k8sClient.Create(beforeCtx, existingKpackImage)).To(Succeed())
					newCFBuild = InitializeCFBuild(newCFBuildGUID, defaultNamespace, cfPackageGUID, cfAppGUID)
					g.Expect(k8sClient.Create(beforeCtx, newCFBuild)).To(Succeed())
				})
				it.After(func() {
					afterCtx := context.Background()
					g.Expect(k8sClient.Delete(afterCtx, existingKpackImage)).To(Succeed())
					g.Expect(k8sClient.Delete(afterCtx, newCFBuild)).To(Succeed())
				})
				it("eventually sets the status conditions on CFBuild", func() {
					testCtx := context.Background()
					cfBuildLookupKey := types.NamespacedName{Name: newCFBuildGUID, Namespace: defaultNamespace}
					createdCFBuild := new(workloadsv1alpha1.CFBuild)
					g.Eventually(func() []metav1.Condition {
						err := k8sClient.Get(testCtx, cfBuildLookupKey, createdCFBuild)
						if err != nil {
							return nil
						}
						return createdCFBuild.Status.Conditions
					}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "CFBuild status conditions were empty")
				})
			})
		})
	})

}
