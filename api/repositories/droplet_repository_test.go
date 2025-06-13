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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("DropletRepository", func() {
	const (
		dropletStack        = "cflinuxfs3"
		registryImage       = "registry/image:tag"
		registryImageSecret = "secret-key"
	)

	var (
		dropletRepo *repositories.DropletRepo
		build       *korifiv1alpha1.CFBuild
	)

	BeforeEach(func() {
		org := createOrgWithCleanup(ctx, uuid.NewString())
		space := createSpaceWithCleanup(ctx, org.Name, uuid.NewString())

		dropletRepo = repositories.NewDropletRepo(spaceScopedKlient)

		packageGUID := uuid.NewString()
		appGUID := uuid.NewString()
		build = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
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
			fetchBuildGUID = build.Name
		})

		JustBeforeEach(func() {
			dropletRecord, fetchErr = dropletRepo.GetDroplet(ctx, authInfo, fetchBuildGUID)
		})

		It("returns a forbidden error", func() {
			Expect(fetchErr).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is authorized to get the droplet", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, build.Namespace)
			})

			It("returns a NotFound error", func() {
				Expect(fetchErr).To(MatchError(apierrors.NewNotFoundError(nil, repositories.DropletResourceType)))
			})

			When("the build is staged", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, build, func() {
						build.Status.State = korifiv1alpha1.BuildStateStaged
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
							Ports: []int32{1234, 2345},
						}
					})).To(Succeed())
				})

				It("returns a droplet record with fields set to expected values", func() {
					Expect(fetchErr).NotTo(HaveOccurred())

					Expect(dropletRecord.State).To(Equal("STAGED"))
					Expect(dropletRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
					Expect(dropletRecord.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
					Expect(dropletRecord.Stack).To(Equal(dropletStack))
					Expect(dropletRecord.Lifecycle.Type).To(Equal(string(build.Spec.Lifecycle.Type)))
					Expect(dropletRecord.Lifecycle.Data.Buildpacks).To(BeEmpty())
					Expect(dropletRecord.Lifecycle.Data.Stack).To(Equal(build.Spec.Lifecycle.Data.Stack))
					Expect(dropletRecord.Image).To(BeEmpty())
					Expect(dropletRecord.Ports).To(ConsistOf(int32(1234), int32(2345)))
					Expect(dropletRecord.AppGUID).To(Equal(build.Spec.AppRef.Name))
					Expect(dropletRecord.PackageGUID).To(Equal(build.Spec.PackageRef.Name))
					Expect(dropletRecord.Labels).To(SatisfyAll(
						HaveKeyWithValue("key1", "val1"),
						HaveKeyWithValue("key2", "val2"),
					))
					Expect(dropletRecord.Annotations).To(SatisfyAll(
						HaveKeyWithValue("key1", "val1"),
						HaveKeyWithValue("key2", "val2"),
					))

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

			When("build does not exist", func() {
				BeforeEach(func() {
					fetchBuildGUID = "i don't exist"
				})

				It("returns an error", func() {
					Expect(fetchErr).To(HaveOccurred())
					Expect(fetchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})
	})

	Describe("ListDroplets", func() {
		var (
			listResult repositories.ListResult[repositories.DropletRecord]
			listErr    error
			message    repositories.ListDropletsMessage
		)

		BeforeEach(func() {
			message = repositories.ListDropletsMessage{}

			Expect(k8s.Patch(ctx, k8sClient, build, func() {
				build.Status.State = korifiv1alpha1.BuildStateStaged
				build.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{}
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			listResult, listErr = dropletRepo.ListDroplets(ctx, authInfo, message)
		})

		It("returns an empty list to users who lack access", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(listResult.Records).To(BeEmpty())
			Expect(listResult.PageInfo.TotalResults).To(BeZero())
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, build.Namespace)
			})

			It("returns the droplets", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listResult.Records).To(HaveLen(1))
				Expect(listResult.PageInfo.TotalResults).To(Equal(1))
			})

			When("the build has no droplet set yet", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, build, func() {
						build.Status.State = korifiv1alpha1.BuildStateStaging
						build.Status.Droplet = nil
					})).To(Succeed())
				})

				It("it does not return a droplet for that build", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(listResult.Records).To(BeEmpty())
					Expect(listResult.PageInfo.TotalResults).To(BeZero())
				})
			})

			Describe("parameters to list options", func() {
				var fakeKlient *fake.Klient

				BeforeEach(func() {
					fakeKlient = new(fake.Klient)
					dropletRepo = repositories.NewDropletRepo(fakeKlient)

					message = repositories.ListDropletsMessage{
						GUIDs:        []string{"a1", "a2"},
						PackageGUIDs: []string{"p1", "p2"},
						AppGUIDs:     []string{"a1", "a2"},
						SpaceGUIDs:   []string{"a1", "a2"},
						Pagination: repositories.Pagination{
							Page:    2,
							PerPage: 10,
						},
					}
				})

				It("translates filter parameters to klient list options", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(fakeKlient.ListCallCount()).To(Equal(1))
					_, _, listOptions := fakeKlient.ListArgsForCall(0)
					Expect(listOptions).To(ConsistOf(
						repositories.WithLabelIn(korifiv1alpha1.CFDropletGUIDLabelKey, []string{"a1", "a2"}),
						repositories.WithLabelIn(korifiv1alpha1.CFPackageGUIDLabelKey, []string{"p1", "p2"}),
						repositories.WithLabelIn(korifiv1alpha1.CFAppGUIDLabelKey, []string{"a1", "a2"}),
						repositories.WithLabelIn(korifiv1alpha1.SpaceGUIDLabelKey, []string{"a1", "a2"}),
						repositories.WithLabel(korifiv1alpha1.CFBuildStateLabelKey, korifiv1alpha1.BuildStateStaged),
						repositories.WithPaging(repositories.Pagination{
							Page:    2,
							PerPage: 10,
						}),
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
				GUID: build.Name,
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

		It("returns a forbidden error", func() {
			Expect(updateError).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is authorized to update the droplet", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, build.Namespace)
			})

			It("returns a NotFound error", func() {
				Expect(updateError).To(MatchError(apierrors.NewNotFoundError(nil, repositories.DropletResourceType)))
			})

			When("the build is staged", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, build, func() {
						build.Status.State = korifiv1alpha1.BuildStateStaged
						build.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{}
					})).To(Succeed())
				})

				It("updates the build metadata in kubernetes", func() {
					Expect(updateError).NotTo(HaveOccurred())

					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(build), build)).To(Succeed())

					Expect(build.Labels).To(SatisfyAll(
						HaveKeyWithValue("key1", "val1edit"),
						Not(HaveKey("key2")),
						HaveKeyWithValue("key3", "val3")))
					Expect(build.Annotations).To(SatisfyAll(
						HaveKeyWithValue("key1", "val1edit"),
						Not(HaveKey("key2")),
						HaveKeyWithValue("key3", "val3")))
				})

				It("returns a droplet record with updated metadata", func() {
					Expect(updateError).NotTo(HaveOccurred())

					Expect(dropletRecord.Labels).To(SatisfyAll(
						HaveKeyWithValue("key1", "val1edit"),
						Not(HaveKey("key2")),
						HaveKeyWithValue("key3", "val3"),
					))

					Expect(dropletRecord.Annotations).To(SatisfyAll(
						HaveKeyWithValue("key1", "val1edit"),
						Not(HaveKey("key2")),
						HaveKeyWithValue("key3", "val3"),
					))
				})
			})

			When("build does not exist", func() {
				BeforeEach(func() {
					dropletUpdateMsg.GUID = "i don't exist"
				})

				It("returns an error", func() {
					Expect(updateError).To(HaveOccurred())
					Expect(updateError).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})
	})
})
