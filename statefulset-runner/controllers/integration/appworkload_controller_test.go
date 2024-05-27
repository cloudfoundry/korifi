package integration_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AppWorkloadsController", func() {
	var (
		ctx           context.Context
		appWorkload   *korifiv1alpha1.AppWorkload
		namespaceName string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespaceName = uuid.NewString()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		})).To(Succeed())

		appWorkload = &korifiv1alpha1.AppWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespaceName,
			},
			Spec: korifiv1alpha1.AppWorkloadSpec{
				GUID:    uuid.NewString(),
				Version: uuid.NewString(),
				AppGUID: uuid.NewString(),

				ProcessType: uuid.NewString(),
				Image:       uuid.NewString(),
				Instances:   5,
				RunnerName:  "statefulset-runner",
			},
		}
	})

	getStatefulsetForAppWorkload := func(g Gomega) appsv1.StatefulSet {
		stsetList := appsv1.StatefulSetList{}
		g.Eventually(func(g Gomega) {
			g.Expect(k8sClient.List(ctx, &stsetList, client.MatchingLabels{
				controllers.LabelGUID: appWorkload.Spec.GUID,
			})).To(Succeed())
			g.Expect(stsetList.Items).To(HaveLen(1))
		}).Should(Succeed())

		return stsetList.Items[0]
	}

	When("AppWorkload is created", func() {
		var err error

		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, appWorkload)).To(Succeed())
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates the statefulset", func() {
			Expect(getStatefulsetForAppWorkload(Default).Namespace).To(Equal(namespaceName))
		})

		It("the created statefulset contains an owner reference to our appworkload", func() {
			statefulset := getStatefulsetForAppWorkload(Default)

			Expect(statefulset.OwnerReferences).To(HaveLen(1))
			Expect(statefulset.OwnerReferences[0].Kind).To(Equal("AppWorkload"))
			Expect(statefulset.OwnerReferences[0].Name).To(Equal(appWorkload.Name))
		})

		It("creates the pod disruption budget", func() {
			statefulSet := getStatefulsetForAppWorkload(Default)
			pdb := new(policyv1.PodDisruptionBudget)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulSet.Name, Namespace: namespaceName}, pdb)).To(Succeed())
			}).Should(Succeed())
			Expect(*pdb.Spec.MinAvailable).To(Equal(intstr.FromString("50%")))
		})

		When("the statefulset replicas is set", func() {
			JustBeforeEach(func() {
				statefulset := getStatefulsetForAppWorkload(Default)
				updatedStatefulset := statefulset.DeepCopy()
				updatedStatefulset.Status.Replicas = 1

				Expect(k8sClient.Status().Patch(ctx, updatedStatefulset, client.MergeFrom(&statefulset))).To(Succeed())
			})

			It("updates workload actual instances", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
					g.Expect(appWorkload.Status.ActualInstances).To(BeEquivalentTo(1))
				}).Should(Succeed())
			})
		})

		When("the appworkload runner name is not 'statefulset-runner'", func() {
			BeforeEach(func() {
				appWorkload.Spec.RunnerName = "another-runner"
			})

			It("does not reconcile it", func() {
				Consistently(func(g Gomega) {
					stsetList := appsv1.StatefulSetList{}
					g.Expect(k8sClient.List(ctx, &stsetList, client.MatchingLabels{
						controllers.LabelGUID: appWorkload.Spec.GUID,
					})).To(Succeed())
					g.Expect(stsetList.Items).To(BeEmpty())

					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
					g.Expect(meta.FindStatusCondition(appWorkload.Status.Conditions, korifiv1alpha1.StatusConditionReady)).To(BeNil())
				}).Should(Succeed())
			})
		})
	})

	When("AppWorkload update", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, appWorkload)).To(Succeed())
		})

		JustBeforeEach(func() {
			Expect(k8s.Patch(ctx, k8sClient, appWorkload, func() {
				appWorkload.Spec.Instances = 2
				appWorkload.Spec.Resources.Requests = corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1024m"),
					corev1.ResourceMemory: resource.MustParse("10Mi"),
				}
				appWorkload.Spec.Resources.Limits = corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("10Mi"),
				}
			})).To(Succeed())
		})

		It("updates the StatefulSet", func() {
			Eventually(func(g Gomega) {
				statefulSet := getStatefulsetForAppWorkload(g)
				g.Expect(statefulSet.Spec.Replicas).To(gstruct.PointTo(BeNumerically("==", 2)))
				g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String()).To(Equal("10Mi"))
				g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String()).To(Equal("1024m"))
				g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().IsZero()).To(BeTrue())
			}).Should(Succeed())
		})
	})
})
