package repositories_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
)

var _ = Describe("BuildRepository", func() {
	var (
		buildRepo *repositories.BuildRepo
		sorter    *fake.BuildSorter
	)

	BeforeEach(func() {
		sorter = new(fake.BuildSorter)
		sorter.SortStub = func(records []repositories.BuildRecord, _ string) []repositories.BuildRecord {
			return records
		}
		buildRepo = repositories.NewBuildRepo(
			klient,
			sorter,
		)
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
			namespace1Name := uuid.NewString()
			namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
			Expect(k8sClient.Create(ctx, namespace1)).To(Succeed())

			namespace2Name := uuid.NewString()
			namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
			Expect(k8sClient.Create(ctx, namespace2)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, namespace1)).To(Succeed())
			Expect(k8sClient.Delete(ctx, namespace2)).To(Succeed())
		})

		makeBuild := func(namespace, buildGUID, packageGUID, appGUID string) *korifiv1alpha1.CFBuild {
			return &korifiv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildGUID,
					Namespace: namespace,
					Labels: map[string]string{
						korifiv1alpha1.SpaceGUIDKey: namespace,
					},
				},
				Spec: korifiv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: packageGUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
					StagingMemoryMB: stagingMemory,
					StagingDiskMB:   stagingDisk,
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: korifiv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
		}

		When("on the happy path", func() {
			var (
				build1GUID string
				build2GUID string
				build1     *korifiv1alpha1.CFBuild
				build2     *korifiv1alpha1.CFBuild
			)

			BeforeEach(func() {
				build1GUID = uuid.NewString()
				build2GUID = uuid.NewString()
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
						Expect(buildRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
					})

					It("returns a record with a UpdatedAt field from the CR", func() {
						Expect(buildRecord.UpdatedAt).To(gstruct.PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
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

					It("sets the space guid on the record", func() {
						Expect(buildRecord.SpaceGUID).To(Equal(namespace2.Name))
					})

					It("returns record with relationships", func() {
						Expect(buildRecord.Relationships()).To(Equal(map[string]string{
							"app": build2.Spec.AppRef.Name,
						}))
					})
				})

				When("status.Conditions \"Staging\": False, \"Succeeded\": True, is set", func() {
					BeforeEach(func() {
						meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
							Type:    korifiv1alpha1.StagingConditionType,
							Status:  metav1.ConditionFalse,
							Reason:  "kpack",
							Message: "kpack",
						})
						meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
							Type:    korifiv1alpha1.SucceededConditionType,
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
							Type:    korifiv1alpha1.StagingConditionType,
							Status:  metav1.ConditionFalse,
							Reason:  "kpack",
							Message: "kpack",
						})
						meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
							Type:    korifiv1alpha1.SucceededConditionType,
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
						Expect(buildRecord.StagingErrorMsg).To(Equal(StagingErrorMessage))
					})
				})
			})
		})

		When("duplicate Builds exist across namespaces with the same GUID", func() {
			var buildGUID string

			BeforeEach(func() {
				buildGUID = uuid.NewString()
				build1 := makeBuild(namespace1.Name, buildGUID, package1GUID, app1GUID)
				Expect(k8sClient.Create(ctx, build1)).To(Succeed())
				build2 := makeBuild(namespace2.Name, buildGUID, package2GUID, app2GUID)
				Expect(k8sClient.Create(ctx, build2)).To(Succeed())
			})

			It("returns an error", func() {
				_, err := buildRepo.GetBuild(ctx, authInfo, buildGUID)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("get-build duplicate records exist")))
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
				buildGUID = uuid.NewString()
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

	Describe("GetLatestBuildByAppGUID", func() {
		const (
			packageGUID = "package-guid"
		)

		var (
			space       *korifiv1alpha1.CFSpace
			appGUID     string
			checkSpace  string
			buildRecord repositories.BuildRecord
			fetchError  error
		)

		BeforeEach(func() {
			orgGUID := prefixedGUID("get-latest-build-org")
			org := createOrgWithCleanup(ctx, orgGUID)
			space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("get-latest-build-space"))
			checkSpace = space.Name
			appGUID = prefixedGUID("get-latest-build-app")
		})

		JustBeforeEach(func() {
			buildRecord, fetchError = buildRepo.GetLatestBuildByAppGUID(ctx, authInfo, checkSpace, appGUID)
		})

		When("the user has space developer role", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			When("on the happy path", func() {
				var build3 *korifiv1alpha1.CFBuild

				BeforeEach(func() {
					_ = createBuild(ctx, k8sClient, space.Name, prefixedGUID("first"), packageGUID, appGUID)
					_ = createBuild(ctx, k8sClient, space.Name, prefixedGUID("second"), packageGUID, appGUID)
					time.Sleep(1001 * time.Millisecond)
					build3 = createBuild(ctx, k8sClient, space.Name, prefixedGUID("third"), packageGUID, appGUID)
				})

				When("fetching builds for an app", func() {
					It("it returns a record that matches the last created build and no error", func() {
						Expect(fetchError).NotTo(HaveOccurred())
						Expect(buildRecord.GUID).To(Equal(build3.Name))
						Expect(buildRecord.AppGUID).To(Equal(appGUID))
					})
				})
			})

			When("the app has no builds", func() {
				BeforeEach(func() {
					appGUID = "i-dont-exist"
				})
				It("returns an empty record and a not found error", func() {
					Expect(fetchError).To(HaveOccurred())
					Expect(fetchError).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
					Expect(buildRecord).To(Equal(repositories.BuildRecord{}))
				})
			})
		})

		When("the user has no role in the space", func() {
			It("returns a forbidden error", func() {
				Expect(fetchError).To(HaveOccurred())
				Expect(fetchError).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
				Expect(buildRecord).To(Equal(repositories.BuildRecord{}))
			})
		})

		When("the namespace doesn't exist", func() {
			BeforeEach(func() {
				checkSpace = "i-dont-exist"
			})

			It("returns a forbidden error", func() {
				Expect(fetchError).To(HaveOccurred())
				Expect(fetchError).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
				Expect(buildRecord).To(Equal(repositories.BuildRecord{}))
			})
		})
	})

	Describe("CreateBuild", func() {
		const (
			appGUID     = "the-app-guid"
			packageGUID = "the-package-guid"

			buildStagingState = "STAGING"

			buildpackLifecycleType = "buildpack"
			buildStack             = "cflinuxfs3"

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
			spaceGUID = uuid.NewString()
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
					Type: buildpackLifecycleType,
					Data: repositories.LifecycleData{
						Buildpacks: []string{},
						Stack:      buildStack,
					},
				},
				Labels:      buildCreateLabels,
				Annotations: buildCreateAnnotations,
			}
		})

		When("the user is authorized to create a Build", func() {
			var (
				buildCreateRecord repositories.BuildRecord
				buildCreateErr    error
			)

			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, spaceGUID)
			})

			JustBeforeEach(func() {
				buildCreateRecord, buildCreateErr = buildRepo.CreateBuild(ctx, authInfo, buildCreateMsg)
			})

			AfterEach(func() {
				if buildCreateErr == nil {
					Expect(cleanupBuild(ctx, buildCreateRecord.GUID, spaceGUID)).To(Succeed())
				}
			})

			It("returns correct build record", func() {
				Expect(buildCreateErr).NotTo(HaveOccurred())

				Expect(buildCreateRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "record GUID was not a 36 character guid")
				Expect(buildCreateRecord.State).To(Equal(buildStagingState))
				Expect(buildCreateRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(buildCreateRecord.UpdatedAt).To(gstruct.PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
				Expect(buildCreateRecord.StagingErrorMsg).To(BeEmpty())
				Expect(buildCreateRecord.StagingMemoryMB).To(Equal(stagingMemory))
				Expect(buildCreateRecord.StagingDiskMB).To(Equal(stagingDisk))
				Expect(buildCreateRecord.Lifecycle.Type).To(Equal(buildpackLifecycleType))
				Expect(buildCreateRecord.Lifecycle.Data.Stack).To(Equal(buildStack))
				Expect(buildCreateRecord.PackageGUID).To(Equal(packageGUID))
				Expect(buildCreateRecord.DropletGUID).To(BeEmpty())
				Expect(buildCreateRecord.AppGUID).To(Equal(appGUID))
				Expect(buildCreateRecord.Labels).To(Equal(buildCreateLabels))
				Expect(buildCreateRecord.Annotations).To(Equal(buildCreateAnnotations))
				Expect(buildCreateRecord.Annotations).To(Equal(buildCreateAnnotations))
			})

			It("creates a new Build CR", func() {
				cfBuildLookupKey := types.NamespacedName{Name: buildCreateRecord.GUID, Namespace: spaceGUID}

				cfBuild := korifiv1alpha1.CFBuild{}
				Expect(k8sClient.Get(ctx, cfBuildLookupKey, &cfBuild)).To(Succeed())

				Expect(cfBuild.Name).To(MatchRegexp("^[-0-9a-f]{36}$"), "record GUID was not a 36 character guid")
				Expect(cfBuild.Labels).To(Equal(buildCreateLabels))
				Expect(cfBuild.Annotations).To(Equal(buildCreateAnnotations))
				Expect(cfBuild.Annotations).To(Equal(buildCreateAnnotations))
				Expect(cfBuild.Spec.PackageRef.Name).To(Equal(packageGUID))
				Expect(cfBuild.Spec.AppRef.Name).To(Equal(appGUID))
				Expect(cfBuild.Spec.StagingMemoryMB).To(Equal(stagingMemory))
				Expect(cfBuild.Spec.StagingDiskMB).To(Equal(stagingDisk))
				Expect(cfBuild.Spec.Lifecycle.Type).To(Equal(korifiv1alpha1.LifecycleType(buildpackLifecycleType)))
				Expect(cfBuild.Spec.Lifecycle.Data.Stack).To(Equal(buildStack))
			})

			When("the lifecycle type is docker", func() {
				BeforeEach(func() {
					buildCreateMsg.Lifecycle = repositories.Lifecycle{
						Type: "docker",
					}
				})

				It("returns correct build record", func() {
					Expect(buildCreateErr).NotTo(HaveOccurred())

					Expect(buildCreateRecord.GUID).To(matchers.BeValidUUID())
					Expect(buildCreateRecord.Lifecycle.Type).To(Equal("docker"))
					Expect(buildCreateRecord.Lifecycle.Data).To(Equal(repositories.LifecycleData{}))
				})

				It("creates a new Build CR", func() {
					cfBuildLookupKey := types.NamespacedName{Name: buildCreateRecord.GUID, Namespace: spaceGUID}

					cfBuild := korifiv1alpha1.CFBuild{}
					Expect(k8sClient.Get(ctx, cfBuildLookupKey, &cfBuild)).To(Succeed())

					Expect(cfBuild.Spec.Lifecycle.Type).To(Equal(korifiv1alpha1.LifecycleType("docker")))
					Expect(cfBuild.Spec.Lifecycle.Data).To(Equal(korifiv1alpha1.LifecycleData{}))
				})
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

	Describe("ListBuilds", func() {
		var (
			app1GUID     string
			app2GUID     string
			package1GUID string
			package2GUID string
			namespace1   *korifiv1alpha1.CFSpace
			namespace2   *korifiv1alpha1.CFSpace
			cfOrg        *korifiv1alpha1.CFOrg
			build1GUID   string
			build2GUID   string
			build1       *korifiv1alpha1.CFBuild
			build2       *korifiv1alpha1.CFBuild
			buildRecords []repositories.BuildRecord
			fetchError   error
			listMessage  repositories.ListBuildsMessage
		)

		BeforeEach(func() {
			build1GUID = uuid.NewString()
			build2GUID = uuid.NewString()
			app1GUID = uuid.NewString()
			app2GUID = uuid.NewString()
			package1GUID = uuid.NewString()
			package2GUID = uuid.NewString()
			listMessage = repositories.ListBuildsMessage{}
			cfOrg = createOrgWithCleanup(ctx, prefixedGUID("org"))
			namespace1 = createSpaceWithCleanup(ctx, cfOrg.Name, prefixedGUID("space2"))
			namespace2 = createSpaceWithCleanup(ctx, cfOrg.Name, prefixedGUID("space3"))
			build1 = &korifiv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      build1GUID,
					Namespace: namespace1.Name,
					Labels: map[string]string{
						korifiv1alpha1.SpaceGUIDKey: namespace1.Name,
					},
				},
				Spec: korifiv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: package1GUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: app1GUID,
					},
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: korifiv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, build1)).To(Succeed())
			build2 = &korifiv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      build2GUID,
					Namespace: namespace2.Name,
					Labels: map[string]string{
						korifiv1alpha1.SpaceGUIDKey: namespace2.Name,
					},
				},
				Spec: korifiv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: package2GUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: app2GUID,
					},
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: korifiv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, build2)).To(Succeed())
		})
		JustBeforeEach(func() {
			buildRecords, fetchError = buildRepo.ListBuilds(ctx, authInfo, listMessage)
		})

		It("returns an empty array (as no roles assigned)", func() {
			Expect(fetchError).NotTo(HaveOccurred())
			Expect(buildRecords).To(BeEmpty())
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace1.Name)
			})
			It("if user is a Space Developer it returns a list with one element", func() {
				Expect(fetchError).NotTo(HaveOccurred())
				Expect(buildRecords).To(HaveLen(1))
				Expect(buildRecords[0].GUID).To(Equal(build1GUID))
			})
			When("the app_guids filter is provided", func() {
				BeforeEach(func() {
					listMessage = repositories.ListBuildsMessage{AppGUIDs: []string{app1GUID}}
				})
				It("fetches all BuildRecords for that app", func() {
					Expect(fetchError).NotTo(HaveOccurred())
					Expect(buildRecords).To(ConsistOf(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"AppGUID": Equal(app1GUID),
					}),
					))
				})
			})
			When("the package_guids filter is provided", func() {
				BeforeEach(func() {
					listMessage = repositories.ListBuildsMessage{PackageGUIDs: []string{package1GUID}}
				})
				It("fetches all BuildRecords for that package", func() {
					Expect(fetchError).NotTo(HaveOccurred())
					Expect(buildRecords).To(ConsistOf(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"PackageGUID": Equal(package1GUID),
					}),
					))
				})
			})
			When("the state filter is provided", func() {
				When("filtering by State=STAGING", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, build1, func() {
							meta.SetStatusCondition(&build1.Status.Conditions, metav1.Condition{
								Type:    korifiv1alpha1.StagingConditionType,
								Status:  metav1.ConditionTrue,
								Reason:  "kpack",
								Message: "kpack",
							})
						})).To(Succeed())
						listMessage = repositories.ListBuildsMessage{States: []string{"STAGING"}}
					})

					It("filters the builds", func() {
						Expect(fetchError).NotTo(HaveOccurred())
						Expect(buildRecords).To(ConsistOf(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal("STAGING"),
						})))
					})
				})
				When("filtering by State=STAGED", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, build1, func() {
							meta.SetStatusCondition(&build1.Status.Conditions, metav1.Condition{
								Type:    korifiv1alpha1.StagingConditionType,
								Status:  metav1.ConditionFalse,
								Reason:  "Unknown",
								Message: "Unknown",
							})
							meta.SetStatusCondition(&build1.Status.Conditions, metav1.Condition{
								Type:    korifiv1alpha1.SucceededConditionType,
								Status:  metav1.ConditionTrue,
								Reason:  "Unknown",
								Message: "Unknown",
							})
						})).To(Succeed())

						listMessage = repositories.ListBuildsMessage{States: []string{"STAGED"}}
					})

					It("filters the builds", func() {
						Expect(fetchError).NotTo(HaveOccurred())
						Expect(buildRecords).To(ConsistOf(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"State": Equal("STAGED"),
						})))
					})
				})
				When("filtering by State=FAILED", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, build1, func() {
							meta.SetStatusCondition(&build1.Status.Conditions, metav1.Condition{
								Type:    korifiv1alpha1.StagingConditionType,
								Status:  metav1.ConditionFalse,
								Reason:  "Unknown",
								Message: "Unknown",
							})
							meta.SetStatusCondition(&build1.Status.Conditions, metav1.Condition{
								Type:    korifiv1alpha1.SucceededConditionType,
								Status:  metav1.ConditionFalse,
								Reason:  "Unknown",
								Message: "Unknown",
							})
						})).To(Succeed())

						listMessage = repositories.ListBuildsMessage{States: []string{"FAILED"}}
					})

					It("filters the builds", func() {
						Expect(fetchError).NotTo(HaveOccurred())
						Expect(buildRecords).To(ConsistOf(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"GUID":  Equal(build1.Name),
							"State": Equal("FAILED"),
						})))
					})
				})
			})
		})
	})
})

func cleanupBuild(ctx context.Context, buildGUID, namespace string) error {
	cfBuild := korifiv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildGUID,
			Namespace: namespace,
		},
	}
	return k8sClient.Delete(ctx, &cfBuild)
}
