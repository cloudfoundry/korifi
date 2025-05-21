package repositories_test

import (
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("DropletRepository", func() {
	const (
		appGUID             = "app-1-guid"
		stagingMemory       = 1024
		stagingDisk         = 2048
		dropletStack        = "cflinuxfs3"
		registryImage       = "registry/image:tag"
		registryImageSecret = "secret-key"
	)

	var (
		dropletRepo *repositories.DropletRepo
		org         *korifiv1alpha1.CFOrg
		space       *korifiv1alpha1.CFSpace
		build       *korifiv1alpha1.CFBuild
		packageGUID string
		buildGUID   string
	)

	BeforeEach(func() {
		orgName := prefixedGUID("org-")
		spaceName := prefixedGUID("space-")
		packageGUID = prefixedGUID("package-")
		buildGUID = prefixedGUID("build-")
		org = createOrgWithCleanup(ctx, orgName)
		space = createSpaceWithCleanup(ctx, org.Name, spaceName)

		dropletRepo = repositories.NewDropletRepo(klient)

		build = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Name:      buildGUID,
				Namespace: space.Name,
				Labels: map[string]string{
					"key1":                               "val1",
					"key2":                               "val2",
					korifiv1alpha1.CFPackageGUIDLabelKey: packageGUID,
					korifiv1alpha1.CFAppGUIDLabelKey:     appGUID,
					korifiv1alpha1.SpaceGUIDKey:          space.Name,
				},
				Annotations: map[string]string{
					"key1": "val1",
					"key2": "val2",
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
				},
			},
		}
		Expect(k8sClient.Create(ctx, build)).To(Succeed())
	})

	Describe("GetDroplet", func() {
		var (
			dropletRecord  repositories.DropletRecord
			fetchBuildGUID string
			fetchErr       error
		)

		BeforeEach(func() {
			fetchBuildGUID = buildGUID
		})

		JustBeforeEach(func() {
			dropletRecord, fetchErr = dropletRepo.GetDroplet(ctx, authInfo, fetchBuildGUID)
		})

		When("the user is authorized to get the droplet", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			When("status.Droplet is set", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, build, func() {
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Staging",
							Status:  metav1.ConditionFalse,
							Reason:  "kpack",
							Message: "kpack",
						})
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Succeeded",
							Status:  metav1.ConditionTrue,
							Reason:  "Unknown",
							Message: "Unknown",
						})
						build.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{
							Stack: dropletStack,
							Registry: korifiv1alpha1.Registry{
								Image: registryImage,
								ImagePullSecrets: []corev1.LocalObjectReference{
									{
										Name: registryImageSecret,
									},
								},
							},
							ProcessTypes: []korifiv1alpha1.ProcessType{
								{
									Type:    "rake",
									Command: "bundle exec rake",
								},
								{
									Type:    "web",
									Command: "bundle exec rackup config.ru -p $PORT",
								},
							},
							Ports: []int32{1234, 2345},
						}
					})).To(Succeed())
				})

				It("should eventually return a droplet record with fields set to expected values", func() {
					Expect(fetchErr).NotTo(HaveOccurred())

					Expect(dropletRecord.State).To(Equal("STAGED"))
					Expect(dropletRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
					Expect(dropletRecord.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
					Expect(dropletRecord.Stack).To(Equal(build.Status.Droplet.Stack))
					Expect(dropletRecord.Lifecycle.Type).To(Equal(string(build.Spec.Lifecycle.Type)))
					Expect(dropletRecord.Lifecycle.Data.Buildpacks).To(BeEmpty())
					Expect(dropletRecord.Lifecycle.Data.Stack).To(Equal(build.Spec.Lifecycle.Data.Stack))
					Expect(dropletRecord.Image).To(BeEmpty())
					Expect(dropletRecord.Ports).To(ConsistOf(int32(1234), int32(2345)))
					Expect(dropletRecord.AppGUID).To(Equal(build.Spec.AppRef.Name))
					Expect(dropletRecord.PackageGUID).To(Equal(build.Spec.PackageRef.Name))
					Expect(dropletRecord.Labels).To(Equal(map[string]string{
						"key1":                               "val1",
						"key2":                               "val2",
						korifiv1alpha1.CFPackageGUIDLabelKey: packageGUID,
						korifiv1alpha1.CFAppGUIDLabelKey:     appGUID,
						korifiv1alpha1.SpaceGUIDKey:          space.Name,
					}))
					Expect(dropletRecord.Annotations).To(Equal(map[string]string{
						"key1": "val1",
						"key2": "val2",
					}))

					processTypesArray := build.Status.Droplet.ProcessTypes
					for index := range processTypesArray {
						Expect(dropletRecord.ProcessTypes).To(HaveKeyWithValue(processTypesArray[index].Type, processTypesArray[index].Command))
					}

					Expect(dropletRecord.Relationships()).To(Equal(map[string]string{
						"app": build.Spec.AppRef.Name,
					}))
				})

				When("the droplet is of type docker", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, build, func() {
							build.Spec.Lifecycle.Type = "docker"
							build.Status.Droplet.Registry.Image = "some/image"
						})).To(Succeed())
					})

					It("returns a droplet with docker lifecycle", func() {
						Expect(dropletRecord.Lifecycle.Type).To(Equal("docker"))
						Expect(dropletRecord.Lifecycle.Data).To(Equal(repositories.LifecycleData{}))
						Expect(dropletRecord.Image).To(Equal("some/image"))
					})
				})
			})

			When("status.Droplet is not set", func() {
				When("status.Conditions \"Staging\": Unknown, \"Succeeded\": Unknown, is set", func() {
					BeforeEach(func() {
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Staging",
							Status:  metav1.ConditionUnknown,
							Reason:  "kpack",
							Message: "kpack",
						})
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Succeeded",
							Status:  metav1.ConditionUnknown,
							Reason:  "Unknown",
							Message: "Unknown",
						})
						Expect(k8sClient.Status().Update(ctx, build)).To(Succeed())
					})

					It("should return a NotFound error", func() {
						Expect(fetchErr).To(MatchError(apierrors.NewNotFoundError(nil, repositories.DropletResourceType)))
					})
				})

				When("status.Conditions \"Staging\": True, \"Succeeded\": Unknown, is set", func() {
					BeforeEach(func() {
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Staging",
							Status:  metav1.ConditionTrue,
							Reason:  "kpack",
							Message: "kpack",
						})
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Succeeded",
							Status:  metav1.ConditionUnknown,
							Reason:  "Unknown",
							Message: "Unknown",
						})
						Expect(k8sClient.Status().Update(ctx, build)).To(Succeed())
					})

					It("should return a NotFound error", func() {
						Expect(fetchErr).To(MatchError(apierrors.NewNotFoundError(nil, repositories.DropletResourceType)))
					})
				})

				When("status.Conditions \"Staging\": False, \"Succeeded\": False, is set", func() {
					BeforeEach(func() {
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Staging",
							Status:  metav1.ConditionTrue,
							Reason:  "kpack",
							Message: "kpack",
						})
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Succeeded",
							Status:  metav1.ConditionUnknown,
							Reason:  "Unknown",
							Message: "Unknown",
						})
						Expect(k8sClient.Status().Update(ctx, build)).To(Succeed())
					})

					It("should return a NotFound error", func() {
						Expect(fetchErr).To(MatchError(apierrors.NewNotFoundError(nil, repositories.DropletResourceType)))
					})
				})
			})

			When("build does not exist", func() {
				BeforeEach(func() {
					meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
						Type:    "Staging",
						Status:  metav1.ConditionFalse,
						Reason:  "kpack",
						Message: "kpack",
					})
					meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
						Type:    "Succeeded",
						Status:  metav1.ConditionTrue,
						Reason:  "Unknown",
						Message: "Unknown",
					})
					build.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{
						Stack: dropletStack,
						Registry: korifiv1alpha1.Registry{
							Image: registryImage,
							ImagePullSecrets: []corev1.LocalObjectReference{
								{
									Name: registryImageSecret,
								},
							},
						},
						ProcessTypes: []korifiv1alpha1.ProcessType{
							{
								Type:    "rake",
								Command: "bundle exec rake",
							},
							{
								Type:    "web",
								Command: "bundle exec rackup config.ru -p $PORT",
							},
						},
						Ports: []int32{8080, 443},
					}
					// Update Build Status based on changes made to local copy
					Expect(k8sClient.Status().Update(ctx, build)).To(Succeed())
					fetchBuildGUID = "i don't exist"
				})

				It("returns an error", func() {
					Expect(fetchErr).To(HaveOccurred())
					Expect(fetchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})

		When("the user is not authorized to get the droplet", func() {
			It("returns a forbidden error", func() {
				Expect(fetchErr).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("ListDroplets", func() {
		var (
			dropletRecords []repositories.DropletRecord
			listErr        error
			message        repositories.ListDropletsMessage
		)

		BeforeEach(func() {
			message = repositories.ListDropletsMessage{}

			Expect(k8s.Patch(ctx, k8sClient, build, func() {
				build.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{}
			})).To(Succeed())

			anotherBuild := &korifiv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space.Name,
					Labels: map[string]string{
						korifiv1alpha1.CFPackageGUIDLabelKey: uuid.NewString(),
						korifiv1alpha1.CFAppGUIDLabelKey:     uuid.NewString(),
						korifiv1alpha1.SpaceGUIDKey:          space.Name,
					},
				},
				Spec: korifiv1alpha1.CFBuildSpec{
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(k8sClient.Create(ctx, anotherBuild)).To(Succeed())
			Expect(k8s.Patch(ctx, k8sClient, anotherBuild, func() {
				anotherBuild.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{}
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			dropletRecords, listErr = dropletRepo.ListDroplets(ctx, authInfo, message)
		})

		It("returns an empty list to users who lack access", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(dropletRecords).To(BeEmpty())
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns the droplets", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(dropletRecords).To(HaveLen(2))
			})

			Describe("filtering by package guid", func() {
				BeforeEach(func() {
					message = repositories.ListDropletsMessage{
						PackageGUIDs: []string{packageGUID},
					}
				})

				It("returns the builds matching the filter parameters", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(dropletRecords).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
						"PackageGUID": Equal(packageGUID),
					})))
				})
			})

			Describe("filtering by build guid", func() {
				BeforeEach(func() {
					message = repositories.ListDropletsMessage{
						GUIDs: []string{buildGUID},
					}
				})

				It("returns the builds matching the filter parameters", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(dropletRecords).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(buildGUID),
					})))
				})
			})

			Describe("filter parameters to list options", func() {
				var fakeKlient *fake.Klient

				BeforeEach(func() {
					fakeKlient = new(fake.Klient)
					dropletRepo = repositories.NewDropletRepo(fakeKlient)

					message = repositories.ListDropletsMessage{
						PackageGUIDs: []string{"p1", "p2"},
						AppGUIDs:     []string{"a1", "a2"},
						SpaceGUIDs:   []string{"a1", "a2"},
					}
				})

				It("translates filter parameters to klient list options", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(fakeKlient.ListCallCount()).To(Equal(1))
					_, _, listOptions := fakeKlient.ListArgsForCall(0)
					Expect(listOptions).To(ConsistOf(
						repositories.WithLabelIn(korifiv1alpha1.CFPackageGUIDLabelKey, []string{"p1", "p2"}),
						repositories.WithLabelIn(korifiv1alpha1.CFAppGUIDLabelKey, []string{"a1", "a2"}),
						repositories.WithLabelIn(korifiv1alpha1.SpaceGUIDKey, []string{"a1", "a2"}),
					))
				})
			})
		})
	})

	Describe("UpdateDroplet", func() {
		var (
			dropletRecord    repositories.DropletRecord
			updateError      error
			dropletUpdateMsg repositories.UpdateDropletMessage
		)

		BeforeEach(func() {
			dropletUpdateMsg = repositories.UpdateDropletMessage{
				GUID: buildGUID,
				MetadataPatch: repositories.MetadataPatch{
					Labels: map[string]*string{
						"key1": tools.PtrTo("val1edit"),
						"key2": nil,
						"key3": tools.PtrTo("val3"),
					},
					Annotations: map[string]*string{
						"key1": tools.PtrTo("val1edit"),
						"key2": nil,
						"key3": tools.PtrTo("val3"),
					},
				},
			}
		})

		JustBeforeEach(func() {
			dropletRecord, updateError = dropletRepo.UpdateDroplet(ctx, authInfo, dropletUpdateMsg)
		})

		When("the user is authorized to get the droplet", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			When("status.Droplet is set", func() {
				BeforeEach(func() {
					meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
						Type:    "Staging",
						Status:  metav1.ConditionFalse,
						Reason:  "kpack",
						Message: "kpack",
					})
					meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
						Type:    "Succeeded",
						Status:  metav1.ConditionTrue,
						Reason:  "Unknown",
						Message: "Unknown",
					})
					build.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{
						Stack: dropletStack,
						Registry: korifiv1alpha1.Registry{
							Image: registryImage,
							ImagePullSecrets: []corev1.LocalObjectReference{
								{
									Name: registryImageSecret,
								},
							},
						},
						ProcessTypes: []korifiv1alpha1.ProcessType{
							{
								Type:    "rake",
								Command: "bundle exec rake",
							},
							{
								Type:    "web",
								Command: "bundle exec rackup config.ru -p $PORT",
							},
						},
						Ports: []int32{8080, 443},
					}
					// Update Build Status based on changes made to local copy
					Expect(k8sClient.Status().Update(ctx, build)).To(Succeed())
				})

				It("updates the build metadata in kubernetes", func() {
					updatedBuild := new(korifiv1alpha1.CFBuild)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(build), updatedBuild)).To(Succeed())

					Expect(updatedBuild.Labels).To(SatisfyAll(
						HaveKeyWithValue("key1", "val1edit"),
						HaveKeyWithValue("key3", "val3")))
					Expect(updatedBuild.Annotations).To(SatisfyAll(
						HaveKeyWithValue("key1", "val1edit"),
						HaveKeyWithValue("key3", "val3")))
				})

				It("should eventually return a droplet record with fields set to expected values", func() {
					Expect(updateError).NotTo(HaveOccurred())

					Expect(dropletRecord.State).To(Equal("STAGED"))

					By("returning a record with a CreatedAt field from the CR", func() {
						Expect(dropletRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
					})

					By("returning a record with a UpdatedAt field from the CR", func() {
						Expect(dropletRecord.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
					})

					By("returning a record with stack field matching the CR", func() {
						Expect(dropletRecord.Stack).To(Equal(build.Status.Droplet.Stack))
					})

					By("returning a record with Lifecycle fields matching the CR", func() {
						Expect(dropletRecord.Lifecycle.Type).To(Equal(string(build.Spec.Lifecycle.Type)), "returned record lifecycle.type did not match CR")
						Expect(dropletRecord.Lifecycle.Data.Buildpacks).To(BeEmpty(), "returned record lifecycle.data.buildpacks did not match CR")
						Expect(dropletRecord.Lifecycle.Data.Stack).To(Equal(build.Spec.Lifecycle.Data.Stack), "returned record lifecycle.data.stack did not match CR")
					})

					By("returning a record with an AppGUID field matching the CR", func() {
						Expect(dropletRecord.AppGUID).To(Equal(build.Spec.AppRef.Name))
					})

					By("returning a record with a PackageGUID field matching the CR", func() {
						Expect(dropletRecord.PackageGUID).To(Equal(build.Spec.PackageRef.Name))
					})

					By("returning a record with all process types and commands matching the CR", func() {
						processTypesArray := build.Status.Droplet.ProcessTypes
						for index := range processTypesArray {
							Expect(dropletRecord.ProcessTypes).To(HaveKeyWithValue(processTypesArray[index].Type, processTypesArray[index].Command))
						}
					})

					By("returning a record with a GUID matching the build", func() {
						Expect(dropletRecord.GUID).To(Equal(buildGUID))
					})

					By("returns a record with a Label field matching the CR", func() {
						Expect(dropletRecord.Labels).To(Equal(map[string]string{
							"key1":                               "val1edit",
							"key3":                               "val3",
							korifiv1alpha1.CFPackageGUIDLabelKey: packageGUID,
							korifiv1alpha1.CFAppGUIDLabelKey:     appGUID,
							korifiv1alpha1.SpaceGUIDKey:          space.Name,
						}))
					})

					By("returns a record with an Annotation field matching the CR", func() {
						Expect(dropletRecord.Annotations).To(Equal(map[string]string{
							"key1": "val1edit",
							"key3": "val3",
						}))
					})
				})
			})

			When("status.Droplet is not set", func() {
				When("status.Conditions \"Staging\": Unknown, \"Succeeded\": Unknown, is set", func() {
					BeforeEach(func() {
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Staging",
							Status:  metav1.ConditionUnknown,
							Reason:  "kpack",
							Message: "kpack",
						})
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Succeeded",
							Status:  metav1.ConditionUnknown,
							Reason:  "Unknown",
							Message: "Unknown",
						})
						Expect(k8sClient.Status().Update(ctx, build)).To(Succeed())
					})

					It("should return a NotFound error", func() {
						Expect(updateError).To(MatchError(apierrors.NewNotFoundError(nil, repositories.DropletResourceType)))
					})
				})

				When("status.Conditions \"Staging\": True, \"Succeeded\": Unknown, is set", func() {
					BeforeEach(func() {
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Staging",
							Status:  metav1.ConditionTrue,
							Reason:  "kpack",
							Message: "kpack",
						})
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Succeeded",
							Status:  metav1.ConditionUnknown,
							Reason:  "Unknown",
							Message: "Unknown",
						})
						Expect(k8sClient.Status().Update(ctx, build)).To(Succeed())
					})

					It("should return a NotFound error", func() {
						Expect(updateError).To(MatchError(apierrors.NewNotFoundError(nil, repositories.DropletResourceType)))
					})
				})

				When("status.Conditions \"Staging\": False, \"Succeeded\": False, is set", func() {
					BeforeEach(func() {
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Staging",
							Status:  metav1.ConditionTrue,
							Reason:  "kpack",
							Message: "kpack",
						})
						meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
							Type:    "Succeeded",
							Status:  metav1.ConditionUnknown,
							Reason:  "Unknown",
							Message: "Unknown",
						})
						Expect(k8sClient.Status().Update(ctx, build)).To(Succeed())
					})

					It("should return a NotFound error", func() {
						Expect(updateError).To(MatchError(apierrors.NewNotFoundError(nil, repositories.DropletResourceType)))
					})
				})
			})

			When("build does not exist", func() {
				BeforeEach(func() {
					meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
						Type:    "Staging",
						Status:  metav1.ConditionFalse,
						Reason:  "kpack",
						Message: "kpack",
					})
					meta.SetStatusCondition(&build.Status.Conditions, metav1.Condition{
						Type:    "Succeeded",
						Status:  metav1.ConditionTrue,
						Reason:  "Unknown",
						Message: "Unknown",
					})
					build.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{
						Stack: dropletStack,
						Registry: korifiv1alpha1.Registry{
							Image: registryImage,
							ImagePullSecrets: []corev1.LocalObjectReference{
								{
									Name: registryImageSecret,
								},
							},
						},
						ProcessTypes: []korifiv1alpha1.ProcessType{
							{
								Type:    "rake",
								Command: "bundle exec rake",
							},
							{
								Type:    "web",
								Command: "bundle exec rackup config.ru -p $PORT",
							},
						},
						Ports: []int32{8080, 443},
					}
					// Update Build Status based on changes made to local copy
					Expect(k8sClient.Status().Update(ctx, build)).To(Succeed())
					dropletUpdateMsg.GUID = "i don't exist"
				})

				It("returns an error", func() {
					Expect(updateError).To(HaveOccurred())
					Expect(updateError).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})

		When("the user is not authorized to get the droplet", func() {
			It("returns a forbidden error", func() {
				Expect(updateError).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})
})
