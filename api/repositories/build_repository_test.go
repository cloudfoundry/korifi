package repositories_test

import (
	"context"
	"time"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BuildRepository", func() {
	Describe("GetBuild", func() {
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

		BeforeEach(func() {
			testCtx = context.Background()

			namespace1Name := generateGUID()
			namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
			Expect(k8sClient.Create(context.Background(), namespace1)).To(Succeed())

			namespace2Name := generateGUID()
			namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
			Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())

			var err error
			client, err = BuildPrivilegedClient(k8sConfig, "")
			Expect(err).ToNot(HaveOccurred())

			buildRepo = NewBuildRepo(client)
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), namespace1)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), namespace2)).To(Succeed())
		})

		When("on the happy path", func() {
			const (
				StagingConditionType   = "Staging"
				SucceededConditionType = "Succeeded"
			)

			var (
				build1GUID string
				build2GUID string
				build1     *workloadsv1alpha1.CFBuild
				build2     *workloadsv1alpha1.CFBuild
			)

			BeforeEach(func() {
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
				Expect(k8sClient.Create(context.Background(), build1)).To(Succeed())

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
				Expect(k8sClient.Create(context.Background(), build2)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), build1)).To(Succeed())
				Expect(k8sClient.Delete(context.Background(), build2)).To(Succeed())
			})

			When("fetching a build", func() {
				var (
					buildRecord *BuildRecord
					fetchError  error
				)
				When("no status.Conditions are set", func() {
					BeforeEach(func() {
						returnedBuildRecord, err := buildRepo.FetchBuild(testCtx, client, build2GUID)
						buildRecord = &returnedBuildRecord
						fetchError = err
					})

					It("succeeds", func() {
						Expect(fetchError).NotTo(HaveOccurred())
					})

					It("returns a record with a matching GUID", func() {
						Expect(buildRecord.GUID).To(Equal(build2GUID))
					})

					It("returns a record with state \"STAGING\" and no StagingErrorMsg", func() {
						Expect(buildRecord.State).To(Equal("STAGING"))
						Expect(buildRecord.StagingErrorMsg).To(BeEmpty(), "record staging error message was supposed to be empty")
					})

					It("returns a record with no DropletGUID", func() {
						Expect(buildRecord.DropletGUID).To(BeEmpty())
					})

					It("returns a record with a CreatedAt field from the CR", func() {
						createdAt, err := time.Parse(time.RFC3339, buildRecord.CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
					})

					It("returns a record with a UpdatedAt field from the CR", func() {
						updatedAt, err := time.Parse(time.RFC3339, buildRecord.UpdatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
					})

					It("returns a record with a StagingMemoryMB field matching the CR", func() {
						Expect(buildRecord.StagingMemoryMB).To(Equal(build2.Spec.StagingMemoryMB))
					})

					It("returns a record with a StagingDiskMB field matching the CR", func() {
						Expect(buildRecord.StagingDiskMB).To(Equal(build2.Spec.StagingDiskMB))
					})

					It("returns a record with Lifecycle fields matching the CR", func() {
						Expect(buildRecord.Lifecycle.Type).To(Equal(string(build2.Spec.Lifecycle.Type)), "returned record lifecycle.type did not match CR")
						Expect(buildRecord.Lifecycle.Data.Buildpacks).To(BeEmpty(), "returned record lifecycle.data.buildpacks did not match CR")
						Expect(buildRecord.Lifecycle.Data.Stack).To(Equal(build2.Spec.Lifecycle.Data.Stack), "returned record lifecycle.data.stack did not match CR")
					})

					It("returns a record with a PackageGUID field matching the CR", func() {
						Expect(buildRecord.PackageGUID).To(Equal(build2.Spec.PackageRef.Name))
					})

					It("returns a record with an AppGUID field matching the CR", func() {
						Expect(buildRecord.AppGUID).To(Equal(build2.Spec.AppRef.Name))
					})
				})

				When("status.Conditions \"Staging\": False, \"Succeeded\": True, is set", func() {
					BeforeEach(func() {
						meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
							Type:    StagingConditionType,
							Status:  metav1.ConditionFalse,
							Reason:  "kpack",
							Message: "kpack",
						})
						meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
							Type:    SucceededConditionType,
							Status:  metav1.ConditionTrue,
							Reason:  "Unknown",
							Message: "Unknown",
						})
						// Update Build Status Conditions based on changes made to local copy
						Expect(k8sClient.Status().Update(testCtx, build2)).To(Succeed())
					})

					It("should eventually return a record with State: \"STAGED\", no StagingErrorMsg, and a DropletGUID that matches the BuildGUID", func() {
						ctx := context.Background()
						Eventually(func() string {
							returnedBuildRecord, err := buildRepo.FetchBuild(ctx, client, build2GUID)
							buildRecord = &returnedBuildRecord
							fetchError = err
							if err != nil {
								return ""
							}
							return buildRecord.State
						}, 10*time.Second, 250*time.Millisecond).Should(Equal("STAGED"), "the returned record State was not STAGED")
						Expect(buildRecord.DropletGUID).To(Equal(build2.Name), " the returned dropletGUID did not match the buildGUID")
						Expect(buildRecord.StagingErrorMsg).To(BeEmpty(), "record staging error message was supposed to be empty")
					})
				})

				When("status.Conditions \"Staging\": False, \"Succeeded\": False, is set", func() {
					const (
						StagingError        = "StagingError"
						StagingErrorMessage = "Staging failed for some reason"
					)

					BeforeEach(func() {
						meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
							Type:    StagingConditionType,
							Status:  metav1.ConditionFalse,
							Reason:  "kpack",
							Message: "kpack",
						})
						meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
							Type:    SucceededConditionType,
							Status:  metav1.ConditionFalse,
							Reason:  "StagingError",
							Message: StagingErrorMessage,
						})
						// Update Build Status Conditions based on changes made to local copy
						Expect(k8sClient.Status().Update(testCtx, build2)).To(Succeed())
					})

					It("should eventually return a record with State: \"FAILED\", no DropletGUID, and a Staging Error Message", func() {
						ctx := context.Background()
						Eventually(func() string {
							returnedBuildRecord, err := buildRepo.FetchBuild(ctx, client, build2GUID)
							buildRecord = &returnedBuildRecord
							fetchError = err
							if err != nil {
								return ""
							}
							return buildRecord.State
						}, 10*time.Second, 250*time.Millisecond).Should(Equal("FAILED"), "the returned record State was not FAILED")
						Expect(buildRecord.DropletGUID).To(BeEmpty())
						Expect(buildRecord.StagingErrorMsg).To(Equal(StagingError + ": " + StagingErrorMessage))
					})
				})
			})
		})

		When("duplicate Builds exist across namespaces with the same GUID", func() {
			var (
				buildGUID string
				build1    *workloadsv1alpha1.CFBuild
				build2    *workloadsv1alpha1.CFBuild
			)

			BeforeEach(func() {
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
				Expect(k8sClient.Create(context.Background(), build1)).To(Succeed())

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
				Expect(k8sClient.Create(context.Background(), build2)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), build1)).To(Succeed())
				Expect(k8sClient.Delete(context.Background(), build2)).To(Succeed())
			})

			It("returns an error", func() {
				_, err := buildRepo.FetchBuild(testCtx, client, buildGUID)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("duplicate builds exist"))
			})
		})

		When("no builds exist", func() {
			It("returns an error", func() {
				_, err := buildRepo.FetchBuild(testCtx, client, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(NotFoundError{}))
			})
		})
	})

	Describe("CreateBuild", func() {
		const (
			appGUID     = "the-app-guid"
			packageGUID = "the-package-guid"

			buildStagingState = "STAGING"

			buildLifecycleType = "buildpack"
			buildStack         = "cflinuxfs3"

			stagingMemory = 1024
			stagingDisk   = 2048
		)

		var (
			buildRepo              *BuildRepo
			client                 client.Client
			buildCreateLabels      map[string]string
			buildCreateAnnotations map[string]string
			buildCreateMsg         BuildCreateMessage
			spaceGUID              string
		)

		BeforeEach(func() {
			var err error
			client, err = BuildPrivilegedClient(k8sConfig, "")
			Expect(err).NotTo(HaveOccurred())

			buildRepo = NewBuildRepo(client)

			beforeCtx := context.Background()
			spaceGUID = generateGUID()
			Expect(
				k8sClient.Create(beforeCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}}),
			).To(Succeed())

			buildCreateLabels = nil
			buildCreateAnnotations = nil
			buildCreateMsg = BuildCreateMessage{
				AppGUID:         appGUID,
				PackageGUID:     packageGUID,
				SpaceGUID:       spaceGUID,
				StagingMemoryMB: stagingMemory,
				StagingDiskMB:   stagingDisk,
				Lifecycle: Lifecycle{
					Type: buildLifecycleType,
					Data: LifecycleData{
						Buildpacks: []string{},
						Stack:      buildStack,
					},
				},
				Labels:      buildCreateLabels,
				Annotations: buildCreateAnnotations,
			}
		})

		When("creating a Build", func() {
			var (
				buildCreateRecord BuildRecord
				buildCreateErr    error
			)

			BeforeEach(func() {
				ctx := context.Background()
				buildCreateRecord, buildCreateErr = buildRepo.CreateBuild(ctx, client, buildCreateMsg)
			})

			AfterEach(func() {
				afterCtx := context.Background()
				cleanupBuild(afterCtx, client, buildCreateRecord.GUID, spaceGUID)
			})

			It("does not return an error", func() {
				Expect(buildCreateErr).NotTo(HaveOccurred())
			})

			When("examining the returned record", func() {
				It("is not empty", func() {
					Expect(buildCreateRecord).ToNot(Equal(BuildCreateMessage{}))
				})
				It("contains a GUID", func() {
					Expect(buildCreateRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "record GUID was not a 36 character guid")
				})
				It("has a State of \"STAGING\"", func() {
					Expect(buildCreateRecord.State).To(Equal(buildStagingState))
				})
				It("has a CreatedAt that makes sense", func() {
					createdAt, err := time.Parse(time.RFC3339, buildCreateRecord.CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
				})
				It("has a UpdatedAt that makes sense", func() {
					createdAt, err := time.Parse(time.RFC3339, buildCreateRecord.UpdatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
				})
				It("has an empty StagingErrorMsg", func() {
					Expect(buildCreateRecord.StagingErrorMsg).To(BeEmpty())
				})
				It("has StagingMemoryMB that matches the CreateMessage", func() {
					Expect(buildCreateRecord.StagingMemoryMB).To(Equal(stagingMemory))
				})
				It("has StagingDiskMB that matches the CreateMessage", func() {
					Expect(buildCreateRecord.StagingDiskMB).To(Equal(stagingDisk))
				})
				It("has Lifecycle fields that match the CreateMessage", func() {
					Expect(buildCreateRecord.Lifecycle.Type).To(Equal(buildLifecycleType))
					Expect(buildCreateRecord.Lifecycle.Data.Stack).To(Equal(buildStack))
				})
				It("has a PackageGUID that matches the CreateMessage", func() {
					Expect(buildCreateRecord.PackageGUID).To(Equal(packageGUID))
				})
				It("has no DropletGUID", func() {
					Expect(buildCreateRecord.DropletGUID).To(BeEmpty())
				})
				It("has an AppGUID that matches the CreateMessage", func() {
					Expect(buildCreateRecord.AppGUID).To(Equal(appGUID))
				})
				It("has Labels that match the CreateMessage", func() {
					Expect(buildCreateRecord.Labels).To(Equal(buildCreateLabels))
				})
				It("has Annotations that match the CreateMessage", func() {
					Expect(buildCreateRecord.Annotations).To(Equal(buildCreateAnnotations))
				})
			})

			It("should eventually create a new Build CR", func() {
				cfBuildLookupKey := types.NamespacedName{Name: buildCreateRecord.GUID, Namespace: spaceGUID}
				createdCFBuild := new(workloadsv1alpha1.CFBuild)
				Eventually(func() bool {
					err := k8sClient.Get(context.Background(), cfBuildLookupKey, createdCFBuild)
					return err == nil
				}, 5*time.Second, 250*time.Millisecond).Should(BeTrue(), "A CFBuild CR was not eventually created")
			})
		})
	})
})

func cleanupBuild(ctx context.Context, k8sClient client.Client, buildGUID, namespace string) error {
	cfBuild := workloadsv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildGUID,
			Namespace: namespace,
		},
	}
	return k8sClient.Delete(ctx, &cfBuild)
}
