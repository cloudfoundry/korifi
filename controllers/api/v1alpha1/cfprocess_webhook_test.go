package v1alpha1_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	cfProcessGUIDLabelKey = "korifi.cloudfoundry.org/process-guid"
	cfProcessTypeLabelKey = "korifi.cloudfoundry.org/process-type"
)

var _ = Describe("CFProcessMutatingWebhook", func() {
	var (
		cfAppGUID     string
		cfProcessGUID string
		cfProcess     *korifiv1alpha1.CFProcess
	)

	BeforeEach(func() {
		cfAppGUID = GenerateGUID()
		cfProcessGUID = GenerateGUID()
		cfProcess = &korifiv1alpha1.CFProcess{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfProcessGUID,
				Namespace: namespace,
				Labels:    map[string]string{"foo": "bar"},
			},
			Spec: korifiv1alpha1.CFProcessSpec{
				AppRef: v1.LocalObjectReference{
					Name: cfAppGUID,
				},
				ProcessType: "test-process-type",
				Ports:       []int32{},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(context.Background(), cfProcess)).To(Succeed())
	})

	Describe("labels", func() {
		It("adds the labels with details about the process and the app", func() {
			Expect(cfProcess.Labels).To(HaveKeyWithValue(cfProcessGUIDLabelKey, cfProcessGUID))
			Expect(cfProcess.Labels).To(HaveKeyWithValue(cfProcessTypeLabelKey, "test-process-type"))
			Expect(cfProcess.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})

		It("preserves all other labels", func() {
			Expect(cfProcess.Labels).To(HaveKeyWithValue("foo", "bar"))
		})
	})

	Describe("memory, disk and timeout", func() {
		It("sets the configured default memory, disk and timeout", func() {
			Expect(cfProcess.Spec.MemoryMB).To(BeEquivalentTo(defaultMemoryMB))
			Expect(cfProcess.Spec.DiskQuotaMB).To(BeEquivalentTo(defaultDiskQuotaMB))
			Expect(cfProcess.Spec.HealthCheck.Data.TimeoutSeconds).To(BeEquivalentTo(defaultTimeout))
		})

		When("the process already has a memory value set", func() {
			BeforeEach(func() {
				cfProcess.Spec.MemoryMB = 42
			})

			It("preserves it", func() {
				Expect(cfProcess.Spec.MemoryMB).To(BeEquivalentTo(42))
				Expect(cfProcess.Spec.HealthCheck.Data.TimeoutSeconds).To(BeEquivalentTo(defaultTimeout))
				Expect(cfProcess.Spec.DiskQuotaMB).To(BeEquivalentTo(defaultDiskQuotaMB))
			})
		})

		When("the process already has a disk value set", func() {
			BeforeEach(func() {
				cfProcess.Spec.DiskQuotaMB = 42
			})

			It("preserves it", func() {
				Expect(cfProcess.Spec.MemoryMB).To(BeEquivalentTo(defaultMemoryMB))
				Expect(cfProcess.Spec.HealthCheck.Data.TimeoutSeconds).To(BeEquivalentTo(defaultTimeout))
				Expect(cfProcess.Spec.DiskQuotaMB).To(BeEquivalentTo(42))
			})
		})

		When("the process already has a timeout value set", func() {
			BeforeEach(func() {
				cfProcess.Spec.HealthCheck.Data.TimeoutSeconds = 16
			})

			It("preserves it", func() {
				Expect(cfProcess.Spec.MemoryMB).To(BeEquivalentTo(defaultMemoryMB))
				Expect(cfProcess.Spec.DiskQuotaMB).To(BeEquivalentTo(defaultDiskQuotaMB))
				Expect(cfProcess.Spec.HealthCheck.Data.TimeoutSeconds).To(BeEquivalentTo(16))
			})
		})
	})

	Describe("instances", func() {
		It("defaults desired instances to zero", func() {
			Expect(cfProcess.Spec.DesiredInstances).To(gstruct.PointTo(Equal(0)))
		})

		When("the process has the instance number set", func() {
			BeforeEach(func() {
				cfProcess.Spec.DesiredInstances = tools.PtrTo(24)
			})

			It("leaves instances unchanged", func() {
				Expect(cfProcess.Spec.DesiredInstances).To(gstruct.PointTo(Equal(24)))
			})
		})

		When("the process is of type web", func() {
			BeforeEach(func() {
				cfProcess.Spec.ProcessType = "web"
			})

			It("defaults instances to 1", func() {
				Expect(cfProcess.Spec.DesiredInstances).To(gstruct.PointTo(Equal(1)))
			})

			When("the process has the instance number set", func() {
				BeforeEach(func() {
					cfProcess.Spec.DesiredInstances = tools.PtrTo(42)
				})

				It("leaves instances unchanged", func() {
					Expect(cfProcess.Spec.DesiredInstances).To(gstruct.PointTo(Equal(42)))
				})
			})
		})
	})

	Describe("healthcheck", func() {
		It("defaults healthcheck type to process", func() {
			Expect(cfProcess.Spec.HealthCheck.Type).To(BeEquivalentTo("process"))
		})

		When("the type is already set", func() {
			BeforeEach(func() {
				cfProcess.Spec.HealthCheck.Type = "http"
			})

			It("preserves the value", func() {
				Expect(cfProcess.Spec.HealthCheck.Type).To(BeEquivalentTo("http"))
			})
		})

		When("the process is of type web", func() {
			BeforeEach(func() {
				cfProcess.Spec.ProcessType = "web"
			})

			It("defaults the type to port", func() {
				Expect(cfProcess.Spec.HealthCheck.Type).To(BeEquivalentTo("port"))
			})

			When("the type is already set", func() {
				BeforeEach(func() {
					cfProcess.Spec.HealthCheck.Type = "http"
				})

				It("preserves the value", func() {
					Expect(cfProcess.Spec.HealthCheck.Type).To(BeEquivalentTo("http"))
				})
			})
		})
	})
})
