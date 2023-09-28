package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("PackageRepository", func() {
	var (
		repoCreator      *fake.RepositoryCreator
		conditionAwaiter *FakeAwaiter[
			*korifiv1alpha1.CFPackage,
			korifiv1alpha1.CFPackageList,
			*korifiv1alpha1.CFPackageList,
		]
		packageRepo *repositories.PackageRepo
		org         *korifiv1alpha1.CFOrg
		space       *korifiv1alpha1.CFSpace
		app         *korifiv1alpha1.CFApp
		appGUID     string
	)

	BeforeEach(func() {
		repoCreator = new(fake.RepositoryCreator)
		conditionAwaiter = &FakeAwaiter[
			*korifiv1alpha1.CFPackage,
			korifiv1alpha1.CFPackageList,
			*korifiv1alpha1.CFPackageList,
		]{}
		packageRepo = repositories.NewPackageRepo(
			userClientFactory,
			namespaceRetriever,
			nsPerms,
			repoCreator,
			"container.registry/foo/my/prefix-",
			conditionAwaiter,
		)
		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))

		appGUID = uuid.NewString()
		app = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appGUID,
				Namespace: space.Name,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  uuid.NewString(),
				DesiredState: "STOPPED",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
	})

	Describe("CreatePackage", func() {
		var (
			packageCreate  repositories.CreatePackageMessage
			createdPackage repositories.PackageRecord
			createErr      error
		)

		BeforeEach(func() {
			appGUID = uuid.NewString()
			app = &korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFAppSpec{
					DisplayName:  uuid.NewString(),
					DesiredState: "STOPPED",
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "docker",
					},
				},
			}

			packageCreate = repositories.CreatePackageMessage{
				AppGUID:   appGUID,
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
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			createdPackage, createErr = packageRepo.CreatePackage(ctx, authInfo, packageCreate)
		})

		It("fails because the user is not a space developer", func() {
			Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
		})

		When("the user is a SpaceDeveloper", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			Describe("bits package", func() {
				BeforeEach(func() {
					app.Spec.Lifecycle = korifiv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: korifiv1alpha1.LifecycleData{
							Buildpacks: []string{"java"},
						},
					}
					packageCreate.Type = "bits"
				})

				It("creates a Package record", func() {
					Expect(createErr).NotTo(HaveOccurred())

					packageGUID := createdPackage.GUID
					Expect(packageGUID).NotTo(BeEmpty())
					Expect(createdPackage.Type).To(Equal("bits"))
					Expect(createdPackage.AppGUID).To(Equal(appGUID))
					Expect(createdPackage.State).To(Equal("AWAITING_UPLOAD"))
					Expect(createdPackage.Labels).To(HaveKeyWithValue("bob", "foo"))
					Expect(createdPackage.Annotations).To(HaveKeyWithValue("jim", "bar"))
					Expect(createdPackage.ImageRef).To(Equal(fmt.Sprintf("container.registry/foo/my/prefix-%s-packages", appGUID)))

					Expect(createdPackage.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))

					Expect(createdPackage.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))

					packageNSName := types.NamespacedName{Name: packageGUID, Namespace: space.Name}
					createdCFPackage := new(korifiv1alpha1.CFPackage)
					Expect(k8sClient.Get(ctx, packageNSName, createdCFPackage)).To(Succeed())

					Expect(createdCFPackage.Name).To(Equal(packageGUID))
					Expect(createdCFPackage.Namespace).To(Equal(space.Name))
					Expect(createdCFPackage.Spec.Type).To(Equal(korifiv1alpha1.PackageType("bits")))
					Expect(createdCFPackage.Spec.AppRef.Name).To(Equal(appGUID))

					Expect(createdCFPackage.Labels).To(HaveKeyWithValue("bob", "foo"))
					Expect(createdCFPackage.Annotations).To(HaveKeyWithValue("jim", "bar"))
				})

				It("awaits the Initialized status", func() {
					Expect(conditionAwaiter.AwaitConditionCallCount()).To(Equal(1))
					obj, conditionType := conditionAwaiter.AwaitConditionArgsForCall(0)
					Expect(obj.GetName()).To(Equal(createdPackage.GUID))
					Expect(obj.GetNamespace()).To(Equal(space.Name))
					Expect(conditionType).To(Equal("Initialized"))
				})

				It("creates a package repository", func() {
					Expect(repoCreator.CreateRepositoryCallCount()).To(Equal(1))
					_, repoName := repoCreator.CreateRepositoryArgsForCall(0)
					Expect(repoName).To(Equal("container.registry/foo/my/prefix-" + appGUID + "-packages"))
				})

				When("the package does not become initialized", func() {
					BeforeEach(func() {
						conditionAwaiter.AwaitConditionReturns(&korifiv1alpha1.CFPackage{}, errors.New("time-out-err"))
					})

					It("returns an error", func() {
						Expect(createErr).To(MatchError(ContainSubstring("time-out-err")))
					})
				})

				When("repo creation errors", func() {
					BeforeEach(func() {
						repoCreator.CreateRepositoryReturns(errors.New("repo create error"))
					})

					It("returns an error", func() {
						Expect(createErr).To(MatchError(ContainSubstring("repo create error")))
					})
				})

				When("the referenced app has docker lifecycle type", func() {
					BeforeEach(func() {
						app.Spec.Lifecycle = korifiv1alpha1.Lifecycle{
							Type: "docker",
						}
					})

					It("returns an error", func() {
						Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
					})
				})
			})

			Describe("docker package", func() {
				BeforeEach(func() {
					app.Spec.Lifecycle = korifiv1alpha1.Lifecycle{
						Type: "docker",
					}

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
					Expect(createdPackage.AppGUID).To(Equal(appGUID))
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
					Expect(createdCFPackage.Spec.AppRef.Name).To(Equal(appGUID))
					Expect(createdCFPackage.Spec.Source.Registry.Image).To(Equal("some/image"))
					Expect(createdCFPackage.Spec.Source.Registry.ImagePullSecrets).To(BeEmpty())

					Expect(createdCFPackage.Labels).To(HaveKeyWithValue("bob", "foo"))
					Expect(createdCFPackage.Annotations).To(HaveKeyWithValue("jim", "bar"))
				})

				It("awaits the Initialized status", func() {
					Expect(conditionAwaiter.AwaitConditionCallCount()).To(Equal(1))
					obj, conditionType := conditionAwaiter.AwaitConditionArgsForCall(0)
					Expect(obj.GetName()).To(Equal(createdPackage.GUID))
					Expect(obj.GetNamespace()).To(Equal(space.Name))
					Expect(conditionType).To(Equal("Initialized"))
				})

				It("does not create source image repository", func() {
					Expect(repoCreator.CreateRepositoryCallCount()).To(BeZero())
				})

				When("the image is private", func() {
					BeforeEach(func() {
						packageCreate.Data.Username = tools.PtrTo("bob")
						packageCreate.Data.Password = tools.PtrTo("paswd")
					})

					It("generates an image pull secret", func() {
						Expect(createErr).NotTo(HaveOccurred())

						createdCFPackage := &korifiv1alpha1.CFPackage{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: space.Name,
								Name:      createdPackage.GUID,
							},
						}
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(createdCFPackage), createdCFPackage)).To(Succeed())
						Expect(createdCFPackage.Spec.Source.Registry.ImagePullSecrets).To(ConsistOf(corev1.LocalObjectReference{Name: createdPackage.GUID}))

						imgPullSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: space.Name,
								Name:      createdPackage.GUID,
							},
						}
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(imgPullSecret), imgPullSecret)).To(Succeed())
						Expect(imgPullSecret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))
						Expect(imgPullSecret.Data).NotTo(BeEmpty())

						Expect(imgPullSecret.GetOwnerReferences()).To(ConsistOf(metav1.OwnerReference{
							UID:        createdCFPackage.UID,
							Kind:       "CFPackage",
							APIVersion: "korifi.cloudfoundry.org/v1alpha1",
							Name:       createdCFPackage.Name,
						}))
					})
				})

				When("the referenced app has buildpack lifecycle type", func() {
					BeforeEach(func() {
						app.Spec.Lifecycle = korifiv1alpha1.Lifecycle{
							Type: "buildpack",
							Data: korifiv1alpha1.LifecycleData{
								Buildpacks: []string{"bp"},
								Stack:      "my-stack",
							},
						}
					})

					It("returns an error", func() {
						Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
					})
				})
			})

			When("the referenced app does not exist", func() {
				BeforeEach(func() {
					packageCreate.AppGUID = uuid.NewString()
				})

				It("returns an error", func() {
					Expect(createErr).To(MatchError(ContainSubstring("not found")))
				})
			})
		})
	})

	Describe("GetPackage", func() {
		var (
			packageGUID   string
			cfPackage     *korifiv1alpha1.CFPackage
			packageRecord repositories.PackageRecord
			getErr        error
		)

		BeforeEach(func() {
			packageGUID = uuid.NewString()
			cfPackage = &korifiv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      packageGUID,
					Namespace: space.Name,
					Labels: map[string]string{
						"foo": "the-original-value",
					},
					Annotations: map[string]string{
						"bar": "the-original-value",
					},
				},
				Spec: korifiv1alpha1.CFPackageSpec{
					Type: "bits",
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
				},
			}

			Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())
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
				Expect(packageRecord.Labels).To(HaveKeyWithValue("foo", "the-original-value"))
				Expect(packageRecord.Annotations).To(HaveKeyWithValue("bar", "the-original-value"))
				Expect(packageRecord.ImageRef).To(Equal(fmt.Sprintf("container.registry/foo/my/prefix-%s-packages", appGUID)))
				Expect(packageRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(packageRecord.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
			})

			Describe("State field", func() {
				It("equals AWAITING_UPLOAD by default", func() {
					Expect(packageRecord.State).To(Equal("AWAITING_UPLOAD"))
				})

				When("the package is ready", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, cfPackage, func() {
							meta.SetStatusCondition(&cfPackage.Status.Conditions, metav1.Condition{
								Type:               "Ready",
								Status:             "True",
								Reason:             "Ready",
								ObservedGeneration: cfPackage.Generation,
							})
						})).To(Succeed())
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
				Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      packageGUID,
						Namespace: anotherSpace.Name,
					},
					Spec: korifiv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
					},
				})).To(Succeed())
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
				package1GUID = uuid.NewString()
				Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      package1GUID,
						Namespace: space.Name,
					},
					Spec: korifiv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
					},
				})).To(Succeed())

				package2GUID = uuid.NewString()
				cfPackage := &korifiv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      package2GUID,
						Namespace: space2.Name,
					},
					Spec: korifiv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: app2.Name,
						},
					},
				}
				Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())
				Expect(k8s.Patch(ctx, k8sClient, cfPackage, func() {
					meta.SetStatusCondition(&cfPackage.Status.Conditions, metav1.Condition{
						Type:               "Ready",
						Status:             "True",
						Reason:             "Ready",
						ObservedGeneration: cfPackage.Generation,
					})
				})).To(Succeed())

				noPermissionsSpace = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("no-permissions-space"))
				noPermissionsPackageGUID = prefixedGUID("no-permissions-package")
				Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      noPermissionsPackageGUID,
						Namespace: noPermissionsSpace.Name,
					},
					Spec: korifiv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: app2.Name,
						},
					},
				})).To(Succeed())
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
					AppRef: corev1.LocalObjectReference{Name: appGUID},
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
			packageGUID = uuid.NewString()
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
			cfPackage = &korifiv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      packageGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFPackageSpec{
					Type: "bits",
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())
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
