package workloads_test

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/testutils"
	buildv1alpha1 "github.com/pivotal/kpack/pkg/apis/build/v1alpha1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = AddToTestSuite("CFBuildReconciler", testCFBuildReconcilerIntegration)

func testCFBuildReconcilerIntegration(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)
	const (
		succeededConditionType  = "Succeeded"
		kpackReadyConditionType = "Ready"
	)
	when("CFBuild status conditions are missing or unknown", func() {
		var (
			namespaceGUID    string
			cfAppGUID        string
			cfPackageGUID    string
			cfBuildGUID      string
			newNamespace     *corev1.Namespace
			desiredCFApp     *workloadsv1alpha1.CFApp
			desiredCFPackage *workloadsv1alpha1.CFPackage
			desiredCFBuild   *workloadsv1alpha1.CFBuild
		)
		it.Before(func() {
			namespaceGUID = GenerateGUID()
			cfAppGUID = GenerateGUID()
			cfPackageGUID = GenerateGUID()
			cfBuildGUID = GenerateGUID()

			beforeCtx := context.Background()

			newNamespace = MockK8sNamespaceObject(namespaceGUID)
			g.Expect(k8sClient.Create(beforeCtx, newNamespace)).To(Succeed())

			desiredCFApp = MockAppCRObject(cfAppGUID, namespaceGUID)
			g.Expect(k8sClient.Create(beforeCtx, desiredCFApp)).To(Succeed())

			desiredCFPackage = MockPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			g.Expect(k8sClient.Create(beforeCtx, desiredCFPackage)).To(Succeed())

			desiredCFBuild = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfBuildGUID,
					Namespace: namespaceGUID,
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
			g.Expect(k8sClient.Delete(afterCtx, newNamespace)).To(Succeed())
		})

		when("on the happy path", func() {
			when("kpack image with CFBuild GUID doesn't exist", func() {
				it("should eventually create a Kpack Image", func() {
					testCtx := context.Background()
					kpackImageLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
					createdKpackImage := new(buildv1alpha1.Image)
					g.Eventually(func() bool {
						err := k8sClient.Get(testCtx, kpackImageLookupKey, createdKpackImage)
						return err == nil
					}, 10*time.Second, 250*time.Millisecond).Should(BeTrue(), "could not retrieve the kpack image")
					kpackImageTag := "image/registry/tag" + "/" + cfBuildGUID
					g.Expect(createdKpackImage.Spec.Tag).To(Equal(kpackImageTag))
					g.Expect(k8sClient.Delete(testCtx, createdKpackImage)).To(Succeed())
				})
				it("eventually sets the status conditions on CFBuild", func() {
					testCtx := context.Background()
					cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
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
							Namespace: namespaceGUID,
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
					newCFBuild = MockCFBuildObject(newCFBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
					g.Expect(k8sClient.Create(beforeCtx, newCFBuild)).To(Succeed())
				})
				it.After(func() {
					afterCtx := context.Background()
					g.Expect(k8sClient.Delete(afterCtx, existingKpackImage)).To(Succeed())
					g.Expect(k8sClient.Delete(afterCtx, newCFBuild)).To(Succeed())
				})
				it("eventually sets the status conditions on CFBuild", func() {
					testCtx := context.Background()
					cfBuildLookupKey := types.NamespacedName{Name: newCFBuildGUID, Namespace: namespaceGUID}
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
	when("CFBuild status conditions for Staging is True and others are unknown", func() {
		var (
			namespaceGUID    string
			cfAppGUID        string
			cfPackageGUID    string
			cfBuildGUID      string
			newNamespace     *corev1.Namespace
			desiredCFApp     *workloadsv1alpha1.CFApp
			desiredCFPackage *workloadsv1alpha1.CFPackage
			desiredCFBuild   *workloadsv1alpha1.CFBuild
		)
		it.Before(func() {
			namespaceGUID = GenerateGUID()
			cfAppGUID = GenerateGUID()
			cfPackageGUID = GenerateGUID()
			cfBuildGUID = GenerateGUID()

			beforeCtx := context.Background()

			newNamespace = MockK8sNamespaceObject(namespaceGUID)
			g.Expect(k8sClient.Create(beforeCtx, newNamespace)).To(Succeed())

			desiredCFApp = MockAppCRObject(cfAppGUID, namespaceGUID)
			g.Expect(k8sClient.Create(beforeCtx, desiredCFApp)).To(Succeed())

			desiredCFPackage = MockPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			g.Expect(k8sClient.Create(beforeCtx, desiredCFPackage)).To(Succeed())

			desiredCFBuild = MockCFBuildObject(cfBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
			g.Expect(k8sClient.Create(beforeCtx, desiredCFBuild)).To(Succeed())
		})
		it.After(func() {
			afterCtx := context.Background()
			g.Expect(k8sClient.Delete(afterCtx, desiredCFApp)).To(Succeed())
			g.Expect(k8sClient.Delete(afterCtx, desiredCFPackage)).To(Succeed())
			g.Expect(k8sClient.Delete(afterCtx, desiredCFBuild)).To(Succeed())
			g.Expect(k8sClient.Delete(afterCtx, newNamespace)).To(Succeed())
		})

		when("kpack image status condition for Type Succeeded is False", func() {
			it.Before(func() {
				testCtx := context.Background()
				kpackImageLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdKpackImage := new(buildv1alpha1.Image)
				g.Eventually(func() bool {
					err := k8sClient.Get(testCtx, kpackImageLookupKey, createdKpackImage)
					return err == nil
				}, 10*time.Second, 250*time.Millisecond).Should(BeTrue(), "could not retrieve the kpack image")
				setKpackImageStatus(createdKpackImage, kpackReadyConditionType, "False")
				g.Expect(k8sClient.Status().Update(testCtx, createdKpackImage)).To(Succeed())
			})
			it("should eventually set the status condition for Type Succeeded on CFBuild to False", func() {
				testCtx := context.Background()
				cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(workloadsv1alpha1.CFBuild)
				g.Eventually(func() bool {
					err := k8sClient.Get(testCtx, cfBuildLookupKey, createdCFBuild)
					if err != nil {
						return false
					}
					return meta.IsStatusConditionFalse(createdCFBuild.Status.Conditions, succeededConditionType)
				}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())
			})
		})
		when("kpack image status condition for Type Succeeded is True", func() {
			it.Before(func() {
				testCtx := context.Background()
				kpackImageLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdKpackImage := new(buildv1alpha1.Image)
				g.Eventually(func() bool {
					err := k8sClient.Get(testCtx, kpackImageLookupKey, createdKpackImage)
					return err == nil
				}, 10*time.Second, 250*time.Millisecond).Should(BeTrue(), "could not retrieve the kpack image")
				setKpackImageStatus(createdKpackImage, kpackReadyConditionType, "True")
				g.Expect(k8sClient.Status().Update(testCtx, createdKpackImage)).To(Succeed())
			})
			it("should eventually set the status condition for Type Succeeded on CFBuild to True", func() {
				testCtx := context.Background()
				cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(workloadsv1alpha1.CFBuild)
				g.Eventually(func() bool {
					err := k8sClient.Get(testCtx, cfBuildLookupKey, createdCFBuild)
					if err != nil {
						return false
					}
					return meta.IsStatusConditionTrue(createdCFBuild.Status.Conditions, succeededConditionType)
				}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())
			})
			it("should eventually set BuildStatusDroplet object", func() {
				testCtx := context.Background()
				cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(workloadsv1alpha1.CFBuild)
				g.Eventually(func() *workloadsv1alpha1.BuildDropletStatus {
					err := k8sClient.Get(testCtx, cfBuildLookupKey, createdCFBuild)
					if err != nil {
						return nil
					}
					return createdCFBuild.Status.BuildDropletStatus
				}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeNil(), "BuildStatusDroplet was nil on CFBuild")
			})
		})
	})

}
