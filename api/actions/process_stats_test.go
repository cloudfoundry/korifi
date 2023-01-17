package actions_test

import (
	"context"
	"errors"
	"time"

	. "code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/actions/fake"
	sfake "code.cloudfoundry.org/korifi/api/actions/shared/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

const (
	LabelVersionKey  = "korifi.cloudfoundry.org/version"
	cfProcessGuidKey = "korifi.cloudfoundry.org/guid"
)

var _ = Describe("ProcessStats", func() {
	var (
		processRepo *sfake.CFProcessRepository
		metricsRepo *fake.MetricsRepository
		appRepo     *sfake.CFAppRepository
		authInfo    authorization.Info

		processStats *ProcessStats

		responseRecords []PodStatsRecord
		responseErr     error

		podMetrics []repositories.PodMetrics
	)

	BeforeEach(func() {
		processRepo = new(sfake.CFProcessRepository)
		metricsRepo = new(fake.MetricsRepository)
		appRepo = new(sfake.CFAppRepository)
		authInfo = authorization.Info{Token: "a-token"}

		processRepo.GetProcessReturns(repositories.ProcessRecord{
			AppGUID:          "the-app-guid",
			DesiredInstances: 2,
			Type:             "web",
			MemoryMB:         1024,
			DiskQuotaMB:      2048,
		}, nil)

		appRepo.GetAppReturns(repositories.AppRecord{
			GUID:      "the-app-guid",
			SpaceGUID: "the-space-guid",
			State:     "STARTED",
			Revision:  "1",
		}, nil)

		podMetrics = []repositories.PodMetrics{
			{
				Pod:     createPod("0", "1"),
				Metrics: createPodMetrics("123m", "456", "890"),
			},
			{
				Pod:     createPod("1", "1"),
				Metrics: createPodMetrics("124m", "457", "891"),
			},
		}
		metricsRepo.GetMetricsReturns(podMetrics, nil)

		processStats = NewProcessStats(processRepo, appRepo, metricsRepo)
	})

	JustBeforeEach(func() {
		responseRecords, responseErr = processStats.FetchStats(context.Background(), authInfo, "the-process-guid")
	})

	It("fetches stats for the process pod", func() {
		Expect(responseErr).NotTo(HaveOccurred())

		Expect(processRepo.GetProcessCallCount()).To(Equal(1))
		_, actualAuthInfo, processGUID := processRepo.GetProcessArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authInfo))
		Expect(processGUID).To(Equal("the-process-guid"))

		Expect(processRepo.GetProcessCallCount()).To(Equal(1))
		_, actualAuthInfo, appGUID := appRepo.GetAppArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authInfo))
		Expect(appGUID).To(Equal("the-app-guid"))

		Expect(metricsRepo.GetMetricsCallCount()).To(Equal(1))
		_, actualAuthInfo, spaceGUID, labelMatcher := metricsRepo.GetMetricsArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authInfo))
		Expect(spaceGUID).To(Equal("the-space-guid"))
		Expect(labelMatcher).To(Equal(client.MatchingLabels{
			korifiv1alpha1.CFAppGUIDLabelKey: "the-app-guid",
			LabelVersion:                     "1",
			LabelGUID:                        "the-process-guid",
		}))

		Expect(responseRecords).To(HaveLen(2))

		Expect(responseRecords[0].Index).To(Equal(0))
		Expect(responseRecords[0].Type).To(Equal("web"))
		Expect(responseRecords[0].State).To(Equal("RUNNING"))
		Expect(responseRecords[0].Usage.Time).To(Equal(tools.PtrTo(time.Now().UTC().Format(repositories.TimestampFormat))))
		Expect(responseRecords[0].Usage.CPU).To(Equal(tools.PtrTo(0.123)))
		Expect(responseRecords[0].Usage.Mem).To(Equal(tools.PtrTo(int64(456))))
		Expect(responseRecords[0].Usage.Disk).To(Equal(tools.PtrTo(int64(890))))
		Expect(responseRecords[0].MemQuota).To(Equal(tools.PtrTo(int64(1024 * 1024 * 1024))))
		Expect(responseRecords[0].DiskQuota).To(Equal(tools.PtrTo(int64(2048 * 1024 * 1024))))

		Expect(responseRecords[1].Index).To(Equal(1))
		Expect(responseRecords[1].Type).To(Equal("web"))
		Expect(responseRecords[1].State).To(Equal("RUNNING"))
		Expect(responseRecords[1].Usage.Time).To(Equal(tools.PtrTo(time.Now().UTC().Format(repositories.TimestampFormat))))
		Expect(responseRecords[1].Usage.CPU).To(Equal(tools.PtrTo(0.124)))
		Expect(responseRecords[1].Usage.Mem).To(Equal(tools.PtrTo(int64(457))))
		Expect(responseRecords[1].Usage.Disk).To(Equal(tools.PtrTo(int64(891))))
		Expect(responseRecords[1].MemQuota).To(Equal(tools.PtrTo(int64(1024 * 1024 * 1024))))
		Expect(responseRecords[1].DiskQuota).To(Equal(tools.PtrTo(int64(2048 * 1024 * 1024))))
	})

	When("stats for some instances are missing", func() {
		BeforeEach(func() {
			metricsRepo.GetMetricsReturns([]repositories.PodMetrics{
				{
					Pod:     createPod("1", "1"),
					Metrics: createPodMetrics("124m", "457", "891"),
				},
			}, nil)
		})

		It("returns a 'down' stat for that instance", func() {
			Expect(responseErr).NotTo(HaveOccurred())
			Expect(responseRecords).To(HaveLen(2))
			Expect(responseRecords[0]).To(Equal(PodStatsRecord{
				Type:  "web",
				Index: 0,
				State: "DOWN",
			}))
		})
	})

	When("getting the app fails", func() {
		BeforeEach(func() {
			appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("get-app-err"))
		})

		It("returns the error", func() {
			Expect(responseErr).To(MatchError("get-app-err"))
		})
	})

	When("getting the process fails", func() {
		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{}, errors.New("get-process-err"))
		})

		It("returns the error", func() {
			Expect(responseErr).To(MatchError("get-process-err"))
		})
	})

	When("the app is stopped", func() {
		BeforeEach(func() {
			appRepo.GetAppReturns(repositories.AppRecord{
				State: repositories.StoppedState,
			}, nil)
		})

		It("returns a single 'down' stat", func() {
			Expect(responseRecords).To(ConsistOf(PodStatsRecord{
				Type:  "web",
				Index: 0,
				State: "DOWN",
			}))
		})
	})

	When("getting the stats fails", func() {
		BeforeEach(func() {
			metricsRepo.GetMetricsReturns(nil, errors.New("get-stats-err"))
		})

		It("returns the error", func() {
			Expect(responseErr).To(MatchError("get-stats-err"))
		})
	})

	When("there are no stats for the application container", func() {
		BeforeEach(func() {
			podMetrics[0].Pod.Spec.Containers[0].Name = "i-contain-no-app"
		})

		It("returns an error", func() {
			Expect(responseErr).To(MatchError(`container "application" not found`))
		})
	})

	When("the CF_INSTANCE_INDEX env var is not set", func() {
		BeforeEach(func() {
			podMetrics[0].Pod.Spec.Containers[0].Env = []corev1.EnvVar{}
		})

		It("returns an error", func() {
			Expect(responseErr).To(MatchError(`CF_INSTANCE_INDEX not set`))
		})
	})

	When("the CF_INSTANCE_INDEX env var value cannot be parsed to an int", func() {
		BeforeEach(func() {
			podMetrics[0].Pod.Spec.Containers[0].Env = []corev1.EnvVar{{
				Name:  "CF_INSTANCE_INDEX",
				Value: "one",
			}}
		})

		It("returns an error", func() {
			Expect(responseErr).To(MatchError(ContainSubstring(`parsing "one"`)))
		})
	})

	When("the CF_INSTANCE_INDEX env var value is a negative integer", func() {
		BeforeEach(func() {
			podMetrics[0].Pod.Spec.Containers[0].Env = []corev1.EnvVar{{
				Name:  "CF_INSTANCE_INDEX",
				Value: "-1",
			}}
		})

		It("returns an error", func() {
			Expect(responseErr).To(MatchError(ContainSubstring("indexes can't be negative")))
		})
	})

	Describe("pod status", func() {
		It("is ready", func() {
			Expect(responseRecords[0].State).To(Equal("RUNNING"))
		})

		When("the pod is not scheduled", func() {
			BeforeEach(func() {
				podMetrics[0].Pod.Status.Conditions = nil
				podMetrics[0].Pod.Status.ContainerStatuses = nil
			})

			It("is down", func() {
				Expect(responseRecords[0].State).To(Equal("DOWN"))
			})
		})

		When("the pod has a terminated container", func() {
			BeforeEach(func() {
				podMetrics[0].Pod.Status.Conditions = makeConditions("Initialized")
				podMetrics[0].Pod.Status.ContainerStatuses = []corev1.ContainerStatus{
					{
						Name: "application",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{},
						},
					},
				}
			})

			It("is crashed", func() {
				Expect(responseRecords[0].State).To(Equal("CRASHED"))
			})
		})

		When("scheduled but not running", func() {
			BeforeEach(func() {
				podMetrics[0].Pod.Status.Conditions = makeConditions("Initialized")
			})

			It("is starting", func() {
				Expect(responseRecords[0].State).To(Equal("STARTING"))
			})
		})
	})
})

