package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/cleanup"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/image"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

var _ = Describe("PackageRepository", func() {
	var (
		repoCreator *fake.RepositoryCreator
		packageRepo *repositories.PackageRepo
		org         *korifiv1alpha1.CFOrg
		space       *korifiv1alpha1.CFSpace
		app         *korifiv1alpha1.CFApp
		stopManager context.CancelFunc
	)

	BeforeEach(func() {
		repoCreator = new(fake.RepositoryCreator)
		packageRepo = repositories.NewPackageRepo(
			userClientFactory,
			namespaceRetriever,
			nsPerms,
			repoCreator,
			"container.registry/foo/my/prefix-",
			time.Second*4,
		)
		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))
		app = createApp(space.Name)

		k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "controllers", "role.yaml"))

		k8sInterface, err := kubernetes.NewForConfig(k8sManager.GetConfig())
		Expect(err).NotTo(HaveOccurred())

		err = (workloads.NewCFPackageReconciler(
			k8sManager.GetClient(),
			k8sManager.GetScheme(),
			ctrl.Log.WithName("controllers").WithName("CFPackage"),
			image.NewClient(k8sInterface),
			cleanup.NewPackageCleaner(k8sClient, 5),
			[]string{"package-repo-secret-name"},
		)).SetupWithManager(k8sManager)
		Expect(err).NotTo(HaveOccurred())

		stopManager = helpers.StartK8sManager(k8sManager)
	})

	AfterEach(func() {
		stopManager()
	})

	Describe("CreatePackage", func() {
		var (
			packageCreate  repositories.CreatePackageMessage
			createdPackage repositories.PackageRecord
			createErr      error
		)

		BeforeEach(func() {
			packageCreate = repositories.CreatePackageMessage{
				AppGUID:   app.Name,
				SpaceGUID: space.Name,
				Metadata: repositories.Metadata{
					Labels: map[string]string{
						"bob": "foo",
					},
					Annotations: map[string]string{
						"jim": "bar",
					},
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

			Describe("bits package", func() {
				BeforeEach(func() {
					packageCreate.Type = "bits"
				})

				It("creates a Package record", func() {
					Expect(createErr).NotTo(HaveOccurred())

					packageGUID := createdPackage.GUID
					Expect(packageGUID).NotTo(BeEmpty())
					Expect(createdPackage.Type).To(Equal("bits"))
					Expect(createdPackage.AppGUID).To(Equal(app.Name))
					Expect(createdPackage.State).To(Equal("AWAITING_UPLOAD"))
					Expect(createdPackage.Labels).To(HaveKeyWithValue("bob", "foo"))
					Expect(createdPackage.Annotations).To(HaveKeyWithValue("jim", "bar"))
					Expect(createdPackage.ImageRef).To(Equal(fmt.Sprintf("container.registry/foo/my/prefix-%s-packages", app.Name)))

					Expect(createdPackage.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))

					Expect(createdPackage.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))

					packageNSName := types.NamespacedName{Name: packageGUID, Namespace: space.Name}
					createdCFPackage := new(korifiv1alpha1.CFPackage)
					Expect(k8sClient.Get(ctx, packageNSName, createdCFPackage)).To(Succeed())

					Expect(createdCFPackage.Name).To(Equal(packageGUID))
					Expect(createdCFPackage.Namespace).To(Equal(space.Name))
					Expect(createdCFPackage.Spec.Type).To(Equal(korifiv1alpha1.PackageType("bits")))
					Expect(createdCFPackage.Spec.AppRef.Name).To(Equal(app.Name))

					Expect(createdCFPackage.Labels).To(HaveKeyWithValue("bob", "foo"))
					Expect(createdCFPackage.Annotations).To(HaveKeyWithValue("jim", "bar"))

					Expect(meta.IsStatusConditionTrue(createdCFPackage.Status.Conditions, "Initialized")).To(BeTrue())
				})

				It("creates a package repository", func() {
					Expect(repoCreator.CreateRepositoryCallCount()).To(Equal(1))
					_, repoName := repoCreator.CreateRepositoryArgsForCall(0)
					Expect(repoName).To(Equal("container.registry/foo/my/prefix-" + app.Name + "-packages"))
				})

				When("repo creation errors", func() {
					BeforeEach(func() {
						repoCreator.CreateRepositoryReturns(errors.New("repo create error"))
					})

					It("returns an error", func() {
						Expect(createErr).To(MatchError(ContainSubstring("repo create error")))
					})
				})
			})

			Describe("docker package", func() {
				BeforeEach(func() {
					packageCreate.Type = "docker"
					packageCreate.Data = &repositories.PackageData{
						Image: "some/image",
					}
				})

				It("creates a Package record", func() {
					Expect(createErr).NotTo(HaveOccurred())

					packageGUID := createdPackage.GUID
					Expect(packageGUID).NotTo(BeEmpty())
					Expect(createdPackage.Type).To(Equal("docker"))
					Expect(createdPackage.AppGUID).To(Equal(app.Name))
					Expect(createdPackage.State).To(Equal("READY"))
					Expect(createdPackage.Labels).To(HaveKeyWithValue("bob", "foo"))
					Expect(createdPackage.Annotations).To(HaveKeyWithValue("jim", "bar"))
					Expect(createdPackage.ImageRef).To(Equal("some/image"))

					Expect(createdPackage.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
					Expect(createdPackage.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))

					packageNSName := types.NamespacedName{Name: packageGUID, Namespace: space.Name}
					createdCFPackage := new(korifiv1alpha1.CFPackage)
					Expect(k8sClient.Get(ctx, packageNSName, createdCFPackage)).To(Succeed())

					Expect(createdCFPackage.Name).To(Equal(packageGUID))
					Expect(createdCFPackage.Namespace).To(Equal(space.Name))
					Expect(createdCFPackage.Spec.Type).To(Equal(korifiv1alpha1.PackageType("docker")))
					Expect(createdCFPackage.Spec.AppRef.Name).To(Equal(app.Name))
					Expect(createdCFPackage.Spec.Source.Registry.Image).To(Equal("some/image"))

					Expect(createdCFPackage.Labels).To(HaveKeyWithValue("bob", "foo"))
					Expect(createdCFPackage.Annotations).To(HaveKeyWithValue("jim", "bar"))

					Expect(meta.IsStatusConditionTrue(createdCFPackage.Status.Conditions, "Initialized")).To(BeTrue())
					Expect(meta.IsStatusConditionTrue(createdCFPackage.Status.Conditions, "Ready")).To(BeTrue())
				})

				It("does not create repository", func() {
					Expect(repoCreator.CreateRepositoryCallCount()).To(BeZero())
				})
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
			createPackageCR(ctx, k8sClient, packageGUID, app.Name, space.Name, "")
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
				Expect(packageRecord.AppGUID).To(Equal(app.Name))
				Expect(packageRecord.State).To(Equal("AWAITING_UPLOAD"))
				Expect(packageRecord.Labels).To(HaveKeyWithValue("foo", "the-original-value"))
				Expect(packageRecord.Annotations).To(HaveKeyWithValue("bar", "the-original-value"))
				Expect(packageRecord.ImageRef).To(Equal(fmt.Sprintf("container.registry/foo/my/prefix-%s-packages", app.Name)))

				Expect(packageRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(packageRecord.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
			})

			Describe("State field", func() {
				It("equals AWAITING_UPLOAD by default", func() {
					Expect(packageRecord.State).To(Equal("AWAITING_UPLOAD"))
				})

				When("the package is ready", func() {
					BeforeEach(func() {
						packageGUID = generateGUID()
						createPackageCR(ctx, k8sClient, packageGUID, app.Name, space.Name, "some-org/some-repo")
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
				createPackageCR(ctx, k8sClient, packageGUID, app.Name, anotherSpace.Name, "")
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
		var (
			app2        *korifiv1alpha1.CFApp
			space2      *korifiv1alpha1.CFSpace
			packageList []repositories.PackageRecord
			listMessage repositories.ListPackagesMessage
		)

		BeforeEach(func() {
			space2 = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space2"))
			app2 = createApp(space2.Name)
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
				createPackageCR(ctx, k8sClient, package1GUID, app.Name, space.Name, "")

				package2GUID = generateGUID()
				createPackageCR(ctx, k8sClient, package2GUID, app2.Name, space2.Name, "my-image-url")

				noPermissionsSpace = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("no-permissions-space"))
				noPermissionsPackageGUID = prefixedGUID("no-permissions-package")
				createPackageCR(ctx, k8sClient, noPermissionsPackageGUID, app2.Name, noPermissionsSpace.Name, "")
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
							"AppGUID": Equal(app.Name),
						}),
						MatchFields(IgnoreExtras, Fields{
							"GUID":    Equal(package2GUID),
							"AppGUID": Equal(app2.Name),
						}),
					))
					Expect(packageList).ToNot(ContainElement(
						MatchFields(IgnoreExtras, Fields{
							"GUID": Equal(noPermissionsPackageGUID),
						}),
					))
				})

				When("app_guids filter is provided", func() {
					BeforeEach(func() {
						listMessage = repositories.ListPackagesMessage{AppGUIDs: []string{app.Name}}
					})

					It("fetches all PackageRecords for that app", func() {
						for _, packageRecord := range packageList {
							Expect(packageRecord).To(
								MatchFields(IgnoreExtras, Fields{
									"AppGUID": Equal(app.Name),
								}),
							)
						}
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
									"AppGUID": Equal(app.Name),
									"State":   Equal("AWAITING_UPLOAD"),
								}),
								MatchFields(IgnoreExtras, Fields{
									"GUID":    Equal(package2GUID),
									"AppGUID": Equal(app2.Name),
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
			existingCFPackage     *korifiv1alpha1.CFPackage
			updatedCFPackage      *korifiv1alpha1.CFPackage
			returnedPackageRecord repositories.PackageRecord
			updateMessage         repositories.UpdatePackageSourceMessage
			updateErr             error
		)

		const (
			packageGUID           = "the-package-guid"
			packageSourceImageRef = "my-org/" + packageGUID
		)

		BeforeEach(func() {
			existingCFPackage = &korifiv1alpha1.CFPackage{
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
					AppRef: corev1.LocalObjectReference{Name: app.Name},
				},
			}

			updateMessage = repositories.UpdatePackageSourceMessage{
				GUID:                packageGUID,
				SpaceGUID:           space.Name,
				ImageRef:            packageSourceImageRef,
				RegistrySecretNames: []string{"image-pull-secret"},
			}
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, existingCFPackage)).To(Succeed())

			returnedPackageRecord, updateErr = packageRepo.UpdatePackageSource(ctx, authInfo, updateMessage)
			updatedCFPackage = &korifiv1alpha1.CFPackage{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(existingCFPackage), updatedCFPackage)).To(Succeed())
		})

		When("the user is authorized", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("updates the CFPackage and returns an updated record", func() {
				Expect(updateErr).NotTo(HaveOccurred())

				Expect(returnedPackageRecord.GUID).To(Equal(existingCFPackage.Name))
				Expect(returnedPackageRecord.Type).To(Equal(string(existingCFPackage.Spec.Type)))
				Expect(returnedPackageRecord.AppGUID).To(Equal(existingCFPackage.Spec.AppRef.Name))
				Expect(returnedPackageRecord.SpaceGUID).To(Equal(existingCFPackage.Namespace))
				Expect(returnedPackageRecord.State).To(Equal("READY"))

				Expect(returnedPackageRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(returnedPackageRecord.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))

				Expect(updatedCFPackage.Name).To(Equal(existingCFPackage.Name))
				Expect(updatedCFPackage.Namespace).To(Equal(existingCFPackage.Namespace))
				Expect(updatedCFPackage.Spec.Type).To(Equal(existingCFPackage.Spec.Type))
				Expect(updatedCFPackage.Spec.AppRef).To(Equal(existingCFPackage.Spec.AppRef))
				Expect(updatedCFPackage.Spec.Source.Registry).To(Equal(korifiv1alpha1.Registry{
					Image:            packageSourceImageRef,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "image-pull-secret"}},
				}))
			})

			When("the package registry secret is not specified on the message", func() {
				BeforeEach(func() {
					updateMessage.RegistrySecretNames = []string{}
				})

				It("does not populate package registry secrets", func() {
					Expect(updatedCFPackage.Spec.Source.Registry.ImagePullSecrets).To(BeEmpty())
				})

				When("the registry secret on the package has been already set", func() {
					BeforeEach(func() {
						existingCFPackage.Spec.Source.Registry.ImagePullSecrets = []corev1.LocalObjectReference{
							{Name: "existing-secret"},
						}
					})

					It("unsets package registry secrets", func() {
						Expect(updatedCFPackage.Spec.Source.Registry.ImagePullSecrets).To(BeEmpty())
					})
				})
			})
		})

		When("user is not authorized to update a package", func() {
			It("returns a forbidden error", func() {
				Expect(updateErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("UpdatePackage", func() {
		var (
			packageGUID   string
			cfPackage     *korifiv1alpha1.CFPackage
			updateErr     error
			packageRecord repositories.PackageRecord
			updateMessage repositories.UpdatePackageMessage
		)

		BeforeEach(func() {
			packageGUID = generateGUID()
			updateMessage = repositories.UpdatePackageMessage{
				GUID: packageGUID,
				MetadataPatch: repositories.MetadataPatch{
					Labels: map[string]*string{
						"foo": tools.PtrTo("bar"),
					},
					Annotations: map[string]*string{
						"bar": tools.PtrTo("baz"),
					},
				},
			}
			cfPackage = createPackageCR(ctx, k8sClient, packageGUID, app.Name, space.Name, "")
		})

		JustBeforeEach(func() {
			packageRecord, updateErr = packageRepo.UpdatePackage(ctx, authInfo, updateMessage)
		})

		It("fails when the user is not auth'ed", func() {
			Expect(updateErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
		})

		When("the user is authorized read-only in the namespace", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceManagerRole.Name, space.Name)
			})

			It("fails with a forbidden error", func() {
				Expect(updateErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user is authorized in the namespace", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("succeeds", func() {
				Expect(updateErr).NotTo(HaveOccurred())
				Expect(packageRecord.GUID).To(Equal(packageGUID))
				Expect(packageRecord.Labels).To(Equal(map[string]string{"foo": "bar"}))
				Expect(packageRecord.Annotations).To(Equal(map[string]string{"bar": "baz"}))

				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfPackage), cfPackage)).To(Succeed())
					g.Expect(cfPackage.Labels).To(Equal(map[string]string{"foo": "bar"}))
					g.Expect(cfPackage.Annotations).To(Equal(map[string]string{"bar": "baz"}))
				}).Should(Succeed())
			})

			When("patch fails", func() {
				BeforeEach(func() {
					updateMessage.GUID = "doesn-t-exist"
				})

				It("returns an error", func() {
					Expect(updateErr).To(MatchError(ContainSubstring("doesn-t-exist")))
				})
			})

			When("unsetting a label", func() {
				BeforeEach(func() {
					updateMessage.MetadataPatch.Labels["foo"] = nil
				})

				It("removes the label", func() {
					Expect(packageRecord.Labels).ToNot(HaveKey("foo"))
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfPackage), cfPackage)).To(Succeed())
						g.Expect(cfPackage.Labels).To(BeEmpty())
					}).Should(Succeed())
				})
			})
		})
	})
})
