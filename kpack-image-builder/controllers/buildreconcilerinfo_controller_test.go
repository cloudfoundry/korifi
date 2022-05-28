package controllers_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"
)

var _ = Describe("BuildReconcilerInfoReconciler", Serial, func() {
	const (
		stack                  = "ubuntu-kreepy-koala"
		golangBuildpackName    = "golang"
		golangBuildpackVersion = "1.2.3"
		pythonBuildpackName    = "python"
		pythonBuildpackVersion = "2.3.4"
		javaBuildpackName      = "java"
		javaBuildpackVersion   = "3.4"
	)

	var (
		clusterBuilder *buildv1alpha2.ClusterBuilder
		info           *v1alpha1.BuildReconcilerInfo
	)

	BeforeEach(func() {
		info = nil
	})

	AfterEach(func() {
		if info != nil {
			Expect(k8sClient.Delete(context.Background(), info)).To(Succeed())
		}
	})

	When("the ClusterBuilder exists", func() {
		BeforeEach(func() {
			clusterBuilder = &buildv1alpha2.ClusterBuilder{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterBuilderName,
				},
			}

			Expect(k8sClient.Create(context.Background(), clusterBuilder)).To(Succeed())

			clusterBuilder.Status = buildv1alpha2.BuilderStatus{
				Order: []corev1alpha1.OrderEntry{
					{Group: []corev1alpha1.BuildpackRef{
						{BuildpackInfo: corev1alpha1.BuildpackInfo{Id: golangBuildpackName, Version: golangBuildpackVersion}},
					}},
					{Group: []corev1alpha1.BuildpackRef{
						{BuildpackInfo: corev1alpha1.BuildpackInfo{Id: pythonBuildpackName, Version: pythonBuildpackVersion}},
					}},
					{Group: []corev1alpha1.BuildpackRef{
						{BuildpackInfo: corev1alpha1.BuildpackInfo{Id: javaBuildpackName, Version: javaBuildpackVersion}},
					}},
				},
				Stack: corev1alpha1.BuildStack{
					ID: stack,
				},
			}
			Expect(k8sClient.Status().Update(context.Background(), clusterBuilder)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), clusterBuilder)).To(Succeed())
		})

		When("the BuildReconcilerInfo is first created", func() {
			JustBeforeEach(func() {
				info = &v1alpha1.BuildReconcilerInfo{
					ObjectMeta: metav1.ObjectMeta{
						Name:      controllers.BuildReconcilerInfoName,
						Namespace: rootNamespace.Name,
					},
				}
				Expect(k8sClient.Create(context.Background(), info)).To(Succeed())
			})

			It("sets the buildpacks on the BuildReconcilerInfo", func() {
				lookupKey := types.NamespacedName{
					Name:      controllers.BuildReconcilerInfoName,
					Namespace: rootNamespace.Name,
				}
				Eventually(func(g Gomega) []v1alpha1.BuildReconcilerInfoStatusBuildpack {
					g.Expect(k8sClient.Get(context.Background(), lookupKey, info)).To(Succeed())
					return info.Status.Buildpacks
				}).ShouldNot(BeEmpty())

				Expect(info.Status.Buildpacks[0]).To(MatchFields(IgnoreExtras, Fields{
					"Name":    Equal(golangBuildpackName),
					"Version": Equal(golangBuildpackVersion),
					"Stack":   Equal(stack),
				}))
				Expect(info.Status.Buildpacks[1]).To(MatchFields(IgnoreExtras, Fields{
					"Name":    Equal(pythonBuildpackName),
					"Version": Equal(pythonBuildpackVersion),
					"Stack":   Equal(stack),
				}))
				Expect(info.Status.Buildpacks[2]).To(MatchFields(IgnoreExtras, Fields{
					"Name":    Equal(javaBuildpackName),
					"Version": Equal(javaBuildpackVersion),
					"Stack":   Equal(stack),
				}))

				readyCondition := meta.FindStatusCondition(info.Status.Conditions, "Ready")
				Expect(readyCondition).NotTo(BeNil())
				Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
				Expect(readyCondition.Reason).To(Equal("cluster_builder_exists"))
				Expect(readyCondition.Message).To(ContainSubstring(clusterBuilderName))
			})
		})

		When("the ClusterBuilder changes after the BuildReconcilerInfo has reconciled", func() {
			const (
				rustBuildpackName    = "rust"
				rustBuildpackVersion = "42.0"
				newStack             = "ubuntu-lucky-leopard"
			)

			BeforeEach(func() {
				info = &v1alpha1.BuildReconcilerInfo{
					ObjectMeta: metav1.ObjectMeta{
						Name:      controllers.BuildReconcilerInfoName,
						Namespace: rootNamespace.Name,
					},
				}
				Expect(k8sClient.Create(context.Background(), info)).To(Succeed())

				lookupKey := types.NamespacedName{
					Name:      controllers.BuildReconcilerInfoName,
					Namespace: rootNamespace.Name,
				}
				Eventually(func(g Gomega) []v1alpha1.BuildReconcilerInfoStatusBuildpack {
					g.Expect(k8sClient.Get(context.Background(), lookupKey, info)).To(Succeed())
					return info.Status.Buildpacks
				}).ShouldNot(BeEmpty())
			})

			JustBeforeEach(func() {
				clusterBuilder.Status = buildv1alpha2.BuilderStatus{
					Order: []corev1alpha1.OrderEntry{
						{Group: []corev1alpha1.BuildpackRef{
							{BuildpackInfo: corev1alpha1.BuildpackInfo{Id: javaBuildpackName, Version: javaBuildpackVersion}},
						}},
						{Group: []corev1alpha1.BuildpackRef{
							{BuildpackInfo: corev1alpha1.BuildpackInfo{Id: golangBuildpackName, Version: golangBuildpackVersion}},
						}},
						{Group: []corev1alpha1.BuildpackRef{
							{BuildpackInfo: corev1alpha1.BuildpackInfo{Id: pythonBuildpackName, Version: pythonBuildpackVersion}},
						}},
						{Group: []corev1alpha1.BuildpackRef{
							{BuildpackInfo: corev1alpha1.BuildpackInfo{Id: rustBuildpackName, Version: rustBuildpackVersion}},
						}},
					},
					Stack: corev1alpha1.BuildStack{
						ID: newStack,
					},
				}
				Expect(k8sClient.Status().Update(context.Background(), clusterBuilder)).To(Succeed())
			})

			It("updates the buildpacks on the BuildReconcilerInfo", func() {
				lookupKey := types.NamespacedName{
					Name:      controllers.BuildReconcilerInfoName,
					Namespace: rootNamespace.Name,
				}
				Eventually(func(g Gomega) []v1alpha1.BuildReconcilerInfoStatusBuildpack {
					g.Expect(k8sClient.Get(context.Background(), lookupKey, info)).To(Succeed())
					return info.Status.Buildpacks
				}).Should(HaveLen(4))

				Expect(info.Status.Buildpacks[0]).To(MatchFields(IgnoreExtras, Fields{
					"Name":    Equal(javaBuildpackName),
					"Version": Equal(javaBuildpackVersion),
					"Stack":   Equal(newStack),
				}))
				Expect(info.Status.Buildpacks[1]).To(MatchFields(IgnoreExtras, Fields{
					"Name":    Equal(golangBuildpackName),
					"Version": Equal(golangBuildpackVersion),
					"Stack":   Equal(newStack),
				}))
				Expect(info.Status.Buildpacks[2]).To(MatchFields(IgnoreExtras, Fields{
					"Name":    Equal(pythonBuildpackName),
					"Version": Equal(pythonBuildpackVersion),
					"Stack":   Equal(newStack),
				}))
				Expect(info.Status.Buildpacks[3]).To(MatchFields(IgnoreExtras, Fields{
					"Name":    Equal(rustBuildpackName),
					"Version": Equal(rustBuildpackVersion),
					"Stack":   Equal(newStack),
				}))
				Expect(meta.IsStatusConditionTrue(info.Status.Conditions, "Ready")).To(BeTrue())
			})
		})

		When("a BuildReconcilerInfo with the wrong name exists", func() {
			JustBeforeEach(func() {
				info = &v1alpha1.BuildReconcilerInfo{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-other-build-reconciler",
						Namespace: rootNamespace.Name,
					},
				}
				Expect(k8sClient.Create(context.Background(), info)).To(Succeed())
			})

			It("doesn't modify that resource", func() {
				lookupKey := types.NamespacedName{
					Name:      info.Name,
					Namespace: info.Namespace,
				}
				Consistently(func(g Gomega) []v1alpha1.BuildReconcilerInfoStatusBuildpack {
					g.Expect(k8sClient.Get(context.Background(), lookupKey, info)).To(Succeed())
					return info.Status.Buildpacks
				}).Should(BeEmpty())
			})
		})

		When("the BuildReconcilerInfo is in a namespace other than the root namespace", func() {
			JustBeforeEach(func() {
				wrongNamespace := &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: PrefixedGUID("wrong-namespace"),
					},
				}
				Expect(k8sClient.Create(context.Background(), wrongNamespace)).To(Succeed())

				info = &v1alpha1.BuildReconcilerInfo{
					ObjectMeta: metav1.ObjectMeta{
						Name:      controllers.BuildReconcilerInfoName,
						Namespace: wrongNamespace.Name,
					},
				}
				Expect(k8sClient.Create(context.Background(), info)).To(Succeed())
			})

			It("doesn't modify that resource", func() {
				lookupKey := types.NamespacedName{
					Name:      info.Name,
					Namespace: info.Namespace,
				}
				Consistently(func(g Gomega) []v1alpha1.BuildReconcilerInfoStatusBuildpack {
					g.Expect(k8sClient.Get(context.Background(), lookupKey, info)).To(Succeed())
					return info.Status.Buildpacks
				}).Should(BeEmpty())
			})
		})
	})

	When("the ClusterBuilder doesn't exist", func() {
		var wrongClusterBuilder *buildv1alpha2.ClusterBuilder

		BeforeEach(func() {
			info = &v1alpha1.BuildReconcilerInfo{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controllers.BuildReconcilerInfoName,
					Namespace: rootNamespace.Name,
				},
			}
			Expect(k8sClient.Create(context.Background(), info)).To(Succeed())

			wrongClusterBuilder = &buildv1alpha2.ClusterBuilder{
				ObjectMeta: metav1.ObjectMeta{
					Name: "wrong-cluster-builder",
				},
			}

			Expect(k8sClient.Create(context.Background(), wrongClusterBuilder)).To(Succeed())

			wrongClusterBuilder.Status = buildv1alpha2.BuilderStatus{
				Order: []corev1alpha1.OrderEntry{
					{Group: []corev1alpha1.BuildpackRef{
						{BuildpackInfo: corev1alpha1.BuildpackInfo{Id: golangBuildpackName, Version: golangBuildpackVersion}},
					}},
				},
				Stack: corev1alpha1.BuildStack{
					ID: stack,
				},
			}
			Expect(k8sClient.Status().Update(context.Background(), wrongClusterBuilder)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), wrongClusterBuilder)).To(Succeed())
		})

		It("doesn't set the buildpacks on the BuildReconcilerInfo", func() {
			lookupKey := types.NamespacedName{
				Name:      info.Name,
				Namespace: info.Namespace,
			}
			Consistently(func(g Gomega) []v1alpha1.BuildReconcilerInfoStatusBuildpack {
				g.Expect(k8sClient.Get(context.Background(), lookupKey, info)).To(Succeed())
				return info.Status.Buildpacks
			}).Should(BeEmpty())

			readyCondition := meta.FindStatusCondition(info.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("cluster_builder_missing"))
			Expect(readyCondition.Message).To(ContainSubstring(clusterBuilderName))
		})

		When("the ClusterBuilder is created", func() {
			BeforeEach(func() {
				clusterBuilder = &buildv1alpha2.ClusterBuilder{
					ObjectMeta: metav1.ObjectMeta{
						Name: clusterBuilderName,
					},
				}

				Expect(k8sClient.Create(context.Background(), clusterBuilder)).To(Succeed())

				clusterBuilder.Status = buildv1alpha2.BuilderStatus{
					Order: []corev1alpha1.OrderEntry{
						{Group: []corev1alpha1.BuildpackRef{
							{BuildpackInfo: corev1alpha1.BuildpackInfo{Id: pythonBuildpackName, Version: pythonBuildpackVersion}},
						}},
					},
					Stack: corev1alpha1.BuildStack{
						ID: stack,
					},
				}
				Expect(k8sClient.Status().Update(context.Background(), clusterBuilder)).To(Succeed())

			})

			It("sets the buildpacks on the BuildReconcilerInfo", func() {
				lookupKey := types.NamespacedName{
					Name:      controllers.BuildReconcilerInfoName,
					Namespace: rootNamespace.Name,
				}
				Eventually(func(g Gomega) []v1alpha1.BuildReconcilerInfoStatusBuildpack {
					g.Expect(k8sClient.Get(context.Background(), lookupKey, info)).To(Succeed())
					return info.Status.Buildpacks
				}).ShouldNot(BeEmpty())

				Expect(info.Status.Buildpacks[0]).To(MatchFields(IgnoreExtras, Fields{
					"Name":    Equal(pythonBuildpackName),
					"Version": Equal(pythonBuildpackVersion),
					"Stack":   Equal(stack),
				}))
				Expect(meta.IsStatusConditionTrue(info.Status.Conditions, "Ready")).To(BeTrue())
			})
		})
	})
})
