package repositories_test

import (
	"context"
	"time"

	. "github.com/onsi/gomega/gstruct"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("PackageRepository", func() {
	const appGUID = "the-app-guid"

	var (
		packageRepo *PackageRepo
		ctx         context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		packageRepo = NewPackageRepo(k8sClient)
	})

	Describe("CreatePackage", func() {
		var packageCreate PackageCreateMessage

		const (
			spaceGUID = "the-space-guid"
		)

		BeforeEach(func() {
			packageCreate = PackageCreateMessage{
				Type:      "bits",
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
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

			Expect(cleanupPackage(ctx, k8sClient, packageGUID, spaceGUID)).To(Succeed())
		})
	})

	Describe("FetchPackage", func() {
		var (
			namespace1 *corev1.Namespace
			namespace2 *corev1.Namespace
		)

		BeforeEach(func() {
			namespace1Name := generateGUID()
			namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
			Expect(k8sClient.Create(ctx, namespace1)).To(Succeed())

			namespace2Name := generateGUID()
			namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
			Expect(k8sClient.Create(ctx, namespace2)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, namespace1)).To(Succeed())
			Expect(k8sClient.Delete(ctx, namespace2)).To(Succeed())
		})

		When("on the happy path", func() {
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
						Namespace: namespace1.Name,
					},
					Spec: workloadsv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
					},
				}
				Expect(k8sClient.Create(ctx, package1)).To(Succeed())

				package2 = &workloadsv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      package2GUID,
						Namespace: namespace2.Name,
					},
					Spec: workloadsv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
					},
				}
				Expect(k8sClient.Create(ctx, package2)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, package1)).To(Succeed())
				Expect(k8sClient.Delete(ctx, package2)).To(Succeed())
			})

			It("can fetch the PackageRecord we're looking for", func() {
				record, err := packageRepo.FetchPackage(ctx, authInfo, package2GUID)
				Expect(err).NotTo(HaveOccurred())
				Expect(record.GUID).To(Equal(package2GUID))
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
		})

		When("table-testing the State field", func() {
			var cfPackage *workloadsv1alpha1.CFPackage

			BeforeEach(func() {
				packageGUID := generateGUID()
				cfPackage = &workloadsv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      packageGUID,
						Namespace: namespace1.Name,
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

						record, err := packageRepo.FetchPackage(ctx, authInfo, cfPackage.Name)
						Expect(err).NotTo(HaveOccurred())
						Expect(record.State).To(Equal(tc.expectedState))
					})
				})
			}
		})

		When("duplicate Packages exist across namespaces with the same GUID", func() {
			var (
				packageGUID string
				cfPackage1  *workloadsv1alpha1.CFPackage
				cfPackage2  *workloadsv1alpha1.CFPackage
			)

			BeforeEach(func() {
				packageGUID = generateGUID()
				cfPackage1 = &workloadsv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      packageGUID,
						Namespace: namespace1.Name,
					},
					Spec: workloadsv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
					},
				}
				Expect(k8sClient.Create(ctx, cfPackage1)).To(Succeed())

				cfPackage2 = &workloadsv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      packageGUID,
						Namespace: namespace2.Name,
					},
					Spec: workloadsv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
					},
				}
				Expect(k8sClient.Create(ctx, cfPackage2)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, cfPackage1)).To(Succeed())
				Expect(k8sClient.Delete(ctx, cfPackage2)).To(Succeed())
			})

			It("returns an error", func() {
				_, err := packageRepo.FetchPackage(ctx, authInfo, packageGUID)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("duplicate packages exist"))
			})
		})

		When("no packages exist", func() {
			It("returns an error", func() {
				_, err := packageRepo.FetchPackage(ctx, authInfo, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(NotFoundError{}))
			})
		})
	})

	Describe("FetchPackageList", Serial, func() {
		const (
			appGUID1 = "the-app-guid-1"
			appGUID2 = "the-app-guid-2"
		)

		var (
			namespace1 *corev1.Namespace
			namespace2 *corev1.Namespace
		)

		BeforeEach(func() {
			namespace1Name := generateGUID()
			namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
			Expect(k8sClient.Create(context.Background(), namespace1)).To(Succeed())
			namespace2Name := generateGUID()
			namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
			Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), namespace1)).To(Succeed())
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
						Namespace: namespace1.Name,
					},
					Spec: workloadsv1alpha1.CFPackageSpec{
						Type: "bits",
						AppRef: corev1.LocalObjectReference{
							Name: appGUID1,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), package1)).To(Succeed())

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
					packageList, err := packageRepo.FetchPackageList(context.Background(), authInfo, PackageListMessage{})
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
								Namespace: namespace1.Name,
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

					It("orders the results in descending created_at order by default", func() {
						packageList, err := packageRepo.FetchPackageList(context.Background(), authInfo, PackageListMessage{})
						Expect(err).NotTo(HaveOccurred())
						Expect(packageList).To(HaveLen(3))
						for i := 0; i < len(packageList)-1; i++ {
							currentRecordCreatedAt, err := time.Parse(time.RFC3339, packageList[i].CreatedAt)
							Expect(err).NotTo(HaveOccurred())

							nextRecordCreatedAt, err := time.Parse(time.RFC3339, packageList[i+1].CreatedAt)
							Expect(err).NotTo(HaveOccurred())

							Expect(currentRecordCreatedAt).To(BeTemporally(">=", nextRecordCreatedAt))
						}
					})
				})
			})

			When("app_guids filter is provided", func() {
				It("fetches all PackageRecords", func() {
					packageList, err := packageRepo.FetchPackageList(context.Background(), authInfo, PackageListMessage{AppGUIDs: []string{appGUID1}})
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
		})

		When("no packages exist", func() {
			It("returns an empty list of PackageRecords", func() {
				packageList, err := packageRepo.FetchPackageList(context.Background(), authInfo, PackageListMessage{})
				Expect(err).NotTo(HaveOccurred())
				Expect(packageList).To(BeEmpty())
			})
		})
	})

	Describe("UpdatePackageSource", func() {
		var (
			existingCFPackage workloadsv1alpha1.CFPackage
			spaceGUID         string
			updateMessage     PackageUpdateSourceMessage
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

			updateMessage = PackageUpdateSourceMessage{
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
