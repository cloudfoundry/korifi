package repositories_test

import (
	"context"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	eiriniLabelVersionKey = "workloads.cloudfoundry.org/version"
	cfProcessGuidKey      = "workloads.cloudfoundry.org/guid"
)

var _ = Describe("PodRepository", func() {
	const (
		appGUID             = "the-app-guid"
		pod1Name            = "some-pod-1"
		pod2Name            = "some-pod-2"
		podOtherVersionName = "other-version-2"
	)

	var (
		podRepo     *PodRepo
		ctx         context.Context
		spaceGUID   string
		processGUID string
		namespace   *corev1.Namespace
	)

	BeforeEach(func() {
		ctx = context.Background()
		spaceGUID = prefixedGUID("space")
		processGUID = prefixedGUID("process")
		podRepo = NewPodRepo(userClientFactory)
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}}

		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	Describe("ListPodStats", func() {
		var (
			message      ListPodStatsMessage
			records      []PodStatsRecord
			listStatsErr error

			pod1            *corev1.Pod
			pod2            *corev1.Pod
			podOtherVersion *corev1.Pod
		)

		BeforeEach(func() {
			pod1 = createPodDef(pod1Name, spaceGUID, appGUID, processGUID, "0", "1")
			pod2 = createPodDef(pod2Name, spaceGUID, appGUID, processGUID, "1", "1")
			podOtherVersion = createPodDef(podOtherVersionName, spaceGUID, appGUID, processGUID, "1", "2")
			Expect(k8sClient.Create(ctx, pod1)).To(Succeed())
			Expect(k8sClient.Create(ctx, pod2)).To(Succeed())
			Expect(k8sClient.Create(ctx, podOtherVersion)).To(Succeed())

			pod3 := createPodDef(prefixedGUID("pod3"), spaceGUID, uuid.NewString(), processGUID, "0", "1")
			Expect(k8sClient.Create(ctx, pod3)).To(Succeed())

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
			Expect(k8sClient.Status().Update(ctx, pod1)).To(Succeed())

			message = ListPodStatsMessage{
				Namespace:   spaceGUID,
				AppGUID:     "the-app-guid",
				ProcessGUID: processGUID,
				ProcessType: "web",
				AppRevision: "1",
			}
		})

		JustBeforeEach(func() {
			records, listStatsErr = podRepo.ListPodStats(ctx, authInfo, message)
		})

		When("authorized in the space", func() {
			var spaceDeveloperClusterRole *rbacv1.ClusterRole

			BeforeEach(func() {
				spaceDeveloperClusterRole = createClusterRole(ctx, SpaceDeveloperClusterRoleRules)
				createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, spaceGUID)
			})

			When("All required pods exists", func() {
				BeforeEach(func() {
					message.Instances = 2
				})

				It("Fetches all the pods and sets the appropriate state", func() {
					Expect(listStatsErr).NotTo(HaveOccurred())
					Expect(records).To(ConsistOf(
						[]PodStatsRecord{
							{Type: "web", Index: 0, State: "RUNNING"},
							{Type: "web", Index: 1, State: "DOWN"},
						},
					))
				})
			})

			When("Some pods are missing", func() {
				BeforeEach(func() {
					message.Instances = 3
				})

				It("Fetches pods and sets the appropriate state", func() {
					Expect(listStatsErr).NotTo(HaveOccurred())
					Expect(records).To(ConsistOf(
						[]PodStatsRecord{
							{Type: "web", Index: 0, State: "RUNNING"},
							{Type: "web", Index: 1, State: "DOWN"},
							{Type: "web", Index: 2, State: "DOWN"},
						},
					))
				})
			})

			When("A pod is in pending state", func() {
				BeforeEach(func() {
					message.Instances = 3

					pod3 := createPodDef(prefixedGUID("pod3"), spaceGUID, appGUID, processGUID, "2", "1")
					Expect(k8sClient.Create(ctx, pod3)).To(Succeed())

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
					Expect(k8sClient.Status().Update(ctx, pod3)).To(Succeed())
				})

				It("fetches pods and sets the appropriate state", func() {
					Expect(listStatsErr).NotTo(HaveOccurred())
					Expect(records).To(ConsistOf(
						[]PodStatsRecord{
							{Type: "web", Index: 0, State: "RUNNING"},
							{Type: "web", Index: 1, State: "DOWN"},
							{Type: "web", Index: 2, State: "STARTING"},
						},
					))
				})
			})

			When("A process has zero instances", func() {
				BeforeEach(func() {
					message.ProcessGUID = uuid.NewString()
					message.Instances = 0
				})

				It("fetches no pods", func() {
					Expect(listStatsErr).NotTo(HaveOccurred())
					Expect(records).To(BeEmpty())
				})
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(listStatsErr).To(BeAssignableToTypeOf(ForbiddenError{}))
				forbiddenErr := listStatsErr.(ForbiddenError)
				Expect(forbiddenErr.ResourceType()).To(Equal(ProcessStatsResourceType))
			})
		})
	})
})

func createPodDef(name, namespace, appGUID, processGUID, index, version string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				workloadsv1alpha1.CFAppGUIDLabelKey: appGUID,
				eiriniLabelVersionKey:               version,
				cfProcessGuidKey:                    processGUID,
			},
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
