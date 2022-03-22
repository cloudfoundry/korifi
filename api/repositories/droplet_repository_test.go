package repositories_test

import (
	"context"
	"time"

	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("DropletRepository",  func() {
	var (
		testCtx     context.Context
		dropletRepo *repositories.DropletRepo
		space       *v1alpha2.SubnamespaceAnchor
	)

	BeforeEach(func() {
		testCtx = context.Background()
		orgName := prefixedGUID("org-")
		spaceName := prefixedGUID("space-")
		org := createOrgAnchorAndNamespace(testCtx, rootNamespace, orgName)
		space = createSpaceAnchorAndNamespace(testCtx, org.Name, spaceName)

		dropletRepo = repositories.NewDropletRepo(k8sClient, namespaceRetriever, userClientFactory, nsPerms)
	})

	Describe("GetDroplet", func() {
		var (
			buildGUID string
			build     *workloadsv1alpha1.CFBuild

			dropletRecord repositories.DropletRecord
			fetchErr      error
		)

		const (
			appGUID             = "app-1-guid"
			packageGUID         = "package-1-guid"
			stagingMemory       = 1024
			stagingDisk         = 2048
			dropletStack        = "cflinuxfs3"
			registryImage       = "registry/image:tag"
			registryImageSecret = "secret-key"
		)

		BeforeEach(func() {
			buildGUID = generateGUID()
			build = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildGUID,
					Namespace: space.Name,
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
			Expect(k8sClient.Create(testCtx, build)).To(Succeed())
		})

		JustBeforeEach(func() {
			dropletRecord, fetchErr = dropletRepo.GetDroplet(testCtx, authInfo, buildGUID)
		})

		When("the user is authorized to get the droplet", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			When("status.BuildDropletStatus is set", func() {
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
					build.Status.BuildDropletStatus = &workloadsv1alpha1.BuildDropletStatus{
						Stack: dropletStack,
						Registry: workloadsv1alpha1.Registry{
							Image: registryImage,
							ImagePullSecrets: []corev1.LocalObjectReference{
								{
									Name: registryImageSecret,
								},
							},
						},
						ProcessTypes: []workloadsv1alpha1.ProcessType{
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
						Expect(dropletRecord.Stack).To(Equal(build.Status.BuildDropletStatus.Stack))
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
						processTypesArray := build.Status.BuildDropletStatus.ProcessTypes
						for index := range processTypesArray {
							Expect(dropletRecord.ProcessTypes).To(HaveKeyWithValue(processTypesArray[index].Type, processTypesArray[index].Command))
						}
					})
				})
			})

			When("status.BuildDropletStatus is not set", func() {
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
				It("returns an error", func() {
					_, err := dropletRepo.GetDroplet(testCtx, authInfo, "i don't exist")
					Expect(err).To(HaveOccurred())
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})

		When("the user is not authorized to get the droplet", func() {
			It("returns a forbidden error", func() {
				_, err := dropletRepo.GetDroplet(testCtx, authInfo, buildGUID)
				Expect(err).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("ListDroplets", func() {
		var (
			build       *workloadsv1alpha1.CFBuild
			packageGUID string
		)

		const (
			appGUID             = "app-1-guid"
			stagingMemory       = 1024
			stagingDisk         = 2048
			dropletStack        = "cflinuxfs3"
			registryImage       = "registry/image:tag"
			registryImageSecret = "secret-key"
		)

		BeforeEach(func() {
			packageGUID = prefixedGUID("package-")
			build = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prefixedGUID("build-"),
					Namespace: space.Name,
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
			Expect(k8sClient.Create(testCtx, build)).To(Succeed())
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
			build.Status.BuildDropletStatus = &workloadsv1alpha1.BuildDropletStatus{
				Stack: dropletStack,
				Registry: workloadsv1alpha1.Registry{
					Image: registryImage,
					ImagePullSecrets: []corev1.LocalObjectReference{
						{
							Name: registryImageSecret,
						},
					},
				},
				ProcessTypes: []workloadsv1alpha1.ProcessType{
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

		When("the packageGUIDs message parameter is provided",  func() {
			var dropletRecords []repositories.DropletRecord

			JustBeforeEach(func() {
				var err error
				dropletRecords, err = dropletRepo.ListDroplets(testCtx, authInfo, repositories.ListDropletsMessage{
					PackageGUIDs: []string{packageGUID},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an empty list to users who lack access", func() {
				Expect(dropletRecords).To(BeEmpty())
			})

			When("the user is a space manager", func() {
				BeforeEach(func() {
					createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
				})

				It("returns a list of droplet records with the packageGUID label set on them", func() {
					Expect(dropletRecords).To(HaveLen(1))
					Expect(dropletRecords[0].GUID).To(Equal(build.Name))
				})
			})
		})
	})
})
