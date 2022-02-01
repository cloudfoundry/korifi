package repositories_test

import (
	"context"
	"time"

	. "github.com/onsi/gomega/gstruct"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("PackageRepository", func() {
	const appGUID = "the-app-guid"
	const appUID = "the-app-uid"

	var (
		packageRepo               *repositories.PackageRepo
		ctx                       context.Context
		spaceDeveloperClusterRole *rbacv1.ClusterRole
	)

	BeforeEach(func() {
		ctx = context.Background()
		packageRepo = repositories.NewPackageRepo(k8sClient, userClientFactory)
		spaceDeveloperClusterRole = createClusterRole(ctx, repositories.SpaceDeveloperClusterRoleRules)
	})

	Describe("CreatePackage", func() {
		var packageCreate repositories.CreatePackageMessage

		const (
			spaceGUID = "the-space-guid"
		)

		BeforeEach(func() {
			packageCreate = repositories.CreatePackageMessage{
				Type:      "bits",
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
				OwnerRef: metav1.OwnerReference{
					APIVersion: "workloads.cloudfoundry.org/v1alpha1",
					Kind:       "CFApp",
					Name:       appGUID,
					UID:        appUID,
				},
			}

			Expect(
				k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}}),
			).To(Succeed())
		})

		AfterEach(func() {
			Expect(
				k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}}),
			).To(Succeed())
		})

		It("creates a Package record", func() {
			returnedPackageRecord, err := packageRepo.CreatePackage(ctx, authInfo, packageCreate)
			Expect(err).NotTo(HaveOccurred())

			packageGUID := returnedPackageRecord.GUID
			Expect(packageGUID).NotTo(BeEmpty())
			Expect(returnedPackageRecord.Type).To(Equal("bits"))
			Expect(returnedPackageRecord.AppGUID).To(Equal(appGUID))
			Expect(returnedPackageRecord.State).To(Equal("AWAITING_UPLOAD"))

			createdAt, err := time.Parse(time.RFC3339, returnedPackageRecord.CreatedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

			updatedAt, err := time.Parse(time.RFC3339, returnedPackageRecord.CreatedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

			packageNSName := types.NamespacedName{Name: packageGUID, Namespace: spaceGUID}
			createdCFPackage := new(workloadsv1alpha1.CFPackage)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, packageNSName, createdCFPackage)
				return err == nil
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())

			Expect(createdCFPackage.Name).To(Equal(packageGUID))
			Expect(createdCFPackage.Namespace).To(Equal(spaceGUID))
			Expect(createdCFPackage.Spec.Type).To(Equal(workloadsv1alpha1.PackageType("bits")))
			Expect(createdCFPackage.Spec.AppRef.Name).To(Equal(appGUID))
			Expect(createdCFPackage.ObjectMeta.OwnerReferences).To(Equal(
				[]metav1.OwnerReference{
					{
						APIVersion: "workloads.cloudfoundry.org/v1alpha1",
						Kind:       "CFApp",
						Name:       appGUID,
						UID:        appUID,
					},
				}))

			Expect(cleanupPackage(ctx, k8sClient, packageGUID, spaceGUID)).To(Succeed())
		})
	})

	Describe("GetPackage", func() {
		var (
			namespace   *corev1.Namespace
			packageGUID string
			cfPackage   *workloadsv1alpha1.CFPackage
		)

		BeforeEach(func() {
			namespaceName := generateGUID()
			namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
			Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

			packageGUID = generateGUID()
			cfPackage = &workloadsv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      packageGUID,
					Namespace: namespace.Name,
				},
				Spec: workloadsv1alpha1.CFPackageSpec{
					Type: "bits",
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, cfPackage)).To(Succeed())
			Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
		})

		When("the user is authorized in the namespace", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, namespace.Name)
			})

			It("can fetch the PackageRecord we're looking for", func() {
				record, err := packageRepo.GetPackage(ctx, authInfo, packageGUID)
				Expect(err).NotTo(HaveOccurred())
				Expect(record.GUID).To(Equal(packageGUID))
				Expect(record.Type).To(Equal("bits"))
				Expect(record.AppGUID).To(Equal(appGUID))
				Expect(record.State).To(Equal("AWAITING_UPLOAD"))

				createdAt, err := time.Parse(time.RFC3339, record.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

				updatedAt, err := time.Parse(time.RFC3339, record.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
			})

			When("table-testing the State field", func() {
				var cfPackage *workloadsv1alpha1.CFPackage

				BeforeEach(func() {
					packageGUID = generateGUID()
					cfPackage = &workloadsv1alpha1.CFPackage{
						ObjectMeta: metav1.ObjectMeta{
							Name:      packageGUID,
							Namespace: namespace.Name,
						},
						Spec: workloadsv1alpha1.CFPackageSpec{
							Type: "bits",
							AppRef: corev1.LocalObjectReference{
								Name: appGUID,
							},
						},
					}
				})

				type testCase struct {
					description   string
					expectedState string
					setupFunc     func(cfPackage2 *workloadsv1alpha1.CFPackage)
				}

				cases := []testCase{
					{
						description:   "no source image is set",
						expectedState: "AWAITING_UPLOAD",
						setupFunc:     func(p *workloadsv1alpha1.CFPackage) { p.Spec.Source = workloadsv1alpha1.PackageSource{} },
					},
					{
						description:   "an source image is set",
						expectedState: "READY",
						setupFunc:     func(p *workloadsv1alpha1.CFPackage) { p.Spec.Source.Registry.Image = "some-org/some-repo" },
					},
				}

				for _, tc := range cases {
					When(tc.description, func() {
						It("has state "+tc.expectedState, func() {
							tc.setupFunc(cfPackage)
							Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())
							defer func() { Expect(k8sClient.Delete(ctx, cfPackage)).To(Succeed()) }()

							record, err := packageRepo.GetPackage(ctx, authInfo, cfPackage.Name)
							Expect(err).NotTo(HaveOccurred())
							Expect(record.State).To(Equal(tc.expectedState))
						})
					})
				}
			})
		})

		When("user is not authorized to get a package", func() {
			It("returns a forbidden error", func() {
				_, err := packageRepo.GetPackage(ctx, authInfo, packageGUID)
				Expect(err).To(BeAssignableToTypeOf(repositories.ForbiddenError{}))
			})
		})

		When("duplicate Packages exist across namespaces with the same GUID", func() {
			var (
				duplicatePackage *workloadsv1alpha1.CFPackage
				anotherNamespace *corev1.Namespace
			)

			BeforeEach(func() {
				anotherNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: generateGUID()}}
				Expect(k8sClient.Create(ctx, anotherNamespace)).To(Succeed())

				duplicatePackage = &workloadsv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      packageGUID,
						Namespace: anotherNamespace.Name,
					},
					Spec: workloadsv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
					},
				}
				Expect(k8sClient.Create(ctx, duplicatePackage)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, duplicatePackage)).To(Succeed())
			})

			It("returns an error", func() {
				_, err := packageRepo.GetPackage(ctx, authInfo, packageGUID)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("duplicate packages exist"))
			})
		})

		When("no packages exist", func() {
			It("returns an error", func() {
				_, err := packageRepo.GetPackage(ctx, authInfo, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(repositories.NotFoundError{}))
			})
		})
	})

	Describe("ListPackages", Serial, func() {
		const (
			appGUID1 = "the-app-guid-1"
			appGUID2 = "the-app-guid-2"
		)

		var (
			namespace  *corev1.Namespace
			namespace2 *corev1.Namespace
		)

		BeforeEach(func() {
			namespace1Name := generateGUID()
			namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
			Expect(k8sClient.Create(context.Background(), namespace)).To(Succeed())
			namespace2Name := generateGUID()
			namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
			Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), namespace2)).To(Succeed())
		})

		When("multiple packages exist in different namespaces", func() {
			var (
				package1GUID string
				package2GUID string
				package1     *workloadsv1alpha1.CFPackage
				package2     *workloadsv1alpha1.CFPackage
			)

			BeforeEach(func() {
				package1GUID = generateGUID()
				package2GUID = generateGUID()
				package1 = &workloadsv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      package1GUID,
						Namespace: namespace.Name,
					},
					Spec: workloadsv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: appGUID1,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), package1)).To(Succeed())

				// add a small delay to test ordering on created_by
				time.Sleep(1 * time.Second)

				package2 = &workloadsv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      package2GUID,
						Namespace: namespace2.Name,
					},
					Spec: workloadsv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: appGUID2,
						},
						Source: workloadsv1alpha1.PackageSource{
							Registry: workloadsv1alpha1.Registry{
								Image: "my-image-url",
							},
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), package2)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), package1)).To(Succeed())
				Expect(k8sClient.Delete(context.Background(), package2)).To(Succeed())
			})

			When("no filters are specified", func() {
				It("fetches all PackageRecords", func() {
					packageList, err := packageRepo.ListPackages(context.Background(), authInfo, repositories.ListPackagesMessage{})
					Expect(err).NotTo(HaveOccurred())

					Expect(packageList).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"GUID":    Equal(package1GUID),
							"AppGUID": Equal(appGUID1),
						}),
						MatchFields(IgnoreExtras, Fields{
							"GUID":    Equal(package2GUID),
							"AppGUID": Equal(appGUID2),
						}),
					))
				})

				When("three packages exist", func() {
					var (
						package3GUID string
						package3     *workloadsv1alpha1.CFPackage
					)

					BeforeEach(func() {
						package3GUID = generateGUID()
						package3 = &workloadsv1alpha1.CFPackage{
							ObjectMeta: metav1.ObjectMeta{
								Name:      package3GUID,
								Namespace: namespace.Name,
							},
							Spec: workloadsv1alpha1.CFPackageSpec{
								Type: "bits",
								AppRef: corev1.LocalObjectReference{
									Name: appGUID1,
								},
							},
						}
						// add a small delay to test ordering on created_by
						time.Sleep(time.Second * 1)
						Expect(k8sClient.Create(context.Background(), package3)).To(Succeed())
					})

					AfterEach(func() {
						Expect(k8sClient.Delete(context.Background(), package3)).To(Succeed())
					})

					It("orders the results in ascending created_at order by default", func() {
						packageList, err := packageRepo.ListPackages(context.Background(), authInfo, repositories.ListPackagesMessage{})
						Expect(err).NotTo(HaveOccurred())
						Expect(packageList).To(HaveLen(3))
						Expect(packageList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package1GUID),
								"AppGUID": Equal(appGUID1),
							}),
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package2GUID),
								"AppGUID": Equal(appGUID2),
							}),
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package3GUID),
								"AppGUID": Equal(appGUID1),
							}),
						))

						Expect(packageList[0].GUID).To(Equal(package1GUID))
						Expect(packageList[1].GUID).To(Equal(package2GUID))
						Expect(packageList[2].GUID).To(Equal(package3GUID))
					})
				})
			})

			When("app_guids filter is provided", func() {
				It("fetches all PackageRecords", func() {
					packageList, err := packageRepo.ListPackages(context.Background(), authInfo, repositories.ListPackagesMessage{AppGUIDs: []string{appGUID1}})
					Expect(err).NotTo(HaveOccurred())
					Expect(packageList).To(HaveLen(1))
					Expect(packageList[0]).To(
						MatchFields(IgnoreExtras, Fields{
							"GUID":    Equal(package1GUID),
							"AppGUID": Equal(appGUID1),
						}),
					)
				})
			})

			When("SortBy is provided and value is created_at", func() {
				var (
					package3GUID string
					package3     *workloadsv1alpha1.CFPackage
				)
				BeforeEach(func() {
					package3GUID = generateGUID()
					package3 = &workloadsv1alpha1.CFPackage{
						ObjectMeta: metav1.ObjectMeta{
							Name:      package3GUID,
							Namespace: namespace2.Name,
						},
						Spec: workloadsv1alpha1.CFPackageSpec{
							Type: "bits",
							AppRef: corev1.LocalObjectReference{
								Name: appGUID1,
							},
						},
					}
					// add a small delay to test ordering on created_by
					time.Sleep(1 * time.Second)

					Expect(k8sClient.Create(context.Background(), package3)).To(Succeed())

					DeferCleanup(func() {
						_ = k8sClient.Delete(context.Background(), package3)
					})
				})

				When("descending order is false", func() {
					It("fetches packages sorted by created_at in ascending order", func() {
						packageList, err := packageRepo.ListPackages(context.Background(), authInfo, repositories.ListPackagesMessage{SortBy: "created_at", DescendingOrder: false})
						Expect(err).NotTo(HaveOccurred())

						Expect(packageList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package1GUID),
								"AppGUID": Equal(appGUID1),
							}),
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package2GUID),
								"AppGUID": Equal(appGUID2),
							}),
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package3GUID),
								"AppGUID": Equal(appGUID1),
							}),
						))

						Expect(packageList[0].GUID).To(Equal(package1GUID))
						Expect(packageList[1].GUID).To(Equal(package2GUID))
						Expect(packageList[2].GUID).To(Equal(package3GUID))
					})
				})

				When("descending order is true", func() {
					It("fetches packages sorted by created_at in descending order", func() {
						packageList, err := packageRepo.ListPackages(context.Background(), authInfo, repositories.ListPackagesMessage{SortBy: "created_at", DescendingOrder: true})
						Expect(err).NotTo(HaveOccurred())

						Expect(packageList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package1GUID),
								"AppGUID": Equal(appGUID1),
							}),
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package2GUID),
								"AppGUID": Equal(appGUID2),
							}),
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package3GUID),
								"AppGUID": Equal(appGUID1),
							}),
						))

						Expect(packageList[0].GUID).To(Equal(package3GUID))
						Expect(packageList[1].GUID).To(Equal(package2GUID))
						Expect(packageList[2].GUID).To(Equal(package1GUID))
					})
				})
			})

			When("State filter is provided", func() {
				When("filtering by State=READY", func() {
					It("filters the packages", func() {
						packageList, err := packageRepo.ListPackages(context.Background(), authInfo, repositories.ListPackagesMessage{States: []string{"READY"}})
						Expect(err).NotTo(HaveOccurred())

						Expect(packageList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package2GUID),
								"AppGUID": Equal(appGUID2),
								"State":   Equal("READY"),
							}),
						))
					})
				})

				When("filtering by State=AWAITING_UPLOAD", func() {
					It("filters the packages", func() {
						packageList, err := packageRepo.ListPackages(context.Background(), authInfo, repositories.ListPackagesMessage{States: []string{"AWAITING_UPLOAD"}})
						Expect(err).NotTo(HaveOccurred())

						Expect(packageList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package1GUID),
								"AppGUID": Equal(appGUID1),
								"State":   Equal("AWAITING_UPLOAD"),
							}),
						))
					})
				})

				When("filtering by State=AWAITING_UPLOAD,READY", func() {
					It("filters the packages", func() {
						packageList, err := packageRepo.ListPackages(context.Background(), authInfo, repositories.ListPackagesMessage{States: []string{"AWAITING_UPLOAD", "READY"}})
						Expect(err).NotTo(HaveOccurred())

						Expect(packageList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{
								"GUID":    Equal(package1GUID),
								"AppGUID": Equal(appGUID1),
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

		When("no packages exist", func() {
			It("returns an empty list of PackageRecords", func() {
				packageList, err := packageRepo.ListPackages(context.Background(), authInfo, repositories.ListPackagesMessage{})
				Expect(err).NotTo(HaveOccurred())
				Expect(packageList).To(BeEmpty())
			})
		})
	})

	Describe("UpdatePackageSource", func() {
		var (
			existingCFPackage workloadsv1alpha1.CFPackage
			spaceGUID         string
			updateMessage     repositories.UpdatePackageSourceMessage
		)

		const (
			packageGUID               = "the-package-guid"
			packageSourceImageRef     = "my-org/" + packageGUID
			packageRegistrySecretName = "image-pull-secret"
		)

		BeforeEach(func() {
			spaceGUID = generateGUID()

			existingCFPackage = workloadsv1alpha1.CFPackage{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFPackage",
					APIVersion: workloadsv1alpha1.GroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      packageGUID,
					Namespace: spaceGUID,
				},
				Spec: workloadsv1alpha1.CFPackageSpec{
					Type:   "bits",
					AppRef: corev1.LocalObjectReference{Name: appGUID},
				},
			}

			updateMessage = repositories.UpdatePackageSourceMessage{
				GUID:               packageGUID,
				SpaceGUID:          spaceGUID,
				ImageRef:           packageSourceImageRef,
				RegistrySecretName: packageRegistrySecretName,
			}

			Expect(
				k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}}),
			).To(Succeed())

			Expect(
				k8sClient.Create(ctx, &existingCFPackage),
			).To(Succeed())
		})

		AfterEach(func() {
			Expect(
				k8sClient.Delete(ctx, &existingCFPackage),
			).To(Succeed())

			Expect(
				k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}}),
			).To(Succeed())
		})

		It("returns an updated record", func() {
			returnedPackageRecord, err := packageRepo.UpdatePackageSource(ctx, authInfo, updateMessage)
			Expect(err).NotTo(HaveOccurred())

			Expect(returnedPackageRecord.GUID).To(Equal(existingCFPackage.ObjectMeta.Name))
			Expect(returnedPackageRecord.Type).To(Equal(string(existingCFPackage.Spec.Type)))
			Expect(returnedPackageRecord.AppGUID).To(Equal(existingCFPackage.Spec.AppRef.Name))
			Expect(returnedPackageRecord.SpaceGUID).To(Equal(existingCFPackage.ObjectMeta.Namespace))
			Expect(returnedPackageRecord.State).To(Equal("READY"))

			createdAt, err := time.Parse(time.RFC3339, returnedPackageRecord.CreatedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

			updatedAt, err := time.Parse(time.RFC3339, returnedPackageRecord.CreatedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
		})

		It("updates only the Registry field of the existing CFPackage", func() {
			_, err := packageRepo.UpdatePackageSource(ctx, authInfo, updateMessage)
			Expect(err).NotTo(HaveOccurred())

			packageNSName := types.NamespacedName{Name: packageGUID, Namespace: spaceGUID}
			createdCFPackage := new(workloadsv1alpha1.CFPackage)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, packageNSName, createdCFPackage)
				return err == nil
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())

			Expect(createdCFPackage.Name).To(Equal(existingCFPackage.ObjectMeta.Name))
			Expect(createdCFPackage.Namespace).To(Equal(existingCFPackage.ObjectMeta.Namespace))
			Expect(createdCFPackage.Spec.Type).To(Equal(existingCFPackage.Spec.Type))
			Expect(createdCFPackage.Spec.AppRef).To(Equal(existingCFPackage.Spec.AppRef))
			Expect(createdCFPackage.Spec.Source.Registry).To(Equal(workloadsv1alpha1.Registry{
				Image:            packageSourceImageRef,
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: packageRegistrySecretName}},
			}))
		})
	})
})

func cleanupPackage(ctx context.Context, k8sClient client.Client, packageGUID, namespace string) error {
	cfPackage := workloadsv1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      packageGUID,
			Namespace: namespace,
		},
	}
	return k8sClient.Delete(ctx, &cfPackage)
}
