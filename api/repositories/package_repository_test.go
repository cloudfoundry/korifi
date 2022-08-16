package repositories_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("PackageRepository", func() {
	const appGUID = "the-app-guid"
	const appUID = "the-app-uid"

	var (
		packageRepo *repositories.PackageRepo
		org         *korifiv1alpha1.CFOrg
		space       *korifiv1alpha1.CFSpace
	)

	BeforeEach(func() {
		packageRepo = repositories.NewPackageRepo(userClientFactory, namespaceRetriever, nsPerms)
		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))
	})

	Describe("CreatePackage", func() {
		var (
			packageCreate  repositories.CreatePackageMessage
			createdPackage repositories.PackageRecord
			createErr      error
		)

		BeforeEach(func() {
			packageCreate = repositories.CreatePackageMessage{
				Type:      "bits",
				AppGUID:   appGUID,
				SpaceGUID: space.Name,
				OwnerRef: metav1.OwnerReference{
					APIVersion: "korifi.cloudfoundry.org/v1alpha1",
					Kind:       "CFApp",
					Name:       appGUID,
					UID:        appUID,
				},
			}
		})

		JustBeforeEach(func() {
			createdPackage, createErr = packageRepo.CreatePackage(ctx, authInfo, packageCreate)
		})

		It("fails because the user is not a space developer", func() {
			Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a SpaceDeveloper", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates a Package record", func() {
				Expect(createErr).NotTo(HaveOccurred())

				packageGUID := createdPackage.GUID
				Expect(packageGUID).NotTo(BeEmpty())
				Expect(createdPackage.Type).To(Equal("bits"))
				Expect(createdPackage.AppGUID).To(Equal(appGUID))
				Expect(createdPackage.State).To(Equal("AWAITING_UPLOAD"))

				createdAt, err := time.Parse(time.RFC3339, createdPackage.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

				updatedAt, err := time.Parse(time.RFC3339, createdPackage.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

				packageNSName := types.NamespacedName{Name: packageGUID, Namespace: space.Name}
				createdCFPackage := new(korifiv1alpha1.CFPackage)
				Expect(k8sClient.Get(ctx, packageNSName, createdCFPackage)).To(Succeed())

				Expect(createdCFPackage.Name).To(Equal(packageGUID))
				Expect(createdCFPackage.Namespace).To(Equal(space.Name))
				Expect(createdCFPackage.Spec.Type).To(Equal(korifiv1alpha1.PackageType("bits")))
				Expect(createdCFPackage.Spec.AppRef.Name).To(Equal(appGUID))
				Expect(createdCFPackage.ObjectMeta.OwnerReferences).To(Equal(
					[]metav1.OwnerReference{
						{
							APIVersion: "korifi.cloudfoundry.org/v1alpha1",
							Kind:       "CFApp",
							Name:       appGUID,
							UID:        appUID,
						},
					}))
			})
		})
	})

	Describe("GetPackage", func() {
		var (
			packageGUID   string
			packageRecord repositories.PackageRecord
			getErr        error
		)

		BeforeEach(func() {
			packageGUID = generateGUID()
			createPackageCR(ctx, k8sClient, packageGUID, appGUID, space.Name, "")
		})

		JustBeforeEach(func() {
			packageRecord, getErr = packageRepo.GetPackage(ctx, authInfo, packageGUID)
		})

		When("the user is authorized in the namespace", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("can fetch the PackageRecord we're looking for", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(packageRecord.GUID).To(Equal(packageGUID))
				Expect(packageRecord.Type).To(Equal("bits"))
				Expect(packageRecord.AppGUID).To(Equal(appGUID))
				Expect(packageRecord.State).To(Equal("AWAITING_UPLOAD"))

				createdAt, err := time.Parse(time.RFC3339, packageRecord.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

				updatedAt, err := time.Parse(time.RFC3339, packageRecord.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
			})

			Describe("State field", func() {
				It("equals AWAITING_UPLOAD by default", func() {
					Expect(packageRecord.State).To(Equal("AWAITING_UPLOAD"))
				})

				When("an source image is set", func() {
					BeforeEach(func() {
						packageGUID = generateGUID()
						createPackageCR(ctx, k8sClient, packageGUID, appGUID, space.Name, "some-org/some-repo")
					})

					It("equals READY", func() {
						Expect(packageRecord.State).To(Equal("READY"))
					})
				})
			})
		})

		When("user is not authorized to get a package", func() {
			It("returns a forbidden error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("duplicate Packages exist across namespaces with the same GUID", func() {
			BeforeEach(func() {
				anotherSpace := createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))
				createPackageCR(ctx, k8sClient, packageGUID, appGUID, anotherSpace.Name, "")
			})

			It("returns an error", func() {
				Expect(getErr).To(MatchError("get-package duplicate records exist"))
			})
		})

		When("no packages exist", func() {
			BeforeEach(func() {
				packageGUID = "i don't exist"
			})

			It("returns an error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("ListPackages", func() {
		const (
			appGUID2 = "the-app-guid-2"
		)

		var (
			space2      *korifiv1alpha1.CFSpace
			packageList []repositories.PackageRecord
			listMessage repositories.ListPackagesMessage
		)

		BeforeEach(func() {
			space2 = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space2"))
			listMessage = repositories.ListPackagesMessage{}
		})

		JustBeforeEach(func() {
			var err error
			packageList, err = packageRepo.ListPackages(context.Background(), authInfo, listMessage)
			Expect(err).NotTo(HaveOccurred())
		})

		When("there are packages in multiple namespaces", func() {
			var (
				package1GUID, package2GUID, noPermissionsPackageGUID string
				noPermissionsSpace                                   *korifiv1alpha1.CFSpace
			)

			BeforeEach(func() {
				package1GUID = generateGUID()
				createPackageCR(ctx, k8sClient, package1GUID, appGUID, space.Name, "")

				// add a small delay to test ordering on created_by
				time.Sleep(100 * time.Millisecond)

				package2GUID = generateGUID()
				createPackageCR(ctx, k8sClient, package2GUID, appGUID2, space2.Name, "my-image-url")

				noPermissionsSpace = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("no-permissions-space"))
				noPermissionsPackageGUID = prefixedGUID("no-permissions-package")
				createPackageCR(ctx, k8sClient, noPermissionsPackageGUID, appGUID2, noPermissionsSpace.Name, "")
			})

			When("the user is allowed to list packages in some namespaces", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
					createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space2.Name)
				})

				It("returns all Packages in spaces where the user has access", func() {
					Expect(packageList).To(ContainElements(
						MatchFields(IgnoreExtras, Fields{
							"GUID":    Equal(package1GUID),
							"AppGUID": Equal(appGUID),
						}),
						MatchFields(IgnoreExtras, Fields{
							"GUID":    Equal(package2GUID),
							"AppGUID": Equal(appGUID2),
						}),
					))
					Expect(packageList).ToNot(ContainElement(
						MatchFields(IgnoreExtras, Fields{
							"GUID": Equal(noPermissionsPackageGUID),
						}),
					))
				})

				It("orders the results in ascending created_at order by default", func() {
					Expect(packageList).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"GUID": Equal(package1GUID),
						}),
						MatchFields(IgnoreExtras, Fields{
							"GUID": Equal(package2GUID),
						}),
					))

					firstCreatedAt, err := time.Parse(time.RFC3339, packageList[0].CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					secondCreatedAt, err := time.Parse(time.RFC3339, packageList[1].CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(firstCreatedAt).To(BeTemporally("<=", secondCreatedAt))
				})

				When("app_guids filter is provided", func() {
					BeforeEach(func() {
						listMessage = repositories.ListPackagesMessage{AppGUIDs: []string{appGUID}}
					})

					It("fetches all PackageRecords for that app", func() {
						for _, packageRecord := range packageList {
							Expect(packageRecord).To(
								MatchFields(IgnoreExtras, Fields{
									"AppGUID": Equal(appGUID),
								}),
							)
						}
					})
				})

				When("SortBy is provided and value is created_at", func() {
					When("descending order is false", func() {
						BeforeEach(func() {
							listMessage = repositories.ListPackagesMessage{SortBy: "created_at", DescendingOrder: false}
						})

						It("fetches packages sorted by created_at in ascending order", func() {
							Expect(packageList).To(ConsistOf(
								MatchFields(IgnoreExtras, Fields{
									"GUID":    Equal(package1GUID),
									"AppGUID": Equal(appGUID),
								}),
								MatchFields(IgnoreExtras, Fields{
									"GUID":    Equal(package2GUID),
									"AppGUID": Equal(appGUID2),
								}),
							))

							firstCreatedAt, err := time.Parse(time.RFC3339, packageList[0].CreatedAt)
							Expect(err).NotTo(HaveOccurred())
							secondCreatedAt, err := time.Parse(time.RFC3339, packageList[1].CreatedAt)
							Expect(err).NotTo(HaveOccurred())
							Expect(firstCreatedAt).To(BeTemporally("<=", secondCreatedAt))
						})
					})

					When("descending order is true", func() {
						BeforeEach(func() {
							listMessage = repositories.ListPackagesMessage{SortBy: "created_at", DescendingOrder: true}
						})

						It("fetches packages sorted by created_at in descending order", func() {
							Expect(packageList).To(ContainElements(
								MatchFields(IgnoreExtras, Fields{
									"GUID":    Equal(package1GUID),
									"AppGUID": Equal(appGUID),
								}),
								MatchFields(IgnoreExtras, Fields{
									"GUID":    Equal(package2GUID),
									"AppGUID": Equal(appGUID2),
								}),
							))

							firstCreatedAt, err := time.Parse(time.RFC3339, packageList[0].CreatedAt)
							Expect(err).NotTo(HaveOccurred())
							secondCreatedAt, err := time.Parse(time.RFC3339, packageList[1].CreatedAt)
							Expect(err).NotTo(HaveOccurred())
							Expect(firstCreatedAt).To(BeTemporally(">=", secondCreatedAt))
						})
					})
				})

				When("State filter is provided", func() {
					When("filtering by State=READY", func() {
						BeforeEach(func() {
							listMessage = repositories.ListPackagesMessage{States: []string{"READY"}}
						})

						It("filters the packages", func() {
							for _, packageRecord := range packageList {
								Expect(packageRecord).To(
									MatchFields(IgnoreExtras, Fields{
										"State": Equal("READY"),
									}),
								)
							}
						})
					})

					When("filtering by State=AWAITING_UPLOAD", func() {
						BeforeEach(func() {
							listMessage = repositories.ListPackagesMessage{States: []string{"AWAITING_UPLOAD"}}
						})

						It("filters the packages", func() {
							for _, packageRecord := range packageList {
								Expect(packageRecord).To(
									MatchFields(IgnoreExtras, Fields{
										"State": Equal("AWAITING_UPLOAD"),
									}),
								)
							}
						})
					})

					When("filtering by State=AWAITING_UPLOAD,READY", func() {
						BeforeEach(func() {
							listMessage = repositories.ListPackagesMessage{States: []string{"AWAITING_UPLOAD", "READY"}}
						})

						It("filters the packages", func() {
							Expect(packageList).To(ContainElements(
								MatchFields(IgnoreExtras, Fields{
									"GUID":    Equal(package1GUID),
									"AppGUID": Equal(appGUID),
									"State":   Equal("AWAITING_UPLOAD"),
								}),
								MatchFields(IgnoreExtras, Fields{
									"GUID":    Equal(package2GUID),
									"AppGUID": Equal(appGUID2),
									"State":   Equal("READY"),
								}),
							))
						})
					})
				})
			})

			When("the user is not allowed to list packages in namespaces with packages", func() {
				It("returns an empty list of PackageRecords", func() {
					Expect(packageList).To(BeEmpty())
				})
			})
		})

		When("there are no packages in allowed namespaces", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space2.Name)
			})

			It("returns an empty list of PackageRecords", func() {
				Expect(packageList).To(BeEmpty())
			})
		})
	})

	Describe("UpdatePackageSource", func() {
		var (
			existingCFPackage     korifiv1alpha1.CFPackage
			returnedPackageRecord repositories.PackageRecord
			updateErr             error
		)

		const (
			packageGUID               = "the-package-guid"
			packageSourceImageRef     = "my-org/" + packageGUID
			packageRegistrySecretName = "image-pull-secret"
		)

		BeforeEach(func() {
			existingCFPackage = korifiv1alpha1.CFPackage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFPackage",
					APIVersion: korifiv1alpha1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      packageGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFPackageSpec{
					Type:   "bits",
					AppRef: corev1.LocalObjectReference{Name: appGUID},
				},
			}

			Expect(
				k8sClient.Create(ctx, &existingCFPackage),
			).To(Succeed())
		})

		JustBeforeEach(func() {
			updateMessage := repositories.UpdatePackageSourceMessage{
				GUID:               packageGUID,
				SpaceGUID:          space.Name,
				ImageRef:           packageSourceImageRef,
				RegistrySecretName: packageRegistrySecretName,
			}
			returnedPackageRecord, updateErr = packageRepo.UpdatePackageSource(ctx, authInfo, updateMessage)
		})

		When("the user is authorized", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns an updated record", func() {
				Expect(updateErr).NotTo(HaveOccurred())

				Expect(returnedPackageRecord.GUID).To(Equal(existingCFPackage.Name))
				Expect(returnedPackageRecord.Type).To(Equal(string(existingCFPackage.Spec.Type)))
				Expect(returnedPackageRecord.AppGUID).To(Equal(existingCFPackage.Spec.AppRef.Name))
				Expect(returnedPackageRecord.SpaceGUID).To(Equal(existingCFPackage.Namespace))
				Expect(returnedPackageRecord.State).To(Equal("READY"))

				createdAt, err := time.Parse(time.RFC3339, returnedPackageRecord.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

				updatedAt, err := time.Parse(time.RFC3339, returnedPackageRecord.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
			})

			It("updates only the Registry field of the existing CFPackage", func() {
				packageNSName := types.NamespacedName{Name: packageGUID, Namespace: space.Name}
				updatedCFPackage := new(korifiv1alpha1.CFPackage)
				Expect(k8sClient.Get(ctx, packageNSName, updatedCFPackage)).To(Succeed())

				Expect(updatedCFPackage.Name).To(Equal(existingCFPackage.Name))
				Expect(updatedCFPackage.Namespace).To(Equal(existingCFPackage.Namespace))
				Expect(updatedCFPackage.Spec.Type).To(Equal(existingCFPackage.Spec.Type))
				Expect(updatedCFPackage.Spec.AppRef).To(Equal(existingCFPackage.Spec.AppRef))
				Expect(updatedCFPackage.Spec.Source.Registry).To(Equal(korifiv1alpha1.Registry{
					Image:            packageSourceImageRef,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: packageRegistrySecretName}},
				}))
			})
		})

		When("user is not authorized to update a package", func() {
			It("returns a forbidden error", func() {
				Expect(updateErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})
})
