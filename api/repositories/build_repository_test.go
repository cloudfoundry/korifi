package repositories_test

import (
	"time"

	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
)

var _ = Describe("BuildRepository", func() {
	var (
		buildRepo *repositories.BuildRepo

		cfSpace *korifiv1alpha1.CFSpace
		build   *korifiv1alpha1.CFBuild
	)

	BeforeEach(func() {
		buildRepo = repositories.NewBuildRepo(spaceScopedKlient)

		org := createOrgWithCleanup(ctx, uuid.NewString())
		cfSpace = createSpaceWithCleanup(ctx, org.Name, uuid.NewString())

		appGUID := uuid.NewString()
		packageGUID := uuid.NewString()
		build = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: cfSpace.Name,
			},
			Spec: korifiv1alpha1.CFBuildSpec{
				PackageRef: corev1.LocalObjectReference{
					Name: packageGUID,
				},
				AppRef: corev1.LocalObjectReference{
					Name: appGUID,
				},
				StagingMemoryMB: 123,
				StagingDiskMB:   456,
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}
		Expect(k8sClient.Create(ctx, build)).To(Succeed())
	})

	Describe("GetBuild", func() {
		var (
			buildGUID string
			record    repositories.BuildRecord
			err       error
		)

		BeforeEach(func() {
			buildGUID = build.Name
		})

		JustBeforeEach(func() {
			record, err = buildRepo.GetBuild(ctx, authInfo, buildGUID)
		})

		It("returns a forbidden error", func() {
			Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("returns a build record", func() {
				Expect(err).NotTo(HaveOccurred())

				Expect(record.GUID).To(Equal(build.Name))
				Expect(record.State).To(Equal(korifiv1alpha1.BuildStateStaging))
				Expect(record.StagingErrorMsg).To(BeEmpty())
				Expect(record.DropletGUID).To(BeEmpty())
				Expect(record.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(record.UpdatedAt).To(gstruct.PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
				Expect(record.StagingMemoryMB).To(Equal(123))
				Expect(record.StagingDiskMB).To(Equal(456))
				Expect(record.Lifecycle.Type).To(Equal("buildpack"))
				Expect(record.Lifecycle.Data.Buildpacks).To(BeEmpty())
				Expect(record.Lifecycle.Data.Stack).To(BeEmpty())
				Expect(record.PackageGUID).To(Equal(build.Spec.PackageRef.Name))
				Expect(record.AppGUID).To(Equal(build.Spec.AppRef.Name))
				Expect(record.SpaceGUID).To(Equal(cfSpace.Name))
				Expect(record.Relationships()).To(Equal(map[string]string{
					"app": build.Spec.AppRef.Name,
				}))
			})

			When("the build is staged", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, build, func() {
						build.Status.State = korifiv1alpha1.BuildStateStaged
					})).To(Succeed())
				})

				It("it returns a staged build record", func() {
					Expect(err).NotTo(HaveOccurred())

					Expect(record.State).To(Equal(korifiv1alpha1.BuildStateStaged))
					Expect(record.DropletGUID).To(Equal(build.Name))
					Expect(record.StagingErrorMsg).To(BeEmpty())
				})
			})

			When("the build has failed", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, build, func() {
						build.Status.State = korifiv1alpha1.BuildStateFailed
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    korifiv1alpha1.SucceededConditionType,
							Status:  metav1.ConditionFalse,
							Reason:  "StagingError",
							Message: "because it failed",
						})
					})).To(Succeed())
				})

				It("returns a failed build record", func() {
					Expect(err).NotTo(HaveOccurred())

					Expect(record.State).To(Equal(korifiv1alpha1.BuildStateFailed))
					Expect(record.DropletGUID).To(BeEmpty())
					Expect(record.StagingErrorMsg).To(Equal("because it failed"))
				})
			})

			When("duplicate Builds exist across namespaces with the same GUID", func() {
				BeforeEach(func() {
					anotherOrg := createOrgWithCleanup(ctx, uuid.NewString())
					anotherSpace := createSpaceWithCleanup(ctx, anotherOrg.Name, uuid.NewString())

					createRoleBinding(ctx, userName, spaceDeveloperRole.Name, anotherSpace.Name)

					Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFBuild{
						ObjectMeta: metav1.ObjectMeta{
							Name:      build.Name,
							Namespace: anotherSpace.Name,
						},
						Spec: korifiv1alpha1.CFBuildSpec{
							Lifecycle: korifiv1alpha1.Lifecycle{
								Type: "buildpack",
							},
						},
					})).To(Succeed())
				})

				It("returns an error", func() {
					Expect(err).To(MatchError(ContainSubstring("get-build duplicate records exist")))
				})
			})

			When("the build does not exist", func() {
				BeforeEach(func() {
					buildGUID = "i do not exist"
				})

				It("returns an error", func() {
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})
	})

	Describe("GetLatestBuildByAppGUID", func() {
		var (
			appGUID   string
			spaceGUID string
			record    repositories.BuildRecord
			err       error
		)

		BeforeEach(func() {
			appGUID = uuid.NewString()
			spaceGUID = cfSpace.Name
		})

		JustBeforeEach(func() {
			record, err = buildRepo.GetLatestBuildByAppGUID(ctx, authInfo, spaceGUID, appGUID)
		})

		It("returns a not found error", func() {
			Expect(err).To(BeAssignableToTypeOf(apierrors.NotFoundError{}))
		})

		When("the user is a space developer", func() {
			var newerBuild *korifiv1alpha1.CFBuild

			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfSpace.Name)
				time.Sleep(1001 * time.Millisecond)

				newerBuild = &korifiv1alpha1.CFBuild{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: cfSpace.Name,
					},
					Spec: korifiv1alpha1.CFBuildSpec{
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
						Lifecycle: korifiv1alpha1.Lifecycle{
							Type: "buildpack",
						},
					},
				}
				Expect(k8sClient.Create(ctx, newerBuild)).To(Succeed())
			})

			It("returns a record for the latest build for the app", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(record.GUID).To(Equal(newerBuild.Name))
			})

			When("the app has no builds", func() {
				BeforeEach(func() {
					appGUID = "i-dont-exist"
				})

				It("returns a not found error", func() {
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})
	})

	Describe("CreateBuild", func() {
		var (
			createMsg repositories.CreateBuildMessage
			record    repositories.BuildRecord
			err       error
		)

		BeforeEach(func() {
			createMsg = repositories.CreateBuildMessage{
				AppGUID:         uuid.NewString(),
				PackageGUID:     uuid.NewString(),
				SpaceGUID:       cfSpace.Name,
				StagingMemoryMB: 123,
				StagingDiskMB:   456,
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Stack: "my-build-stack",
					},
				},
				Labels:      map[string]string{"label-key": "label-value"},
				Annotations: map[string]string{"annotation-key": "annotation-value"},
			}
		})

		JustBeforeEach(func() {
			record, err = buildRepo.CreateBuild(ctx, authInfo, createMsg)
		})

		It("returns a forbidden error", func() {
			Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("returns correct build record", func() {
				Expect(err).NotTo(HaveOccurred())

				Expect(record.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "record GUID was not a 36 character guid")
				Expect(record.State).To(Equal(korifiv1alpha1.BuildStateStaging))
				Expect(record.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(record.UpdatedAt).To(gstruct.PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
				Expect(record.StagingErrorMsg).To(BeEmpty())
				Expect(record.StagingMemoryMB).To(Equal(123))
				Expect(record.StagingDiskMB).To(Equal(456))
				Expect(record.Lifecycle.Type).To(Equal("buildpack"))
				Expect(record.Lifecycle.Data.Stack).To(Equal("my-build-stack"))
				Expect(record.PackageGUID).To(Equal(createMsg.PackageGUID))
				Expect(record.DropletGUID).To(BeEmpty())
				Expect(record.AppGUID).To(Equal(createMsg.AppGUID))
				Expect(record.Labels).To(HaveKeyWithValue("label-key", "label-value"))
				Expect(record.Annotations).To(HaveKeyWithValue("annotation-key", "annotation-value"))
			})

			It("creates a new Build CR", func() {
				Expect(err).NotTo(HaveOccurred())

				cfBuild := &korifiv1alpha1.CFBuild{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: cfSpace.Name,
						Name:      record.GUID,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())

				Expect(cfBuild.Labels).To(HaveKeyWithValue("label-key", "label-value"))
				Expect(cfBuild.Annotations).To(HaveKeyWithValue("annotation-key", "annotation-value"))
				Expect(cfBuild.Spec.PackageRef.Name).To(Equal(createMsg.PackageGUID))
				Expect(cfBuild.Spec.AppRef.Name).To(Equal(createMsg.AppGUID))
				Expect(cfBuild.Spec.StagingMemoryMB).To(Equal(123))
				Expect(cfBuild.Spec.StagingDiskMB).To(Equal(456))
				Expect(cfBuild.Spec.Lifecycle.Type).To(BeEquivalentTo("buildpack"))
				Expect(cfBuild.Spec.Lifecycle.Data.Stack).To(Equal("my-build-stack"))
			})

			When("the lifecycle type is docker", func() {
				BeforeEach(func() {
					createMsg.Lifecycle = repositories.Lifecycle{
						Type: "docker",
					}
				})

				It("returns correct build record", func() {
					Expect(err).NotTo(HaveOccurred())

					Expect(record.GUID).To(matchers.BeValidUUID())
					Expect(record.Lifecycle.Type).To(Equal("docker"))
					Expect(record.Lifecycle.Data).To(Equal(repositories.LifecycleData{}))
				})

				It("creates a new Build CR", func() {
					Expect(err).NotTo(HaveOccurred())

					cfBuild := &korifiv1alpha1.CFBuild{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: cfSpace.Name,
							Name:      record.GUID,
						},
					}
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())

					Expect(cfBuild.Spec.Lifecycle.Type).To(Equal(korifiv1alpha1.LifecycleType("docker")))
					Expect(cfBuild.Spec.Lifecycle.Data).To(Equal(korifiv1alpha1.LifecycleData{}))
				})
			})
		})
	})

	Describe("ListBuilds", func() {
		var (
			anotherBuild *korifiv1alpha1.CFBuild
			listResult   repositories.ListResult[repositories.BuildRecord]
			fetchError   error
			listMessage  repositories.ListBuildsMessage
		)

		BeforeEach(func() {
			listMessage = repositories.ListBuildsMessage{}

			app2GUID := uuid.NewString()
			package2GUID := uuid.NewString()

			anotherBuild = &korifiv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: cfSpace.Name,
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, anotherBuild)).To(Succeed())
		})

		JustBeforeEach(func() {
			listResult, fetchError = buildRepo.ListBuilds(ctx, authInfo, listMessage)
		})

		It("returns an empty array (as no roles assigned)", func() {
			Expect(fetchError).NotTo(HaveOccurred())
			Expect(listResult.Records).To(BeEmpty())
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("returns records for all builds", func() {
				Expect(fetchError).NotTo(HaveOccurred())

				Expect(listResult.Records).To(ConsistOf(
					gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"GUID": Equal(build.Name),
					}),
					gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"GUID": Equal(anotherBuild.Name),
					}),
				))

				Expect(listResult.PageInfo).To(Equal(descriptors.PageInfo{
					TotalResults: 2,
					TotalPages:   1,
					PageNumber:   1,
					PageSize:     2,
				}))
			})

			Describe("list parameters", func() {
				var fakeKlient *fake.Klient

				BeforeEach(func() {
					fakeKlient = new(fake.Klient)
					buildRepo = repositories.NewBuildRepo(fakeKlient)

					listMessage = repositories.ListBuildsMessage{
						PackageGUIDs: []string{"p1", "p2"},
						AppGUIDs:     []string{"a1", "a2"},
						States:       []string{"s1", "s2"},
						OrderBy:      "created_at",
						Pagination: repositories.Pagination{
							PerPage: 10,
							Page:    1,
						},
					}
				})

				It("translates  parameters to klient list options", func() {
					Expect(fakeKlient.ListCallCount()).To(Equal(1))
					_, _, listOptions := fakeKlient.ListArgsForCall(0)
					Expect(listOptions).To(ConsistOf(
						repositories.WithLabelIn(korifiv1alpha1.CFPackageGUIDLabelKey, []string{"p1", "p2"}),
						repositories.WithLabelIn(korifiv1alpha1.CFAppGUIDLabelKey, []string{"a1", "a2"}),
						repositories.WithLabelIn(korifiv1alpha1.CFBuildStateLabelKey, []string{"s1", "s2"}),
						repositories.WithOrdering("created_at"),
						repositories.WithPaging(repositories.Pagination{
							PerPage: 10,
							Page:    1,
						}),
					))
				})
			})
		})
	})
})
