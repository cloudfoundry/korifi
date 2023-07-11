package workloads_test

import (
	"context"
	"strconv"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CFTaskMutatingWebhook", func() {
	var cfTask *korifiv1alpha1.CFTask

	BeforeEach(func() {
		cfTask = &korifiv1alpha1.CFTask{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testutils.GenerateGUID(),
				Namespace: rootNamespace,
			},
			Spec: korifiv1alpha1.CFTaskSpec{
				Command: "echo",
				AppRef: corev1.LocalObjectReference{
					Name: testutils.GenerateGUID(),
				},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(context.Background(), cfTask)).To(Succeed())
		Expect(k8s.Patch(context.Background(), adminClient, cfTask, func() {
			cfTask.Status = korifiv1alpha1.CFTaskStatus{
				Conditions: []metav1.Condition{},
			}
		})).To(Succeed())
	})

	It("defaults Status.SequenceID", func() {
		seqId := cfTask.Status.SequenceID
		Expect(seqId).NotTo(BeZero())
		yearMonthDay := time.Now().Format("20060102")
		Expect(strconv.FormatInt(seqId, 10)).To(HavePrefix(yearMonthDay))
	})

	It("defaults Status.MemoryMB", func() {
		Expect(cfTask.Status.MemoryMB).To(BeNumerically("==", 500))
	})

	It("defaults Status.DiskQuotaMB", func() {
		Expect(cfTask.Status.DiskQuotaMB).To(BeNumerically("==", 512))
	})

	Describe("subsequent updates", func() {
		var (
			updateTaskFunc func()
			currentSeqId   int64
		)

		JustBeforeEach(func() {
			currentSeqId = cfTask.Status.SequenceID
			Expect(k8s.Patch(context.Background(), adminClient, cfTask, updateTaskFunc)).To(Succeed())
		})

		When("the spec is updated", func() {
			BeforeEach(func() {
				updateTaskFunc = func() {
					cfTask.Spec.Canceled = true
				}
			})

			It("does not update the sequenceID", func() {
				Expect(cfTask.Status.SequenceID).To(Equal(currentSeqId))
			})
		})

		When("the status is updated", func() {
			BeforeEach(func() {
				updateTaskFunc = func() {
					meta.SetStatusCondition(&cfTask.Status.Conditions, metav1.Condition{
						Type:    korifiv1alpha1.TaskInitializedConditionType,
						Status:  metav1.ConditionTrue,
						Reason:  "taskInitialized",
						Message: "taskInitialized",
					})
				}
			})

			It("does not update the sequenceID", func() {
				Expect(cfTask.Status.SequenceID).To(Equal(currentSeqId))
			})
		})
	})
})
