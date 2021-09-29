package repositories_test

import (
	"context"
	"k8s.io/apimachinery/pkg/api/meta"
	"testing"
	"time"

	. "code.cloudfoundry.org/cf-k8s-api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = SuiteDescribe("Build Repository FetchBuild", testFetchBuild)

func testFetchBuild(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		app1GUID      = "app-1-guid"
		app2GUID      = "app-2-guid"
		package1GUID  = "package-1-guid"
		package2GUID  = "package-2-guid"
		stagingMemory = 1024
		stagingDisk   = 2048
	)

	var (
		testCtx   context.Context
		buildRepo *BuildRepo
		client    client.Client

		namespace1 *corev1.Namespace
		namespace2 *corev1.Namespace
	)

	it.Before(func() {
		testCtx = context.Background()

		namespace1Name := generateGUID()
		namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
		g.Expect(k8sClient.Create(context.Background(), namespace1)).To(Succeed())

		namespace2Name := generateGUID()
		namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
		g.Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())

		buildRepo = new(BuildRepo)
		var err error
		client, err = BuildClient(k8sConfig)
		g.Expect(err).ToNot(HaveOccurred())
	})

	it.After(func() {
		g.Expect(k8sClient.Delete(context.Background(), namespace1)).To(Succeed())
		g.Expect(k8sClient.Delete(context.Background(), namespace2)).To(Succeed())
	})

	when("on the happy path", func() {

		const (
			StagingConditionType   = "Staging"
			ReadyConditionType     = "Ready"
			SucceededConditionType = "Succeeded"
		)

		var (
			build1GUID string
			build2GUID string
			build1     *workloadsv1alpha1.CFBuild
			build2     *workloadsv1alpha1.CFBuild
		)

		it.Before(func() {
			build1GUID = generateGUID()
			build2GUID = generateGUID()
			build1 = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      build1GUID,
					Namespace: namespace1.Name,
				},
				Spec: workloadsv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: package1GUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: app1GUID,
					},
					StagingMemoryMB: stagingMemory,
					StagingDiskMB:   stagingDisk,
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), build1)).To(Succeed())

			build2 = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      build2GUID,
					Namespace: namespace2.Name,
				},
				Spec: workloadsv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: package2GUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: app2GUID,
					},
					StagingMemoryMB: stagingMemory,
					StagingDiskMB:   stagingDisk,
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), build2)).To(Succeed())
		})

		it.After(func() {
			g.Expect(k8sClient.Delete(context.Background(), build1)).To(Succeed())
			g.Expect(k8sClient.Delete(context.Background(), build2)).To(Succeed())
		})

		when("fetching a build", func() {
			var (
				buildRecord *BuildRecord
				fetchError  error
			)
			when("no status.Conditions are set", func() {
				it.Before(func() {
					returnedBuildRecord, err := buildRepo.FetchBuild(testCtx, client, build2GUID)
					buildRecord = &returnedBuildRecord
					fetchError = err
				})

				it("succeeds", func() {
					g.Expect(fetchError).NotTo(HaveOccurred())
				})

				it("returns a record with a matching GUID", func() {
					g.Expect(buildRecord.GUID).To(Equal(build2GUID))
				})

				it("returns a record with state \"STAGING\" and no StagingErrorMsg", func() {
					g.Expect(buildRecord.State).To(Equal("STAGING"))
					g.Expect(buildRecord.StagingErrorMsg).To(BeEmpty(), "record staging error message was supposed to be empty")
				})

				it("returns a record with no DropletGUID", func() {
					g.Expect(buildRecord.DropletGUID).To(BeEmpty())
				})

				it("returns a record with a CreatedAt field from the CR", func() {
					createdAt, err := time.Parse(time.RFC3339, buildRecord.CreatedAt)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(createdAt).To(BeTemporally("~", time.Now(), time.Second))
				})

				it("returns a record with a UpdatedAt field from the CR", func() {
					updatedAt, err := time.Parse(time.RFC3339, buildRecord.UpdatedAt)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(updatedAt).To(BeTemporally("~", time.Now(), time.Second))
				})

				it("returns a record with a StagingMemoryMB field matching the CR", func() {
					g.Expect(buildRecord.StagingMemoryMB).To(Equal(build2.Spec.StagingMemoryMB))
				})

				it("returns a record with a StagingDiskMB field matching the CR", func() {
					g.Expect(buildRecord.StagingDiskMB).To(Equal(build2.Spec.StagingDiskMB))
				})

				it("returns a record with Lifecycle fields matching the CR", func() {
					g.Expect(buildRecord.Lifecycle.Type).To(Equal(string(build2.Spec.Lifecycle.Type)), "returned record lifecycle.type did not match CR")
					g.Expect(buildRecord.Lifecycle.Data.Buildpacks).To(Equal(build2.Spec.Lifecycle.Data.Buildpacks), "returned record lifecycle.data.buildpacks did not match CR")
					g.Expect(buildRecord.Lifecycle.Data.Stack).To(Equal(build2.Spec.Lifecycle.Data.Stack), "returned record lifecycle.data.stack did not match CR")
				})

				it("returns a record with a PackageGUID field matching the CR", func() {
					g.Expect(buildRecord.PackageGUID).To(Equal(build2.Spec.PackageRef.Name))
				})

				it("returns a record with an AppGUID field matching the CR", func() {
					g.Expect(buildRecord.AppGUID).To(Equal(build2.Spec.AppRef.Name))
				})
			})

			when("status.Conditions \"Staging\": False, \"Ready\": True, \"Succeeded\": True, are set", func() {
				it.Before(func() {
					meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
						Type:    StagingConditionType,
						Status:  metav1.ConditionFalse,
						Reason:  "Unknown",
						Message: "Unknown",
					})
					meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
						Type:    SucceededConditionType,
						Status:  metav1.ConditionTrue,
						Reason:  "Unknown",
						Message: "Unknown",
					})
					meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
						Type:    ReadyConditionType,
						Status:  metav1.ConditionTrue,
						Reason:  "Unknown",
						Message: "Unknown",
					})
					// Update Build Status Conditions based on changes made to local copy
					g.Expect(k8sClient.Status().Update(testCtx, build2)).To(Succeed())
				})

				it("should eventually return a record with State: \"STAGED\", no StagingErrorMsg, and a DropletGUID that matches the BuildGUID", func() {
					ctx := context.Background()
					g.Eventually(func() string {

						returnedBuildRecord, err := buildRepo.FetchBuild(ctx, client, build2GUID)
						buildRecord = &returnedBuildRecord
						fetchError = err
						if err != nil {
							return ""
						}
						return buildRecord.State
					}, 10*time.Second, 250*time.Millisecond).Should(Equal("STAGED"), "the returned record State was not STAGED")
					g.Expect(buildRecord.DropletGUID).To(Equal(build2.Name), " the returned dropletGUID did not match the buildGUID")
					g.Expect(buildRecord.StagingErrorMsg).To(BeEmpty(), "record staging error message was supposed to be empty")
				})
			})

			when("status.Conditions \"Staging\": False, \"Ready\": False, \"Succeeded\": False, are set", func() {
				const (
					StagingErrorMessage = "Staging failed for some reason"
				)

				it.Before(func() {
					meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
						Type:    StagingConditionType,
						Status:  metav1.ConditionFalse,
						Reason:  "StagingError",
						Message: StagingErrorMessage,
					})
					meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
						Type:    SucceededConditionType,
						Status:  metav1.ConditionFalse,
						Reason:  "Unknown",
						Message: "Unknown",
					})
					meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
						Type:    ReadyConditionType,
						Status:  metav1.ConditionFalse,
						Reason:  "Unknown",
						Message: "Unknown",
					})
					// Update Build Status Conditions based on changes made to local copy
					g.Expect(k8sClient.Status().Update(testCtx, build2)).To(Succeed())
				})

				it("should eventually return a record with State: \"FAILED\", no DropletGUID, and a Staging Error Message", func() {
					ctx := context.Background()
					g.Eventually(func() string {
						returnedBuildRecord, err := buildRepo.FetchBuild(ctx, client, build2GUID)
						buildRecord = &returnedBuildRecord
						fetchError = err
						if err != nil {
							return ""
						}
						return buildRecord.State
					}, 10*time.Second, 250*time.Millisecond).Should(Equal("FAILED"), "the returned record State was not FAILED")
					g.Expect(buildRecord.DropletGUID).To(BeEmpty())
					g.Expect(buildRecord.StagingErrorMsg).ToNot(BeEmpty(), "record staging error message was not supposed to be empty")
				})
			})
		})
	})

	when("duplicate Builds exist across namespaces with the same GUID", func() {
		var (
			buildGUID string
			build1    *workloadsv1alpha1.CFBuild
			build2    *workloadsv1alpha1.CFBuild
		)

		it.Before(func() {
			buildGUID = generateGUID()
			build1 = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildGUID,
					Namespace: namespace1.Name,
				},
				Spec: workloadsv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: package1GUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: app1GUID,
					},
					StagingMemoryMB: stagingMemory,
					StagingDiskMB:   stagingDisk,
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), build1)).To(Succeed())

			build2 = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildGUID,
					Namespace: namespace2.Name,
				},
				Spec: workloadsv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: package2GUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: app2GUID,
					},
					StagingMemoryMB: stagingMemory,
					StagingDiskMB:   stagingDisk,
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), build2)).To(Succeed())
		})

		it.After(func() {
			g.Expect(k8sClient.Delete(context.Background(), build1)).To(Succeed())
			g.Expect(k8sClient.Delete(context.Background(), build2)).To(Succeed())
		})

		it("returns an error", func() {
			_, err := buildRepo.FetchBuild(testCtx, client, buildGUID)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError("duplicate builds exist"))
		})
	})

	when("no builds exist", func() {
		it("returns an error", func() {
			_, err := buildRepo.FetchBuild(testCtx, client, "i don't exist")
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError(NotFoundError{}))
		})
	})
}
