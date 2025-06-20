package repositories_test

import (
	"errors"
	"fmt"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	"code.cloudfoundry.org/korifi/api/repositories/fakeawaiter"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("PackageRepository", func() {
	var (
		repoCreator      *fake.RepositoryCreator
		conditionAwaiter *fakeawaiter.FakeAwaiter[
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
		conditionAwaiter = &fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFPackage,
			korifiv1alpha1.CFPackageList,
			*korifiv1alpha1.CFPackageList,
		]{}

		packageRepo = repositories.NewPackageRepo(
			spaceScopedKlient,
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
					Expect(packageGUID).To(matchers.BeValidUUID())
					Expect(createdPackage.Type).To(Equal("bits"))
					Expect(createdPackage.AppGUID).To(Equal(appGUID))
					Expect(createdPackage.State).To(Equal("AWAITING_UPLOAD"))
					Expect(createdPackage.Labels).To(HaveKeyWithValue("bob", "foo"))
					Expect(createdPackage.Annotations).To(HaveKeyWithValue("jim", "bar"))
					Expect(createdPackage.ImageRef).To(Equal(fmt.Sprintf("container.registry/foo/my/prefix-%s-packages", appGUID)))

					Expect(createdPackage.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))

					Expect(createdPackage.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))

					Expect(createdPackage.Relationships()).To(Equal(map[string]string{
						"app": appGUID,
					}))

					createdCFPackage := &korifiv1alpha1.CFPackage{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: space.Name,
							Name:      packageGUID,
						},
					}
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(createdCFPackage), createdCFPackage)).To(Succeed())

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
					Expect(packageGUID).To(matchers.BeValidUUID())
					Expect(createdPackage.Type).To(Equal("docker"))
					Expect(createdPackage.AppGUID).To(Equal(appGUID))
					Expect(createdPackage.Labels).To(HaveKeyWithValue("bob", "foo"))
					Expect(createdPackage.Annotations).To(HaveKeyWithValue("jim", "bar"))
					Expect(createdPackage.ImageRef).To(Equal("some/image"))

					Expect(createdPackage.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
					Expect(createdPackage.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))

					createdCFPackage := &korifiv1alpha1.CFPackage{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: space.Name,
							Name:      packageGUID,
						},
					}
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(createdCFPackage), createdCFPackage)).To(Succeed())

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
				Expect(getErr).To(MatchError(ContainSubstring("get-package duplicate records exist")))
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
			cfPackage *korifiv1alpha1.CFPackage

			listResult  repositories.ListResult[repositories.PackageRecord]
			listMessage repositories.ListPackagesMessage
		)

		BeforeEach(func() {
			cfPackage = &korifiv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space.Name,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFPackageSpec{
					Type: "docker",
				},
			}
			Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())

			listMessage = repositories.ListPackagesMessage{}
		})

		JustBeforeEach(func() {
			var err error
			listResult, err = packageRepo.ListPackages(ctx, authInfo, listMessage)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an empty list", func() {
			Expect(listResult.Records).To(BeEmpty())
		})

		When("the user is authorised to list packages", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("lists the packages", func() {
				Expect(listResult.Records).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"GUID": Equal(cfPackage.Name),
				})))
				Expect(listResult.PageInfo).To(Equal(descriptors.PageInfo{
					TotalResults: 1,
					TotalPages:   1,
					PageNumber:   1,
					PageSize:     1,
				}))
			})
		})

		Describe("parameters to list options", func() {
			var fakeKlient *fake.Klient

			BeforeEach(func() {
				fakeKlient = new(fake.Klient)
				packageRepo = repositories.NewPackageRepo(fakeKlient, nil, "", nil)

				listMessage = repositories.ListPackagesMessage{
					GUIDs:    []string{"g1", "g2"},
					AppGUIDs: []string{"ag1", "ag2"},
					States:   []string{"s1", "s2"},
					OrderBy:  "created_at",
					Pagination: repositories.Pagination{
						Page:    2,
						PerPage: 3,
					},
				}
			})

			It("translates  parameters to klient list options", func() {
				Expect(fakeKlient.ListCallCount()).To(Equal(1))
				_, _, listOptions := fakeKlient.ListArgsForCall(0)
				Expect(listOptions).To(ConsistOf(
					repositories.WithLabelIn(korifiv1alpha1.GUIDLabelKey, listMessage.GUIDs),
					repositories.WithLabelIn(korifiv1alpha1.CFAppGUIDLabelKey, listMessage.AppGUIDs),
					repositories.WithLabelIn(korifiv1alpha1.CFPackageStateLabelKey, listMessage.States),
					repositories.WithOrdering("created_at"),
					repositories.WithPaging(repositories.Pagination{
						Page:    2,
						PerPage: 3,
					}),
				))
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
					APIVersion: korifiv1alpha1.SchemeGroupVersion.String(),
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
				Expect(packageRecord.Labels).To(HaveKeyWithValue("foo", "bar"))
				Expect(packageRecord.Annotations).To(Equal(map[string]string{"bar": "baz"}))

				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfPackage), cfPackage)).To(Succeed())
					g.Expect(cfPackage.Labels).To(HaveKeyWithValue("foo", "bar"))
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
						g.Expect(cfPackage.Labels).NotTo(HaveKey("foo"))
					}).Should(Succeed())
				})
			})
		})
	})
})
