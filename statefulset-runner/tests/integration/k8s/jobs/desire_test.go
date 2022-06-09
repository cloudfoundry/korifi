package jobs_test

import (
	"context"
	"fmt"
	"os"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/jobs"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	eirinischeme "code.cloudfoundry.org/korifi/statefulset-runner/pkg/generated/clientset/versioned/scheme"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests/integration"
	"code.cloudfoundry.org/lager"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Task Desirer", func() {
	var (
		desirer   *jobs.Desirer
		task      *eiriniv1.Task
		taskGUID  string
		desireErr error
	)

	BeforeEach(func() {
		taskGUID = tests.GenerateGUID()
		taskName := tests.GenerateGUID()
		task = &eiriniv1.Task{
			ObjectMeta: metav1.ObjectMeta{
				Name: taskName,
			},
			Spec: eiriniv1.TaskSpec{
				GUID:    taskGUID,
				Name:    taskName,
				Image:   "eirini/busybox",
				Command: []string{"sh", "-c", "sleep 1"},
				Env: map[string]string{
					"FOO": "BAR",
				},
				AppName:   "app-name",
				AppGUID:   "app-guid",
				OrgName:   "org-name",
				OrgGUID:   "org-guid",
				SpaceName: "s",
				SpaceGUID: "s-guid",
				MemoryMB:  1024,
				DiskMB:    2048,
			},
		}
		var err error
		task, err = fixture.EiriniClientset.EiriniV1().Tasks(fixture.Namespace).Create(ctx, task, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		desirer = createTaskDesirer()
		desireErr = desirer.Desire(context.Background(), task)
	})

	It("succeeds", func() {
		Expect(desireErr).NotTo(HaveOccurred())
	})

	It("creates a corresponding job in the same namespace", func() {
		allJobs := integration.ListJobs(fixture.Clientset, fixture.Namespace, taskGUID)()
		job := allJobs[0]
		Expect(job.Name).To(Equal(fmt.Sprintf("app-name-s-%s", task.Spec.Name)))
		Expect(job.Labels).To(SatisfyAll(
			HaveKeyWithValue(jobs.LabelGUID, task.Spec.GUID),
			HaveKeyWithValue(jobs.LabelAppGUID, task.Spec.AppGUID),
			HaveKeyWithValue(jobs.LabelSourceType, "TASK"),
			HaveKeyWithValue(jobs.LabelName, task.Spec.Name),
		))
		Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))

		taskContainer := job.Spec.Template.Spec.Containers[0]
		Expect(taskContainer.Image).To(Equal("eirini/busybox"))
		Expect(taskContainer.Env).To(ContainElement(corev1.EnvVar{Name: "FOO", Value: "BAR"}))
		Expect(taskContainer.Command).To(Equal([]string{"sh", "-c", "sleep 1"}))

		Eventually(integration.GetTaskJobConditions(fixture.Clientset, fixture.Namespace, taskGUID)).Should(
			ConsistOf(MatchFields(IgnoreExtras, Fields{
				"Type":   Equal(batchv1.JobComplete),
				"Status": Equal(corev1.ConditionTrue),
			})),
		)
	})

	When("the task image lives in a private registry", func() {
		BeforeEach(func() {
			task.Spec.Image = "eiriniuser/notdora:latest"
			task.Spec.Command = []string{"/bin/echo", "hello"}
			task.Spec.PrivateRegistry = &eiriniv1.PrivateRegistry{
				Username: "eiriniuser",
				Password: tests.GetEiriniDockerHubPassword(),
			}
		})

		It("runs and completes the job", func() {
			allJobs := integration.ListJobs(fixture.Clientset, fixture.Namespace, taskGUID)()
			job := allJobs[0]
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			taskContainer := job.Spec.Template.Spec.Containers[0]
			Expect(taskContainer.Image).To(Equal("eiriniuser/notdora:latest"))

			Eventually(integration.GetTaskJobConditions(fixture.Clientset, fixture.Namespace, taskGUID)).Should(
				ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Type":   Equal(batchv1.JobComplete),
					"Status": Equal(corev1.ConditionTrue),
				})),
			)
		})

		It("creates a ImagePullSecret with the credentials", func() {
			registrySecretName := integration.GetRegistrySecretName(fixture.Clientset, fixture.Namespace, taskGUID, jobs.PrivateRegistrySecretGenerateName)

			getSecret := func() (*corev1.Secret, error) {
				return fixture.Clientset.
					CoreV1().
					Secrets(fixture.Namespace).
					Get(context.Background(), registrySecretName, metav1.GetOptions{})
			}

			secret, err := getSecret()

			Expect(err).NotTo(HaveOccurred())

			By("creating the secret", func() {
				Expect(secret.Name).To(ContainSubstring(jobs.PrivateRegistrySecretGenerateName))
				Expect(secret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))
				Expect(secret.Data).To(HaveKey(".dockerconfigjson"))
			})

			By("setting the owner reference on the secret", func() {
				allJobs := integration.ListJobs(fixture.Clientset, fixture.Namespace, taskGUID)()
				job := allJobs[0]

				var ownerRefs []metav1.OwnerReference
				Eventually(func() []metav1.OwnerReference {
					s, err := getSecret()
					if err != nil {
						return nil
					}

					ownerRefs = s.OwnerReferences

					return ownerRefs
				}).Should(HaveLen(1))

				Expect(ownerRefs[0].Name).To(Equal(job.Name))
				Expect(ownerRefs[0].UID).To(Equal(job.UID))
			})
		})
	})
})

func createTaskDesirer() *jobs.Desirer {
	logger := lager.NewLogger("task-desirer")
	logger.RegisterSink(lager.NewPrettySink(os.Stdout, lager.DEBUG))

	taskToJobConverter := jobs.NewTaskToJobConverter(
		tests.GetApplicationServiceAccount(),
		"registry-secret",
		false,
	)

	return jobs.NewDesirer(logger, taskToJobConverter, fixture.RuntimeClient, eirinischeme.Scheme)
}
