package repositories_test

import (
	"context"

	. "github.com/onsi/gomega/gstruct"

	. "code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	buildv1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BuildpackRepository", func() {
	var (
		beforeCtx     context.Context
		buildpackRepo *BuildpackRepository
	)

	BeforeEach(func() {
		beforeCtx = context.Background()
	})

	Describe("GetBuildpacksForBuilder", func() {
		var clusterBuilder *buildv1alpha2.ClusterBuilder

		BeforeEach(func() {
			clusterBuilder = &buildv1alpha2.ClusterBuilder{
				ObjectMeta: metav1.ObjectMeta{
					Name: generateGUID(),
				},
				Spec: buildv1alpha2.ClusterBuilderSpec{
					BuilderSpec: buildv1alpha2.BuilderSpec{
						Tag: "registry/builder-image",
						Stack: corev1.ObjectReference{
							Kind: "ClusterStack",
							Name: "some-cluster-stack",
						},
						Store: corev1.ObjectReference{
							Kind: "ClusterStore",
							Name: "some-cluster-store",
						},
						Order: []buildv1alpha1.OrderEntry{
							{
								Group: []buildv1alpha1.BuildpackRef{
									newBuildpackRef("paketo-buildpacks/buildpack-1-1"),
								},
							},
							{
								Group: []buildv1alpha1.BuildpackRef{
									newBuildpackRef("paketo-buildpacks/buildpack-2-1"),
									newBuildpackRef("paketo-buildpacks/buildpack-2-2"),
									newBuildpackRef("paketo-buildpacks/buildpack-2-3"),
								},
							},
							{
								Group: []buildv1alpha1.BuildpackRef{
									newBuildpackRef("paketo-buildpacks/buildpack-3-1"),
								},
							},
						},
					},
					ServiceAccountRef: corev1.ObjectReference{
						Namespace: "some-namespace",
						Name:      "some-service-account",
					},
				},
			}

			Expect(k8sClient.Create(beforeCtx, clusterBuilder)).To(Succeed())

			clusterBuilderOrderStatus := []buildv1alpha1.OrderEntry{
				{
					Group: []buildv1alpha1.BuildpackRef{
						newBuildpackRef("paketo-buildpacks/buildpack-1-1", "1.1"),
					},
				},
				{
					Group: []buildv1alpha1.BuildpackRef{
						newBuildpackRef("paketo-buildpacks/buildpack-2-1", "2.1"),
						newBuildpackRef("paketo-buildpacks/buildpack-2-2", "2.2"),
						newBuildpackRef("paketo-buildpacks/buildpack-2-3", "2.3"),
					},
				},
				{
					Group: []buildv1alpha1.BuildpackRef{
						newBuildpackRef("paketo-buildpacks/buildpack-3-1", "3.1"),
					},
				},
			}
			clusterBuilder.Status.Order = clusterBuilderOrderStatus

			clusterBuilderStackStatus := buildv1alpha1.BuildStack{
				ID:       "io.buildpacks.stacks.bionic",
				RunImage: "index.docker.io/paketobuildpacks/run@sha256:79185c8427ebfed9b7df3e0fa12e101ec8b24aa899bbc541648d5923fb494084",
			}
			clusterBuilder.Status.Stack = clusterBuilderStackStatus

			Expect(k8sClient.Status().Update(beforeCtx, clusterBuilder)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), clusterBuilder)).To(Succeed())
		})

		Describe("List", func() {
			It("returns records matching the buildpacks of the ClusterBuilder and no error", func() {
				buildpackRepo = NewBuildpackRepository(userClientFactory)
				createClusterRoleBinding(beforeCtx, userName, spaceDeveloperRole.Name)

				buildpackRecords, err := buildpackRepo.GetBuildpacksForBuilder(context.Background(), authInfo, clusterBuilder.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(buildpackRecords).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name":     Equal("paketo-buildpacks/buildpack-1-1"),
						"Position": Equal(1),
						"Stack":    Equal("io.buildpacks.stacks.bionic"),
						"Version":  Equal("1.1"),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":     Equal("paketo-buildpacks/buildpack-2-1"),
						"Position": Equal(2),
						"Stack":    Equal("io.buildpacks.stacks.bionic"),
						"Version":  Equal("2.1"),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":     Equal("paketo-buildpacks/buildpack-3-1"),
						"Position": Equal(3),
						"Stack":    Equal("io.buildpacks.stacks.bionic"),
						"Version":  Equal("3.1"),
					}),
				))
			})

			When("insufficient perm", func() {
				It("fails to retrieve buildpack records", func() {
					buildpackRepo = NewBuildpackRepository(userClientFactory)
					_, err := buildpackRepo.GetBuildpacksForBuilder(context.Background(), authInfo, clusterBuilder.Name)
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})
})

func newBuildpackRef(id string, version ...string) buildv1alpha1.BuildpackRef {
	toReturn := buildv1alpha1.BuildpackRef{
		BuildpackInfo: buildv1alpha1.BuildpackInfo{
			Id: id,
		},
		Optional: true,
	}

	if len(version) > 0 {
		toReturn.BuildpackInfo.Version = version[0]
	}
	return toReturn
}
