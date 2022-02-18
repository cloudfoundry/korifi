package repositories_test

import (
	"context"
	"errors"
	"strconv"
	"time"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/fake"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
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
		podRepo         *PodRepo
		ctx             context.Context
		spaceGUID       string
		processGUID     string
		namespace       *corev1.Namespace
		metricFetcherFn *fake.MetricsFetcherFn
	)

	BeforeEach(func() {
		ctx = context.Background()
		metricFetcherFn = new(fake.MetricsFetcherFn)
		spaceGUID = prefixedGUID("space")
		processGUID = prefixedGUID("process")
		podRepo = NewPodRepo(userClientFactory, metricFetcherFn.Spy)
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
			cpu             resource.Quantity
			mem             resource.Quantity
			disk            resource.Quantity
			err             error
			metricstime     time.Time
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

			cpu, err = resource.ParseQuantity("423730n")
			Expect(err).NotTo(HaveOccurred())
			mem, err = resource.ParseQuantity("19177472")
			Expect(err).NotTo(HaveOccurred())
			disk, err = resource.ParseQuantity("69705728")
			Expect(err).NotTo(HaveOccurred())
			metricstime = time.Now()

			podMetrics := metricsv1beta1.PodMetrics{
				Timestamp: metav1.Time{
					Time: metricstime,
				},
				Window: metav1.Duration{},
				Containers: []metricsv1beta1.ContainerMetrics{
					{
						Name: "my-container",
						Usage: corev1.ResourceList{
							corev1.ResourceCPU:     cpu,
							corev1.ResourceMemory:  mem,
							corev1.ResourceStorage: disk,
						},
					},
				},
			}
			metricFetcherFn.Returns(&podMetrics, nil)
		})

		JustBeforeEach(func() {
			records, listStatsErr = podRepo.ListPodStats(ctx, authInfo, message)
		})

		When("authorized in the space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, spaceGUID)
			})

			When("All required pods exists", func() {
				BeforeEach(func() {
					message.Instances = 2
				})

				It("Fetches all the pods and sets the appropriate state", func() {
					Expect(listStatsErr).NotTo(HaveOccurred())

					Expect(records).To(MatchElementsWithIndex(matchElementsWithIndexIDFn, IgnoreExtras, Elements{
						"0": MatchFields(IgnoreExtras, Fields{
							"Type":  Equal("web"),
							"Index": Equal(0),
							"State": Equal("RUNNING"),
							"Usage": MatchFields(IgnoreExtras, Fields{
								"Time": PointTo(Equal(metricstime.UTC().Format(TimestampFormat))),
								"CPU":  PointTo(Equal(0.042373)),
								"Mem":  PointTo(Equal(mem.Value())),
								"Disk": PointTo(Equal(disk.Value())),
							}),
						}),
						"1": MatchFields(IgnoreExtras, Fields{
							"Type":  Equal("web"),
							"Index": Equal(1),
							"State": Equal("DOWN"),
							"Usage": Equal(Usage{}),
						}),
					}))
				})
			})

			When("Some pods are missing", func() {
				BeforeEach(func() {
					message.Instances = 3
				})

				It("Fetches pods and sets the appropriate state", func() {
					Expect(listStatsErr).NotTo(HaveOccurred())
					Expect(records).To(MatchElementsWithIndex(matchElementsWithIndexIDFn, IgnoreExtras, Elements{
						"0": MatchFields(IgnoreExtras, Fields{
							"Type":  Equal("web"),
							"Index": Equal(0),
							"State": Equal("RUNNING"),
							"Usage": MatchFields(IgnoreExtras, Fields{
								"Time": PointTo(Equal(metricstime.UTC().Format(TimestampFormat))),
								"CPU":  PointTo(Equal(0.042373)),
								"Mem":  PointTo(Equal(mem.Value())),
								"Disk": PointTo(Equal(disk.Value())),
							}),
						}),
						"1": MatchFields(IgnoreExtras, Fields{
							"Type":  Equal("web"),
							"Index": Equal(1),
							"State": Equal("DOWN"),
							"Usage": Equal(Usage{}),
						}),
						"2": MatchFields(IgnoreExtras, Fields{
							"Type":  Equal("web"),
							"Index": Equal(2),
							"State": Equal("DOWN"),
							"Usage": Equal(Usage{}),
						}),
					}))
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

					Expect(records).To(MatchElementsWithIndex(matchElementsWithIndexIDFn, IgnoreExtras, Elements{
						"0": MatchFields(IgnoreExtras, Fields{
							"Type":  Equal("web"),
							"Index": Equal(0),
							"State": Equal("RUNNING"),
							"Usage": MatchFields(IgnoreExtras, Fields{
								"Time": PointTo(Equal(metricstime.UTC().Format(TimestampFormat))),
								"CPU":  PointTo(Equal(0.042373)),
								"Mem":  PointTo(Equal(mem.Value())),
								"Disk": PointTo(Equal(disk.Value())),
							}),
						}),
						"1": MatchFields(IgnoreExtras, Fields{
							"Type":  Equal("web"),
							"Index": Equal(1),
							"State": Equal("DOWN"),
							"Usage": Equal(Usage{}),
						}),
						"2": MatchFields(IgnoreExtras, Fields{
							"Type":  Equal("web"),
							"Index": Equal(2),
							"State": Equal("STARTING"),
							"Usage": MatchFields(IgnoreExtras, Fields{
								"Time": PointTo(Equal(metricstime.UTC().Format(TimestampFormat))),
								"CPU":  PointTo(Equal(0.042373)),
								"Mem":  PointTo(Equal(mem.Value())),
								"Disk": PointTo(Equal(disk.Value())),
							}),
						}),
					}))
				})
			})

			When("MetricFetcherFunction return an metrics resource not found error", func() {
				BeforeEach(func() {
					message.Instances = 2
					metricFetcherFn.Returns(nil, errors.New("the server could not find the requested resource"))
				})
				It("fetches all the pods and sets the usage stats with empty values", func() {
					Expect(listStatsErr).NotTo(HaveOccurred())
					Expect(records).To(ConsistOf(
						[]PodStatsRecord{
							{Type: "web", Index: 0, State: "RUNNING"},
							{Type: "web", Index: 1, State: "DOWN"},
						},
					))
				})
			})

			When("MetricFetcherFunction return some other error", func() {
				BeforeEach(func() {
					message.Instances = 2
					metricFetcherFn.Returns(nil, errors.New("boom"))
				})
				It("returns the error", func() {
					Expect(listStatsErr.Error()).To(ContainSubstring("boom"))
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

func matchElementsWithIndexIDFn(index int, element interface{}) string {
	return strconv.Itoa(index)
}
