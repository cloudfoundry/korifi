package repositories_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("BuildRepository", func() {
	var (
		ctx       context.Context
		buildRepo *repositories.BuildRepo
	)

	BeforeEach(func() {
		ctx = context.Background()

		buildRepo = repositories.NewBuildRepo(namespaceRetriever, userClientFactory)
	})

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
			namespace1 *corev1.Namespace
			namespace2 *corev1.Namespace
		)

		BeforeEach(func() {
			namespace1Name := generateGUID()
			namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
			Expect(k8sClient.Create(ctx, namespace1)).To(Succeed())

			namespace2Name := generateGUID()
			namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
			Expect(k8sClient.Create(ctx, namespace2)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, namespace1)).To(Succeed())
			Expect(k8sClient.Delete(ctx, namespace2)).To(Succeed())
		})

		makeBuild := func(namespace, buildGUID, packageGUID, appGUID string) *workloadsv1alpha1.CFBuild {
			return &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildGUID,
					Namespace: namespace,
				},
				Spec: workloadsv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: packageGUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
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
		}

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
				build1 = makeBuild(namespace1.Name, build1GUID, package1GUID, app1GUID)
				Expect(k8sClient.Create(ctx, build1)).To(Succeed())
				build2 = makeBuild(namespace2.Name, build2GUID, package2GUID, app2GUID)
				Expect(k8sClient.Create(ctx, build2)).To(Succeed())

				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace1.Name)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace2.Name)
			})

			When("fetching a build", func() {
				var (
					buildRecord *repositories.BuildRecord
					fetchError  error
				)
				When("no status.Conditions are set", func() {
					BeforeEach(func() {
						returnedBuildRecord, err := buildRepo.GetBuild(ctx, authInfo, build2GUID)
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
						Expect(k8sClient.Status().Update(ctx, build2)).To(Succeed())
					})

					It("should return a record with State: \"STAGED\", no StagingErrorMsg, and a DropletGUID that matches the BuildGUID", func() {
						buildRecord, err := buildRepo.GetBuild(ctx, authInfo, build2GUID)
						Expect(err).NotTo(HaveOccurred())
						Expect(buildRecord.State).To(Equal("STAGED"))
						Expect(buildRecord.DropletGUID).To(Equal(build2.Name))
						Expect(buildRecord.StagingErrorMsg).To(BeEmpty())
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
						Expect(k8sClient.Status().Update(ctx, build2)).To(Succeed())
					})

					It("should return a record with State: \"FAILED\", no DropletGUID, and a Staging Error Message", func() {
						buildRecord, err := buildRepo.GetBuild(ctx, authInfo, build2GUID)
						Expect(err).NotTo(HaveOccurred())
						Expect(buildRecord.State).To(Equal("FAILED"))
						Expect(buildRecord.DropletGUID).To(BeEmpty())
						Expect(buildRecord.StagingErrorMsg).To(Equal(StagingError + ": " + StagingErrorMessage))
					})
				})
			})
		})

		When("duplicate Builds exist across namespaces with the same GUID", func() {
			var buildGUID string

			BeforeEach(func() {
				buildGUID = generateGUID()
				build1 := makeBuild(namespace1.Name, buildGUID, package1GUID, app1GUID)
				Expect(k8sClient.Create(ctx, build1)).To(Succeed())
				build2 := makeBuild(namespace2.Name, buildGUID, package2GUID, app2GUID)
				Expect(k8sClient.Create(ctx, build2)).To(Succeed())
			})

			It("returns an error", func() {
				_, err := buildRepo.GetBuild(ctx, authInfo, buildGUID)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("get-build duplicate records exist"))
			})
		})

		When("no builds exist", func() {
			It("returns an error", func() {
				_, err := buildRepo.GetBuild(ctx, authInfo, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

		When("the user is not authorized for builds in the namespace", func() {
			var buildGUID string

			BeforeEach(func() {
				buildGUID = generateGUID()
				build1 := makeBuild(namespace1.Name, buildGUID, package1GUID, app1GUID)
				Expect(k8sClient.Create(ctx, build1)).To(Succeed())
			})

			It("returns a forbidden error", func() {
				_, err := buildRepo.GetBuild(ctx, authInfo, buildGUID)
				Expect(err).To(HaveOccurred())
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("CreateBuild", func() {
		const (
			appGUID     = "the-app-guid"
			packageUID  = "the-package-uid"
			packageGUID = "the-package-guid"

			buildStagingState = "STAGING"

			buildLifecycleType = "buildpack"
			buildStack         = "cflinuxfs3"

			stagingMemory = 1024
			stagingDisk   = 2048
		)

		var (
			buildCreateLabels      map[string]string
			buildCreateAnnotations map[string]string
			buildCreateMsg         repositories.CreateBuildMessage
			spaceGUID              string
		)

		BeforeEach(func() {
			spaceGUID = generateGUID()
			Expect(
				k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}}),
			).To(Succeed())

			buildCreateLabels = nil
			buildCreateAnnotations = nil
			buildCreateMsg = repositories.CreateBuildMessage{
				AppGUID:         appGUID,
				PackageGUID:     packageGUID,
				SpaceGUID:       spaceGUID,
				StagingMemoryMB: stagingMemory,
				StagingDiskMB:   stagingDisk,
				Lifecycle: repositories.Lifecycle{
					Type: buildLifecycleType,
					Data: repositories.LifecycleData{
						Buildpacks: []string{},
						Stack:      buildStack,
					},
				},
				Labels:      buildCreateLabels,
				Annotations: buildCreateAnnotations,
				OwnerRef: metav1.OwnerReference{
					APIVersion: repositories.APIVersion,
					Kind:       "CFPackage",
					Name:       packageGUID,
					UID:        packageUID,
				},
			}
		})

		When("creating a Build", func() {
			var (
				buildCreateRecord repositories.BuildRecord
				buildCreateErr    error
			)

			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, spaceGUID)
				buildCreateRecord, buildCreateErr = buildRepo.CreateBuild(ctx, authInfo, buildCreateMsg)
			})

			AfterEach(func() {
				Expect(cleanupBuild(ctx, buildCreateRecord.GUID, spaceGUID)).To(Succeed())
			})

			It("does not return an error", func() {
				Expect(buildCreateErr).NotTo(HaveOccurred())
			})

			When("examining the returned record", func() {
				It("is not empty", func() {
					Expect(buildCreateRecord).ToNot(Equal(repositories.CreateBuildMessage{}))
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
				err := k8sClient.Get(ctx, cfBuildLookupKey, createdCFBuild)
				Expect(err).NotTo(HaveOccurred())

				Expect(createdCFBuild.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
					{
						APIVersion: "workloads.cloudfoundry.org/v1alpha1",
						Kind:       "CFPackage",
						Name:       packageGUID,
						UID:        packageUID,
					},
				}))
			})
		})

		When("the user is not authorized for builds in the namespace", func() {
			It("returns a forbidden error", func() {
				_, err := buildRepo.CreateBuild(ctx, authInfo, buildCreateMsg)
				Expect(err).To(HaveOccurred())
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})
})

func cleanupBuild(ctx context.Context, buildGUID, namespace string) error {
	cfBuild := workloadsv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildGUID,
			Namespace: namespace,
		},
	}
	return k8sClient.Delete(ctx, &cfBuild)
}