func createPod(index, version string) corev1.Pod {
	return corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				LabelVersionKey: version,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "application",
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
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: makeConditions("Ready"),
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "cf-proc",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.Now(),
						},
					},
					Ready:   true,
					Started: tools.PtrTo(true),
				},
			},
		},
	}
}

func makeConditions(target string) []corev1.PodCondition {
	var conditions []corev1.PodCondition

	for _, condition := range []string{"PodScheduled", "Initialized", "ContainersReady", "Ready"} {
		conditions = append(conditions, corev1.PodCondition{
			Type:               corev1.PodConditionType(condition),
			Status:             "True",
			LastTransitionTime: metav1.Now(),
		})
		if condition == target {
			return conditions
		}
	}

	return nil
}

func createPodMetrics(cpuStr, memStr, diskStr string) metricsv1beta1.PodMetrics {
	cpu, err := resource.ParseQuantity(cpuStr)
	Expect(err).NotTo(HaveOccurred())
	mem, err := resource.ParseQuantity(memStr)
	Expect(err).NotTo(HaveOccurred())
	disk, err := resource.ParseQuantity(diskStr)
	Expect(err).NotTo(HaveOccurred())

	return metricsv1beta1.PodMetrics{
		Timestamp: metav1.Time{
			Time: time.Now(),
		},
		Window: metav1.Duration{},
		Containers: []metricsv1beta1.ContainerMetrics{
			{
				Name: "application",
				Usage: corev1.ResourceList{
					corev1.ResourceCPU:     cpu,
					corev1.ResourceMemory:  mem,
					corev1.ResourceStorage: disk,
				},
			},
		},
	}
}
