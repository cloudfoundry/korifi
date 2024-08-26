package repositories_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var _ = Describe("LogRepository", func() {
	var (
		appPod   *corev1.Pod
		buildPod *corev1.Pod
		message  repositories.GetLogsMessage
		cfOrg    *korifiv1alpha1.CFOrg
		cfSpace  *korifiv1alpha1.CFSpace

		logStreamer *fake.LogStreamer
		logRepo     *repositories.LogRepo
		logRecords  []repositories.LogRecord
		err         error
	)

	BeforeEach(func() {
		cfOrg = createOrgWithCleanup(ctx, uuid.NewString())
		cfSpace = createSpaceWithCleanup(ctx, cfOrg.Name, uuid.NewString())

		buildGUID := uuid.NewString()
		buildPod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Name,
				Name:      buildGUID,
				Labels: map[string]string{
					repositories.BuildWorkloadLabelKey: buildGUID,
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Image: "dont/care",
					Name:  "build-completion-container",
				}},
				InitContainers: []corev1.Container{{
					Image: "dont/care",
					Name:  "build-container",
				}},
			},
		}
		Expect(k8sClient.Create(ctx, buildPod)).To(Succeed())
		Expect(k8s.Patch(ctx, k8sClient, buildPod, func() {
			buildPod.Status = corev1.PodStatus{
				InitContainerStatuses: []corev1.ContainerStatus{{
					Name: "build-container",
				}},
			}
		})).To(Succeed())

		appGUID := uuid.NewString()
		appPod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Name,
				Name:      appGUID,
				Labels: map[string]string{
					korifiv1alpha1.CFAppGUIDLabelKey: appGUID,
					korifiv1alpha1.VersionLabelKey:   "7",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Image: "dont/care",
					Name:  "app-container",
				}},
			},
		}
		Expect(k8sClient.Create(ctx, appPod)).To(Succeed())
		Expect(k8s.Patch(ctx, k8sClient, appPod, func() {
			appPod.Status = corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "app-container",
				}},
			}
		})).To(Succeed())

		logStreamer = new(fake.LogStreamer)
		logStreamer.Stub = func(_ context.Context, _ kubernetes.Interface, pod corev1.Pod, _ corev1.PodLogOptions) (io.ReadCloser, error) {
			switch pod.Name {
			case buildPod.Name:
				return readerFor(map[time.Time]string{
					time.Unix(0, 100):  "b0",
					time.Unix(0, 1000): "b1",
					time.Unix(0, 2000): "b2",
				}), nil
			case appPod.Name:
				return readerFor(map[time.Time]string{
					time.Unix(0, 110):  "a0",
					time.Unix(0, 1100): "a1",
					time.Unix(0, 2100): "a2",
				}), nil
			}
			return nil, nil
		}

		logRepo = repositories.NewLogRepo(userClientFactory, logStreamer.Spy)

		message = repositories.GetLogsMessage{
			App: repositories.AppRecord{
				GUID:      appGUID,
				Revision:  "7",
				SpaceGUID: cfSpace.Name,
			},
			Build: repositories.BuildRecord{
				GUID:      buildGUID,
				SpaceGUID: cfSpace.Name,
			},
			StartTime: tools.PtrTo[int64](1000),
		}
	})

	JustBeforeEach(func() {
		logRecords, err = logRepo.GetAppLogs(ctx, authInfo, message)
	})

	It("returns a forbidden error", func() {
		Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
	})

	When("the user is allowed to get logs", func() {
		BeforeEach(func() {
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg.Name)
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfSpace.Name)
		})

		It("asks streams for build and app pod containers", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(logStreamer.CallCount()).To(Equal(2))

			_, _, actualPod, actualLogOptions := logStreamer.ArgsForCall(0)
			Expect(actualPod.Name).To(Equal(buildPod.Name))
			Expect(actualLogOptions).To(Equal(corev1.PodLogOptions{
				Container:  "build-container",
				SinceTime:  tools.PtrTo(metav1.NewTime(time.Unix(0, 1000))),
				Timestamps: true,
			}))

			_, _, actualPod, actualLogOptions = logStreamer.ArgsForCall(1)
			Expect(actualPod.Name).To(Equal(appPod.Name))
			Expect(actualLogOptions).To(Equal(corev1.PodLogOptions{
				Container:  "app-container",
				SinceTime:  tools.PtrTo(metav1.NewTime(time.Unix(0, 1000))),
				Timestamps: true,
			}))
		})

		When("a pod container is in waiting state", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, appPod, func() {
					appPod.Status = corev1.PodStatus{
						ContainerStatuses: []corev1.ContainerStatus{{
							Name: "app-container",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{},
							},
						}},
					}
				})).To(Succeed())
			})

			It("does not fetch its logs", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(logStreamer.CallCount()).To(Equal(1))

				_, _, actualPod, _ := logStreamer.ArgsForCall(0)
				Expect(actualPod.Name).To(Equal(buildPod.Name))
			})
		})

		It("merges build and app log entries later than the start time specified in ascending order", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(logRecords).To(HaveLen(4))
			Expect(logRecords[0]).To(matchLogRecord(1000, "b1", "STG"))
			Expect(logRecords[1]).To(matchLogRecord(1100, "a1", "APP"))
			Expect(logRecords[2]).To(matchLogRecord(2000, "b2", "STG"))
			Expect(logRecords[3]).To(matchLogRecord(2100, "a2", "APP"))
		})

		When("start time is not provided", func() {
			BeforeEach(func() {
				message.StartTime = nil
			})

			It("returns all logs", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(logRecords).To(HaveLen(6))
				Expect(logRecords[0]).To(matchLogRecord(100, "b0", "STG"))
				Expect(logRecords[1]).To(matchLogRecord(110, "a0", "APP"))
				Expect(logRecords[2]).To(matchLogRecord(1000, "b1", "STG"))
				Expect(logRecords[3]).To(matchLogRecord(1100, "a1", "APP"))
				Expect(logRecords[4]).To(matchLogRecord(2000, "b2", "STG"))
				Expect(logRecords[5]).To(matchLogRecord(2100, "a2", "APP"))
			})
		})

		When("limit is provided", func() {
			BeforeEach(func() {
				message.Limit = tools.PtrTo[int64](2)
			})

			It("uses the limit when getting app ad build log streams", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(logStreamer.CallCount()).To(Equal(2))

				_, _, actualPod, actualLogOptions := logStreamer.ArgsForCall(0)
				Expect(actualPod.Name).To(Equal(buildPod.Name))
				Expect(actualLogOptions.TailLines).To(PointTo(BeEquivalentTo(2)))

				_, _, actualPod, actualLogOptions = logStreamer.ArgsForCall(1)
				Expect(actualPod.Name).To(Equal(appPod.Name))
				Expect(actualLogOptions.TailLines).To(PointTo(BeEquivalentTo(2)))
			})

			It("limits the logs", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(logRecords).To(HaveLen(2))
				Expect(logRecords[0]).To(matchLogRecord(1000, "b1", "STG"))
				Expect(logRecords[1]).To(matchLogRecord(1100, "a1", "APP"))
			})

			When("the limit is greater than the number of log lines", func() {
				BeforeEach(func() {
					message.Limit = tools.PtrTo[int64](1000)
				})

				It("returns all logs since the desired timestamp", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(logRecords).To(HaveLen(4))
					Expect(logRecords[0]).To(matchLogRecord(1000, "b1", "STG"))
					Expect(logRecords[1]).To(matchLogRecord(1100, "a1", "APP"))
					Expect(logRecords[2]).To(matchLogRecord(2000, "b2", "STG"))
					Expect(logRecords[3]).To(matchLogRecord(2100, "a2", "APP"))
				})
			})
		})

		When("descending is requested", func() {
			BeforeEach(func() {
				message.Descending = true
			})

			It("sorts logs in descending order", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(logRecords).To(HaveLen(4))
				Expect(logRecords[0]).To(matchLogRecord(2100, "a2", "APP"))
				Expect(logRecords[1]).To(matchLogRecord(2000, "b2", "STG"))
				Expect(logRecords[2]).To(matchLogRecord(1100, "a1", "APP"))
				Expect(logRecords[3]).To(matchLogRecord(1000, "b1", "STG"))
			})
		})
	})
})

func readerFor(logs map[time.Time]string) io.ReadCloser {
	result := []string{}
	for k, v := range logs {
		result = append(result, fmt.Sprintf("%s %s", k.Format(time.RFC3339Nano), v))
	}

	return io.NopCloser(strings.NewReader(strings.Join(result, "\n")))
}

func matchLogRecord(timestamp int64, message string, tag string) types.GomegaMatcher {
	return Equal(repositories.LogRecord{
		Message:   message,
		Timestamp: timestamp,
		Tags: map[string]string{
			"source_type": tag,
		},
	})
}
