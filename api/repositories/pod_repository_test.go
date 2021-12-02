package repositories_test

import (
	"context"
	"time"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("PodRepository", func() {
	const (
		appGUID  = "the-app-guid"
		pod1Name = "some-pod-1"
		pod2Name = "some-pod-2"
	)

	var (
		podRepo   *PodRepo
		ctx       context.Context
		spaceGUID string
		namespace *corev1.Namespace
		pod1      *corev1.Pod
		pod2      *corev1.Pod
	)

	BeforeEach(func() {
		spaceGUID = uuid.NewString()

		podRepo = NewPodRepo(k8sClient)

		ctx = context.Background()

		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}}
		Expect(
			k8sClient.Create(ctx, namespace),
		).To(Succeed())

		pod1 = createPodDef(pod1Name, spaceGUID, appGUID, "0")
		pod2 = createPodDef(pod2Name, spaceGUID, appGUID, "1")
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	Describe("FetchPodStatsByAppGUID", func() {
		var message FetchPodStatsMessage

		const (
			pod3Name = "some-other-pod-1"
			pod4Name = "some-pod-4"
		)

		BeforeEach(func() {
			Expect(
				k8sClient.Create(ctx, pod1),
			).To(Succeed())

			pod1.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
						Ready: true,
					},
				},
			}
			Expect(
				k8sClient.Status().Update(ctx, pod1),
			).To(Succeed())

			Expect(
				k8sClient.Create(ctx, pod2),
			).To(Succeed())
			Expect(
				k8sClient.Create(ctx, createPodDef(pod3Name, spaceGUID, uuid.NewString(), "0")),
			)
		})

		When("All required pods exists", func() {
			BeforeEach(func() {
				message = FetchPodStatsMessage{
					Namespace:   spaceGUID,
					AppGUID:     "the-app-guid",
					Instances:   2,
					ProcessType: "web",
				}
			})
			It("Fetches all the pods and sets the appropriate state", func() {
				records, err := podRepo.FetchPodStatsByAppGUID(ctx, authInfo, message)
				Expect(err).NotTo(HaveOccurred())
				Expect(records).To(HaveLen(2))
				Expect(records).To(ConsistOf(
					[]PodStatsRecord{
						{
							Type:  "web",
							Index: 0,
							State: "RUNNING",
						},
						{
							Type:  "web",
							Index: 1,
							State: "DOWN",
						},
					},
				))
			})
		})

		When("Some pods are missing", func() {
			BeforeEach(func() {
				message = FetchPodStatsMessage{
					Namespace:   spaceGUID,
					AppGUID:     "the-app-guid",
					Instances:   3,
					ProcessType: "web",
				}
			})
			It("Fetches pods and sets the appropriate state", func() {
				records, err := podRepo.FetchPodStatsByAppGUID(ctx, authInfo, message)
				Expect(err).NotTo(HaveOccurred())
				Expect(records).To(HaveLen(3))
				Expect(records).To(ConsistOf(
					[]PodStatsRecord{
						{
							Type:  "web",
							Index: 0,
							State: "RUNNING",
						},
						{
							Type:  "web",
							Index: 1,
							State: "DOWN",
						},
						{
							Type:  "web",
							Index: 2,
							State: "DOWN",
						},
					},
				))
			})
		})

		When("A pod is in pending state", func() {
			var pod3 *corev1.Pod

			BeforeEach(func() {
				message = FetchPodStatsMessage{
					Namespace:   spaceGUID,
					AppGUID:     "the-app-guid",
					Instances:   3,
					ProcessType: "web",
				}

				pod3 = createPodDef(pod4Name, spaceGUID, appGUID, "2")
				Expect(
					k8sClient.Create(ctx, pod3),
				).To(Succeed())

				pod3.Status = corev1.PodStatus{
					Phase: corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
							Ready: false,
						},
					},
				}

				Expect(
					k8sClient.Status().Update(ctx, pod3),
				).To(Succeed())
			})

			It("fetches pods and sets the appropriate state", func() {
				records, err := podRepo.FetchPodStatsByAppGUID(ctx, authInfo, message)
				Expect(err).NotTo(HaveOccurred())
				Expect(records).To(HaveLen(3))
				Expect(records).To(ConsistOf(
					[]PodStatsRecord{
						{
							Type:  "web",
							Index: 0,
							State: "RUNNING",
						},
						{
							Type:  "web",
							Index: 1,
							State: "DOWN",
						},
						{
							Type:  "web",
							Index: 2,
							State: "STARTING",
						},
					},
				))
			})
		})
	})

	Describe("WatchPodsForTermination", func() {
		BeforeEach(func() {
			Expect(
				k8sClient.Create(ctx, pod1),
			).To(Succeed())

			Expect(
				k8sClient.Create(ctx, pod2),
			).To(Succeed())
		})

		When("pods exist", func() {
			It("returns true when pods are deleted", func() {
				go func() {
					defer GinkgoRecover()
					time.Sleep(time.Millisecond * 200)
					Expect(k8sClient.Delete(context.Background(), pod1)).To(Succeed())
					Expect(k8sClient.Delete(context.Background(), pod2)).To(Succeed())
				}()

				terminated, err := podRepo.WatchForPodsTermination(ctx, authInfo, appGUID, spaceGUID)
				Expect(err).NotTo(HaveOccurred())
				Expect(terminated).To(BeTrue())
			})
		})

		When("no pods exist", func() {
			It("returns true", func() {
				terminated, err := podRepo.WatchForPodsTermination(ctx, authInfo, "i-dont-exist", spaceGUID)
				Expect(err).NotTo(HaveOccurred())
				Expect(terminated).To(BeTrue())
			})
		})

		When(" pods exist and context is cancelled", func() {
			It("returns false", func() {
				cctx, cancelfun := context.WithCancel(context.Background())
				go func() {
					time.Sleep(time.Millisecond * 200)
					cancelfun()
				}()
				terminated, err := podRepo.WatchForPodsTermination(cctx, authInfo, "i-dont-exist", spaceGUID)
				Expect(err).To(HaveOccurred())
				Expect(terminated).To(BeFalse())
			})
		})
	})
})

func createPodDef(name, namespace, appGUID, index string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{workloadsv1alpha1.CFAppGUIDLabelKey: appGUID},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "opi",
					Image: "some-image",
					Env: []corev1.EnvVar{
						{
							Name:  "CF_INSTANCE_INDEX",
							Value: index,
						},
					},
				},
			},
		},
	}
}
