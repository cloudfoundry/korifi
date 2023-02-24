package repositories_test

import (
	"context"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		testCtx     context.Context
		dropletRepo *repositories.DropletRepo
		org         *korifiv1alpha1.CFOrg
		space       *korifiv1alpha1.CFSpace
		build       *korifiv1alpha1.CFBuild
		packageGUID string
		buildGUID   string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		orgName := prefixedGUID("org-")
		spaceName := prefixedGUID("space-")
		packageGUID = prefixedGUID("package-")
		buildGUID = prefixedGUID("build-")
		org = createOrgWithCleanup(testCtx, orgName)
		space = createSpaceWithCleanup(testCtx, org.Name, spaceName)

		dropletRepo = repositories.NewDropletRepo(userClientFactory, namespaceRetriever, nsPerms)

		build = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Name:      buildGUID,
				Namespace: space.Name,
				Labels: map[string]string{
					"key1": "val1",
					"key2": "val2",
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
					Data: korifiv1alpha1.LifecycleData{
						Buildpacks: []string{},
						Stack:      "",
					},
				},
			},
		}
		Expect(k8sClient.Create(testCtx, build)).To(Succeed())
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
			dropletRecord, fetchErr = dropletRepo.GetDroplet(testCtx, authInfo, fetchBuildGUID)
		})

		When("the user is authorized to get the droplet", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
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
					Expect(k8sClient.Status().Update(testCtx, build)).To(Succeed())
				})

				It("should eventually return a droplet record with fields set to expected values", func() {
					Expect(fetchErr).NotTo(HaveOccurred())

					Expect(dropletRecord.State).To(Equal("STAGED"))

					By("returning a record with a CreatedAt field from the CR", func() {
						createdAt, err := time.Parse(time.RFC3339, dropletRecord.CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
					})

					By("returning a record with a UpdatedAt field from the CR", func() {
						updatedAt, err := time.Parse(time.RFC3339, dropletRecord.UpdatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
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

					By("returns a record with a Label field matching the CR", func() {
						Expect(dropletRecord.Labels).To(Equal(map[string]string{
							"key1": "val1",
							"key2": "val2",
						}))
					})

					By("returns a record with an Annotation field matching the CR", func() {
						Expect(dropletRecord.Annotations).To(Equal(map[string]string{
							"key1": "val1",
							"key2": "val2",
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
						Expect(k8sClient.Status().Update(testCtx, build)).To(Succeed())
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
						Expect(k8sClient.Status().Update(testCtx, build)).To(Succeed())
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
						Expect(k8sClient.Status().Update(testCtx, build)).To(Succeed())
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
					Expect(k8sClient.Status().Update(testCtx, build)).To(Succeed())
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
		)

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
			Expect(k8sClient.Status().Update(testCtx, build)).To(Succeed())
		})

		JustBeforeEach(func() {
			dropletRecords, listErr = dropletRepo.ListDroplets(testCtx, authInfo, repositories.ListDropletsMessage{
				PackageGUIDs: []string{packageGUID},
			})
		})

		When("the user is not authorized to list the droplet", func() {
			It("returns an empty list to users who lack access", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(dropletRecords).To(BeEmpty())
			})
		})

		When("the user is a space manager", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns a list of droplet records with the packageGUID label set on them", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(dropletRecords).To(HaveLen(1))
				Expect(dropletRecords[0].GUID).To(Equal(build.Name))
			})

			When("a space exists with a rolebinding for the user, but without permission to list droplets", func() {
				BeforeEach(func() {
					anotherSpace := createSpaceWithCleanup(testCtx, org.Name, "space-without-droplet-space-perm")
					createRoleBinding(testCtx, userName, rootNamespaceUserRole.Name, anotherSpace.Name)
				})

				It("returns the droplet", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(dropletRecords).To(HaveLen(1))
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
						"key1": pointerTo("val1edit"),
						"key2": nil,
						"key3": pointerTo("val3"),
					},
					Annotations: map[string]*string{
						"key1": pointerTo("val1edit"),
						"key2": nil,
						"key3": pointerTo("val3"),
					},
				},
			}
		})

		JustBeforeEach(func() {
			dropletRecord, updateError = dropletRepo.UpdateDroplet(ctx, authInfo, dropletUpdateMsg)
		})

		When("the user is authorized to get the droplet", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
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
					Expect(k8sClient.Status().Update(testCtx, build)).To(Succeed())
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
						createdAt, err := time.Parse(time.RFC3339, dropletRecord.CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
					})

					By("returning a record with a UpdatedAt field from the CR", func() {
						updatedAt, err := time.Parse(time.RFC3339, dropletRecord.UpdatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
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
							"key1": "val1edit",
							"key3": "val3",
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
						Expect(k8sClient.Status().Update(testCtx, build)).To(Succeed())
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
						Expect(k8sClient.Status().Update(testCtx, build)).To(Succeed())
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
						Expect(k8sClient.Status().Update(testCtx, build)).To(Succeed())
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
					Expect(k8sClient.Status().Update(testCtx, build)).To(Succeed())
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
