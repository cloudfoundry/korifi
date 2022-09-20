package integration_test

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
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CFProcessMutatingWebhook Integration Tests", func() {
	const (
		cfAppGUIDLabelKey     = "korifi.cloudfoundry.org/app-guid"
		cfProcessGUIDLabelKey = "korifi.cloudfoundry.org/process-guid"
		cfProcessType         = "test-process-type"
		cfProcessTypeLabelKey = "korifi.cloudfoundry.org/process-type"
		namespace             = "default"
	)

	var (
		cfAppGUID        string
		cfProcessGUID    string
		cfProcess        *korifiv1alpha1.CFProcess
		createdCFProcess *korifiv1alpha1.CFProcess
	)

	BeforeEach(func() {
		cfAppGUID = GenerateGUID()
		cfProcessGUID = GenerateGUID()
		cfProcess = &korifiv1alpha1.CFProcess{
			TypeMeta: metav1.TypeMeta{
				Kind:       "CFProcess",
				APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfProcessGUID,
				Namespace: namespace,
			},
			Spec: korifiv1alpha1.CFProcessSpec{
				AppRef: v1.LocalObjectReference{
					Name: cfAppGUID,
				},
				ProcessType: cfProcessType,
				Ports:       []int32{},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(context.Background(), cfProcess)).To(Succeed())
		createdCFProcess = new(korifiv1alpha1.CFProcess)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      cfProcessGUID,
				Namespace: namespace,
			}, createdCFProcess)).To(Succeed())
		}).Should(Succeed())
	})

	Describe("labels", func() {
		It("adds the appropriate labels", func() {
			Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfProcessGUIDLabelKey, cfProcessGUID))
			Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfProcessTypeLabelKey, cfProcessType))
			Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})

		When("there are other existing labels on the CFProcess record", func() {
			BeforeEach(func() {
				cfProcess.Labels = map[string]string{
					"anotherLabel": "process-label",
				}
			})

			It("preserves them", func() {
				Expect(createdCFProcess.ObjectMeta.Labels).To(HaveLen(4))
				Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue("anotherLabel", "process-label"))
			})
		})
	})

	Describe("memory and disk", func() {
		It("sets the configured default memory and disk", func() {
			Expect(createdCFProcess.Spec.MemoryMB).To(BeEquivalentTo(defaultMemoryMB))
			Expect(createdCFProcess.Spec.DiskQuotaMB).To(BeEquivalentTo(defaultDiskQuotaMB))
		})

		When("the process already has a memory value set", func() {
			BeforeEach(func() {
				cfProcess.Spec.MemoryMB = 42
			})

			It("preserves it", func() {
				Expect(createdCFProcess.Spec.MemoryMB).To(BeEquivalentTo(42))
				Expect(createdCFProcess.Spec.DiskQuotaMB).To(BeEquivalentTo(defaultDiskQuotaMB))
			})
		})

		When("the process already has a memory value set", func() {
			BeforeEach(func() {
				cfProcess.Spec.DiskQuotaMB = 42
			})

			It("preserves it", func() {
				Expect(createdCFProcess.Spec.MemoryMB).To(BeEquivalentTo(defaultMemoryMB))
				Expect(createdCFProcess.Spec.DiskQuotaMB).To(BeEquivalentTo(42))
			})
		})
	})

	Describe("instances", func() {
		It("defaults desired instances to zero", func() {
			Expect(createdCFProcess.Spec.DesiredInstances).To(gstruct.PointTo(Equal(0)))
		})

		When("the process has the instance number set", func() {
			BeforeEach(func() {
				cfProcess.Spec.DesiredInstances = tools.PtrTo(24)
			})

			It("leaves instances unchanged", func() {
				Expect(createdCFProcess.Spec.DesiredInstances).To(gstruct.PointTo(Equal(24)))
			})
		})

		When("the process is of type web", func() {
			BeforeEach(func() {
				cfProcess.Spec.ProcessType = "web"
			})

			It("defaults instances to 1", func() {
				Expect(createdCFProcess.Spec.DesiredInstances).To(gstruct.PointTo(Equal(1)))
			})

			When("the process has the instance number set", func() {
				BeforeEach(func() {
					cfProcess.Spec.DesiredInstances = tools.PtrTo(42)
				})

				It("leaves instances unchanged", func() {
					Expect(createdCFProcess.Spec.DesiredInstances).To(gstruct.PointTo(Equal(42)))
				})
			})
		})
	})

	Describe("healthcheck", func() {
		It("defaults healthcheck type to process", func() {
			Expect(createdCFProcess.Spec.HealthCheck.Type).To(BeEquivalentTo("process"))
		})

		When("the type is already set", func() {
			BeforeEach(func() {
				cfProcess.Spec.HealthCheck.Type = "http"
			})

			It("preserves the value", func() {
				Expect(createdCFProcess.Spec.HealthCheck.Type).To(BeEquivalentTo("http"))
			})
		})

		When("the process is of type web", func() {
			BeforeEach(func() {
				cfProcess.Spec.ProcessType = "web"
			})

			It("defaults the type to port", func() {
				Expect(createdCFProcess.Spec.HealthCheck.Type).To(BeEquivalentTo("port"))
			})

			When("the type is already set", func() {
				BeforeEach(func() {
					cfProcess.Spec.HealthCheck.Type = "http"
				})

				It("preserves the value", func() {
					Expect(createdCFProcess.Spec.HealthCheck.Type).To(BeEquivalentTo("http"))
				})
			})
		})
	})
})
