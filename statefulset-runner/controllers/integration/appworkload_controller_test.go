package integration_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AppWorkloadsController", func() {
	var appWorkload *korifiv1alpha1.AppWorkload

	BeforeEach(func() {
		appWorkload = &korifiv1alpha1.AppWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Name:       uuid.NewString(),
				Namespace:  namespaceName,
				Generation: 1,
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
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, appWorkload)).To(Succeed())
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

		It("sets the observed generation", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
				g.Expect(appWorkload.Status.ObservedGeneration).To(BeEquivalentTo(1))
			}).Should(Succeed())
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
				statefulset := tools.PtrTo(getStatefulsetForAppWorkload(Default))
				Expect(k8s.Patch(ctx, k8sClient, statefulset, func() {
					statefulset.Status.Replicas = 1
				})).To(Succeed())
			})

			It("updates workload actual instances", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
					g.Expect(appWorkload.Status.ActualInstances).To(BeEquivalentTo(1))
				}).Should(Succeed())
			})

			When("instance pods are available", func() {
				var pod *corev1.Pod

				BeforeEach(func() {
					pod = &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespaceName,
							Name:      uuid.NewString(),
							Labels: map[string]string{
								"apps.kubernetes.io/pod-index":             "4",
								"korifi.cloudfoundry.org/guid":             appWorkload.Spec.GUID,
								"korifi.cloudfoundry.org/appworkload-guid": appWorkload.Name,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "pod-container",
								Image: "pod/image",
							}},
						},
					}
					Expect(k8sClient.Create(ctx, pod)).To(Succeed())
				})

				It("updates workload instances state", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
						g.Expect(appWorkload.Status.InstancesState).To(HaveKeyWithValue("4", korifiv1alpha1.InstanceStateDown))
					}).Should(Succeed())
				})

				When("the instance pod starts", func() {
					JustBeforeEach(func() {
						Eventually(func(g Gomega) {
							g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
							g.Expect(appWorkload.Status.InstancesState).To(HaveKeyWithValue("4", korifiv1alpha1.InstanceStateDown))
						}).Should(Succeed())

						Expect(k8s.Patch(ctx, k8sClient, pod, func() {
							pod.Status = corev1.PodStatus{
								Conditions: []corev1.PodCondition{{
									Type:   corev1.PodReady,
									Status: corev1.ConditionTrue,
								}},
							}
						})).To(Succeed())
					})

					It("updates workload instances state", func() {
						Eventually(func(g Gomega) {
							g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
							g.Expect(appWorkload.Status.InstancesState).To(HaveKeyWithValue("4", korifiv1alpha1.InstanceStateRunning))
						}).Should(Succeed())
					})
				})
			})
		})

		When("the appworkload runner name is not 'statefulset-runner'", func() {
			var anotherAppWorkload *korifiv1alpha1.AppWorkload

			BeforeEach(func() {
				anotherAppWorkload = &korifiv1alpha1.AppWorkload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: namespaceName,
					},
					Spec: korifiv1alpha1.AppWorkloadSpec{
						RunnerName: "not-statefulset-runner",
					},
				}
				Expect(k8sClient.Create(ctx, anotherAppWorkload)).To(Succeed())
			})

			It("does not reconcile it", func() {
				Consistently(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(anotherAppWorkload), anotherAppWorkload)).To(Succeed())
					g.Expect(anotherAppWorkload.Status).To(BeZero())
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
