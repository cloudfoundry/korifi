package integration_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Job TaskWorkload Controller Integration Test", func() {
	var (
		taskWorkload *korifiv1alpha1.TaskWorkload
		createErr    error
	)

	BeforeEach(func() {
		taskWorkload = &korifiv1alpha1.TaskWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prefixedGUID("taskworkload"),
				Namespace: testNamespace.Name,
			},
			Spec: korifiv1alpha1.TaskWorkloadSpec{
				Image:   "my-image",
				Command: []string{"echo", "hello"},
				Env:     []corev1.EnvVar{{Name: "MY_ENV_VAR", Value: "foo"}},
				Resources: corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:              *resource.NewScaledQuantity(1000, resource.Milli),
						corev1.ResourceMemory:           *resource.NewScaledQuantity(1024, resource.Mega),
						corev1.ResourceEphemeralStorage: *resource.NewScaledQuantity(1024, resource.Mega),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:              *resource.NewScaledQuantity(500, resource.Milli),
						corev1.ResourceMemory:           *resource.NewScaledQuantity(512, resource.Mega),
						corev1.ResourceEphemeralStorage: *resource.NewScaledQuantity(512, resource.Mega),
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		createErr = k8sClient.Create(context.Background(), taskWorkload)
	})

	It("creates a job owned by the task workload", func() {
		Expect(createErr).NotTo(HaveOccurred())

		jobList := &batchv1.JobList{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.List(context.Background(), jobList, client.InNamespace(testNamespace.Name)))
			g.Expect(jobList.Items).To(HaveLen(1))
		}).Should(Succeed())

		job := jobList.Items[0]
		Expect(job.Name).To(Equal(taskWorkload.Name))
		Expect(job.OwnerReferences).To(HaveLen(1))
		Expect(job.OwnerReferences[0].Name).To(Equal(taskWorkload.Name))
		Expect(*job.OwnerReferences[0].Controller).To(BeTrue())
		Expect(job.Spec.Parallelism).To(Equal(tools.PtrTo(int32(1))))
		Expect(job.Spec.Completions).To(Equal(tools.PtrTo(int32(1))))
		Expect(job.Spec.BackoffLimit).To(Equal(tools.PtrTo(int32(0))))

		podSpec := job.Spec.Template.Spec
		Expect(podSpec.RestartPolicy).To(Equal(corev1.RestartPolicyNever))
		Expect(podSpec.SecurityContext).To(Equal(&corev1.PodSecurityContext{
			RunAsNonRoot: tools.PtrTo(true),
		}))
		Expect(podSpec.AutomountServiceAccountToken).To(Equal(tools.PtrTo(false)))
		Expect(podSpec.Containers).To(HaveLen(1))
		Expect(podSpec.Containers[0].Name).To(Equal("workload"))
		Expect(podSpec.Containers[0].Image).To(Equal("my-image"))
		Expect(podSpec.Containers[0].Command).To(Equal([]string{"echo", "hello"}))
		Expect(podSpec.Containers[0].Env).To(Equal(taskWorkload.Spec.Env))
		Expect(podSpec.Containers[0].Resources).To(Equal(taskWorkload.Spec.Resources))
		Expect(podSpec.Containers[0].SecurityContext).To(Equal(&corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			AllowPrivilegeEscalation: tools.PtrTo(false),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		}))
	})

	It("sets the initialized condition on the task workload status", func() {
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(taskWorkload), taskWorkload)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(taskWorkload.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType)).To(BeTrue())
		}).Should(Succeed())
	})
})
