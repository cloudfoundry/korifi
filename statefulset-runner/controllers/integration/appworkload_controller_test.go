package integration_test

import (
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers/appworkload"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"code.cloudfoundry.org/korifi/tests/helpers"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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
				Finalizers: []string{
					finalizer.AppWorkloadFinalizerName,
				},
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
		GinkgoHelper()

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

		When("the statefulset replicas are set", func() {
			JustBeforeEach(func() {
				statefulset := tools.PtrTo(getStatefulsetForAppWorkload(Default))
				Expect(k8s.Patch(ctx, k8sClient, statefulset, func() {
					statefulset.Status.Replicas = 1
					statefulset.Status.ReadyReplicas = 1
				})).To(Succeed())
			})

			It("updates workload actual instances based on ready replicas", func() {
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
						g.Expect(appWorkload.Status.InstancesStatus).To(HaveKeyWithValue("4", korifiv1alpha1.InstanceStatus{
							State: korifiv1alpha1.InstanceStateDown,
						}))
					}).Should(Succeed())
				})

				When("the instance pod starts", func() {
					JustBeforeEach(func() {
						Eventually(func(g Gomega) {
							g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
							g.Expect(appWorkload.Status.InstancesStatus).To(HaveKeyWithValue("4", korifiv1alpha1.InstanceStatus{
								State: korifiv1alpha1.InstanceStateDown,
							}))
						}).Should(Succeed())

						Expect(k8s.Patch(ctx, k8sClient, pod, func() {
							pod.Status = corev1.PodStatus{
								Conditions: []corev1.PodCondition{{
									Type:               corev1.PodReady,
									Status:             corev1.ConditionTrue,
									LastTransitionTime: metav1.NewTime(time.UnixMilli(2000).UTC()),
								}},
							}
						})).To(Succeed())
					})

					It("updates workload instances state", func() {
						Eventually(func(g Gomega) {
							g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
							g.Expect(appWorkload.Status.InstancesStatus).To(HaveKeyWithValue("4", MatchAllFields(Fields{
								"State": BeEquivalentTo(korifiv1alpha1.InstanceStateRunning),
								"Timestamp": PointTo(MatchAllFields(Fields{
									"Time": BeTemporally("==", time.UnixMilli(2000).UTC()),
								})),
							})))
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

	When("AppWorkload is updated", func() {
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
				g.Expect(statefulSet.Spec.Replicas).To(PointTo(BeNumerically("==", 2)))
				g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String()).To(Equal("10Mi"))
				g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String()).To(Equal("1024m"))
				g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().IsZero()).To(BeTrue())
			}).Should(Succeed())
		})
	})

	When("AppWorkload is deleted", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, appWorkload)).To(Succeed())
		})

		JustBeforeEach(func() {
			// For deletion test we want to request deletion and verify the behaviour when finalization fails.
			// Therefore we use the standard k8s client instnce of `adminClient` as it ensures that the object is deleted
			Expect(k8sManager.GetClient().Delete(ctx, appWorkload)).To(Succeed())
		})

		It("deletes the appworkload", func() {
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})

		It("deletes the statefulset", func() {
			stsetList := appsv1.StatefulSetList{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.List(ctx, &stsetList, client.MatchingLabels{
					controllers.LabelGUID: appWorkload.Spec.GUID,
				})).To(Succeed())
				g.Expect(stsetList.Items).To(BeEmpty())
			}).Should(Succeed())
		})

		When("the statefulset cannot be deleted", func() {
			BeforeEach(func() {
				converter := appworkload.NewAppWorkloadToStatefulsetConverter(k8sManager.GetScheme())
				stSet, err := converter.Convert(appWorkload)
				Expect(err).NotTo(HaveOccurred())

				stSet.Finalizers = []string{"korifi.cloudfoundry.org/do-not-delete"}
				Expect(k8sClient.Create(ctx, stSet)).To(Succeed())
			})

			It("does not delete the appworkload", func() {
				helpers.EventuallyShouldHold(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
					g.Expect(appWorkload.DeletionTimestamp).NotTo(BeZero())
				})
			})

			When("the statefulset replicas are set", func() {
				JustBeforeEach(func() {
					statefulset := getStatefulsetForAppWorkload(Default)
					Expect(k8s.Patch(ctx, k8sClient, &statefulset, func() {
						statefulset.Status.Replicas = 3
					})).To(Succeed())
				})

				It("updates workload actual instances based on statefulset replicas", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
						g.Expect(appWorkload.Status.ActualInstances).To(BeEquivalentTo(3))
					}).Should(Succeed())
				})

				It("sets not ready condition on appworkload", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(appWorkload), appWorkload)).To(Succeed())
						g.Expect(appWorkload.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionFalse)),
							HasReason(Equal("StillRunning")),
							HasMessage(Equal("3 instances still running")),
						)))
					}).Should(Succeed())
				})
			})
		})
	})
})
