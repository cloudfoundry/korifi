package integration_test

import (
	"maps"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers/appworkload/state"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("AppWorkload State", func() {
	var (
		stateCollector  *state.AppWorkloadStateCollector
		workloadState   map[string]korifiv1alpha1.InstanceState
		appWorkloadGUID string
		pod             *corev1.Pod
	)

	BeforeEach(func() {
		stateCollector = state.NewAppWorkloadStateCollector(k8sClient)

		appWorkloadGUID = uuid.NewString()
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaceName,
				Name:      uuid.NewString(),
				Labels: map[string]string{
					"apps.kubernetes.io/pod-index": "4",
					"korifi.cloudfoundry.org/guid": appWorkloadGUID,
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

	JustBeforeEach(func() {
		var err error
		workloadState, err = stateCollector.CollectState(ctx, appWorkloadGUID)
		Expect(err).NotTo(HaveOccurred())
	})

	It("reports state DOWN", func() {
		Expect(workloadState).To(Equal(map[string]korifiv1alpha1.InstanceState{
			"4": korifiv1alpha1.InstanceStateDown,
		}))
	})

	When("the pod is ready", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, k8sClient, pod, func() {
				pod.Status = corev1.PodStatus{
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				}
			})).To(Succeed())
		})

		It("reports state RUNNING", func() {
			Expect(workloadState).To(Equal(map[string]korifiv1alpha1.InstanceState{
				"4": korifiv1alpha1.InstanceStateRunning,
			}))
		})
	})

	When("the pod is scheduled", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, k8sClient, pod, func() {
				pod.Status = corev1.PodStatus{
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionTrue,
					}},
				}
			})).To(Succeed())
		})

		It("reports state STARTING", func() {
			Expect(workloadState).To(Equal(map[string]korifiv1alpha1.InstanceState{
				"4": korifiv1alpha1.InstanceStateStarting,
			}))
		})

		When("the pod has a crashed container", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, pod, func() {
					pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason: "CrashLoopBackOff",
							},
						},
					}}
				})).To(Succeed())
			})

			It("reports state CRASHED", func() {
				Expect(workloadState).To(Equal(map[string]korifiv1alpha1.InstanceState{
					"4": korifiv1alpha1.InstanceStateCrashed,
				}))
			})
		})
	})

	When("the app workload has two instances", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespaceName,
					Name:      uuid.NewString(),
					Labels: map[string]string{
						"apps.kubernetes.io/pod-index": "5",
						"korifi.cloudfoundry.org/guid": appWorkloadGUID,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "pod-container",
						Image: "pod/image",
					}},
				},
			})).To(Succeed())
		})

		It("reports all instances state", func() {
			Expect(maps.Keys(workloadState)).To(ConsistOf("4", "5"))
		})
	})
})
