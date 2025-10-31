package controllers_test

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BuilderInfoReconciler", func() {
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
		info           *v1alpha1.BuilderInfo
	)

	BeforeEach(func() {
		clusterBuilder = &buildv1alpha2.ClusterBuilder{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterBuilderName,
			},
		}

		Expect(adminClient.Create(context.Background(), clusterBuilder)).To(Succeed())
		Expect(k8s.Patch(ctx, adminClient, clusterBuilder, func() {
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
			clusterBuilder.Status.Conditions = append(clusterBuilder.Status.Conditions, corev1alpha1.Condition{
				Type:   "Ready",
				Status: "True",
			})
		})).To(Succeed())

		info = &v1alpha1.BuilderInfo{
			ObjectMeta: metav1.ObjectMeta{
				Name:       controllers.BuilderInfoName,
				Namespace:  rootNamespace.Name,
				Generation: 1,
			},
		}
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(context.Background(), info)).To(Succeed())
	})

	It("sets the buildpacks on the BuilderInfo", func() {
		Eventually(func(g Gomega) []v1alpha1.BuilderInfoStatusBuildpack {
			g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
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
	})

	It("sets the stacks on the BuilderInfo", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
			g.Expect(info.Status.Stacks).To(HaveLen(1))
		}).Should(Succeed())

		Expect(info.Status.Stacks[0]).To(HaveField("Name", Equal(stack)))
	})

	It("marks the BuilderInfo as ready", func() {
		Eventually(func(g Gomega) []v1alpha1.BuilderInfoStatusBuildpack {
			g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
			return info.Status.Buildpacks
		}).ShouldNot(BeEmpty())

		readyCondition := meta.FindStatusCondition(info.Status.Conditions, "Ready")
		Expect(readyCondition).NotTo(BeNil())
		Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(readyCondition.ObservedGeneration).To(Equal(info.Generation))
	})

	It("sets the ObservedGeneration status field", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
			g.Expect(info.Status.ObservedGeneration).To(Equal(info.Generation))
		}).Should(Succeed())
	})

	When("the builder info is being deleted gracefully", func() {
		BeforeEach(func() {
			info.Finalizers = []string{"do-not-delete-yet"}
		})

		JustBeforeEach(func() {
			Expect(k8sManager.GetClient().Delete(ctx, info)).To(Succeed())
		})

		It("does not reconcile the process", func() {
			helpers.EventuallyShouldHold(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(info), info)).To(Succeed())
				g.Expect(info.Status.ObservedGeneration).NotTo(Equal(info.Generation))
			})
		})
	})

	When("the cluster builder status is not ready", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, clusterBuilder, func() {
				ok := false
				for i, cond := range clusterBuilder.Status.Conditions {
					if cond.Type == "Ready" {
						clusterBuilder.Status.Conditions[i].Status = v1.ConditionFalse
						clusterBuilder.Status.Conditions[i].Message = "something happened"
						ok = true
						break
					}
				}
				Expect(ok).To(BeTrue())
			})).To(Succeed())
		})

		It("marks the BuilderInfo as not ready", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
				readyCondition := meta.FindStatusCondition(info.Status.Conditions, "Ready")
				g.Expect(readyCondition).NotTo(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Reason).To(Equal("ClusterBuilderNotReady"))
				g.Expect(readyCondition.Message).To(ContainSubstring(fmt.Sprintf("ClusterBuilder %q is not ready: something happened", clusterBuilderName)))
				g.Expect(readyCondition.ObservedGeneration).To(Equal(info.Generation))
			}).Should(Succeed())
		})
	})

	When("the cluster builder status is missing a message", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, clusterBuilder, func() {
				ok := false
				for i, cond := range clusterBuilder.Status.Conditions {
					if cond.Type == "Ready" {
						clusterBuilder.Status.Conditions[i].Status = v1.ConditionFalse
						clusterBuilder.Status.Conditions[i].Message = ""
						ok = true
						break
					}
				}
				Expect(ok).To(BeTrue())
			})).To(Succeed())
		})

		It("marks the BuilderInfo as not ready", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
				readyCondition := meta.FindStatusCondition(info.Status.Conditions, "Ready")
				g.Expect(readyCondition).NotTo(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCondition.Reason).To(Equal("ClusterBuilderNotReady"))
				g.Expect(readyCondition.ObservedGeneration).To(Equal(info.Generation))
			}).Should(Succeed())
		})
	})

	When("the ClusterBuilder changes after the BuilderInfo has reconciled", func() {
		const (
			rustBuildpackName    = "rust"
			rustBuildpackVersion = "42.0"
			newStack             = "ubuntu-lucky-leopard"
		)

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
				g.Expect(info.Status.Buildpacks).NotTo(BeEmpty())
			}).Should(Succeed())

			Expect(k8s.Patch(ctx, adminClient, clusterBuilder, func() {
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
					Status: corev1alpha1.Status{
						Conditions: []corev1alpha1.Condition{{
							Type:   "Ready",
							Status: "False",
						}},
					},
				}
			})).To(Succeed())
		})

		It("updates the buildpacks on the BuilderInfo", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
				g.Expect(info.Status.Buildpacks).To(HaveLen(4))
			}).Should(Succeed())

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
			Expect(meta.IsStatusConditionFalse(info.Status.Conditions, "Ready")).To(BeTrue())
		})

		It("updates the stacks on the BuilderInfo", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
				g.Expect(info.Status.Stacks).To(ConsistOf(HaveField("Name", newStack)))
			}).Should(Succeed())
		})
	})

	When("a BuilderInfo with the wrong name exists", func() {
		BeforeEach(func() {
			info.Name = "some-other-build-reconciler"
		})

		It("doesn't modify that resource", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
				g.Expect(info.Status.Buildpacks).To(BeEmpty())
			}).Should(Succeed())
		})
	})

	When("the BuilderInfo is in a namespace other than the root namespace", func() {
		BeforeEach(func() {
			wrongNamespace := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: PrefixedGUID("wrong-namespace"),
				},
			}
			Expect(adminClient.Create(context.Background(), wrongNamespace)).To(Succeed())

			info.Namespace = wrongNamespace.Name
		})

		It("doesn't modify that resource", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
				g.Expect(info.Status.Buildpacks).To(BeEmpty())
			}).Should(Succeed())
		})
	})

	When("the ClusterBuilder doesn't exist", func() {
		BeforeEach(func() {
			Expect(adminClient.Delete(ctx, clusterBuilder)).To(Succeed())
		})

		It("doesn't set the buildpacks on the BuilderInfo", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(info), info)).To(Succeed())
				g.Expect(info.Status.Buildpacks).To(BeEmpty())
			}).Should(Succeed())

			readyCondition := meta.FindStatusCondition(info.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("ClusterBuilderMissing"))
			Expect(readyCondition.ObservedGeneration).To(Equal(info.Generation))
		})
	})
})
