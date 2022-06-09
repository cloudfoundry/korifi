package jobs_test

import (
	"context"
	"encoding/base64"
	"fmt"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/jobs"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/jobs/jobsfakes"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/k8sfakes"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	eirinischeme "code.cloudfoundry.org/korifi/statefulset-runner/pkg/generated/clientset/versioned/scheme"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Desire", func() {
	const (
		image    = "gcr.io/foo/bar"
		taskGUID = "task-123"
	)

	var (
		taskToJobConverter *jobsfakes.FakeTaskToJobConverter
		client             *k8sfakes.FakeClient

		job       *batchv1.Job
		task      *eiriniv1.Task
		desireErr error

		desirer *jobs.Desirer
	)

	BeforeEach(func() {
		job = &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "the-job-name",
				UID:  "the-job-uid",
			},
		}

		client = new(k8sfakes.FakeClient)
		taskToJobConverter = new(jobsfakes.FakeTaskToJobConverter)
		taskToJobConverter.ConvertReturns(job)

		task = &eiriniv1.Task{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "app-namespace",
			},
			Spec: eiriniv1.TaskSpec{
				Image:     image,
				Command:   []string{"/lifecycle/launch"},
				AppName:   "my-app",
				Name:      "task-name",
				AppGUID:   "my-app-guid",
				OrgName:   "my-org",
				SpaceName: "my-space",
				SpaceGUID: "space-id",
				OrgGUID:   "org-id",
				GUID:      taskGUID,
				MemoryMB:  1,
				CPUWeight: 2,
				DiskMB:    3,
			},
		}

		desirer = jobs.NewDesirer(
			tests.NewTestLogger("desiretask"),
			taskToJobConverter,
			client,
			eirinischeme.Scheme,
		)
	})

	JustBeforeEach(func() {
		desireErr = desirer.Desire(ctx, task)
	})

	It("succeeds", func() {
		Expect(desireErr).NotTo(HaveOccurred())
	})

	It("creates a job", func() {
		Expect(client.CreateCallCount()).To(Equal(1))
		_, actualJob, _ := client.CreateArgsForCall(0)
		Expect(actualJob).To(Equal(job))
	})

	When("creating the job fails", func() {
		BeforeEach(func() {
			client.CreateReturns(errors.New("create-failed"))
		})

		It("returns an error", func() {
			Expect(desireErr).To(MatchError(ContainSubstring("create-failed")))
		})
	})

	It("converts the task to job", func() {
		Expect(taskToJobConverter.ConvertCallCount()).To(Equal(1))
		Expect(taskToJobConverter.ConvertArgsForCall(0)).To(Equal(task))
	})

	It("sets the job namespace", func() {
		Expect(job.Namespace).To(Equal("app-namespace"))
	})

	When("the task uses a private registry", func() {
		var (
			createSecretError error
			createJobError    error
		)

		BeforeEach(func() {
			createSecretError = nil
			createJobError = nil
			task.Spec.PrivateRegistry = &eiriniv1.PrivateRegistry{
				Username: "username",
				Password: "password",
			}

			client.CreateStub = func(_ context.Context, object k8sclient.Object, _ ...k8sclient.CreateOption) error {
				secret, ok := object.(*corev1.Secret)
				if ok {
					if createSecretError != nil {
						return createSecretError
					}
					secret.Name = secret.GenerateName + "1234"
				}

				_, ok = object.(*batchv1.Job)
				if ok {
					return createJobError
				}

				return nil
			}
		})

		It("creates a secret with the registry credentials", func() {
			Expect(client.CreateCallCount()).To(Equal(2))
			_, actualObject, _ := client.CreateArgsForCall(0)
			actualSecret, ok := actualObject.(*corev1.Secret)
			Expect(ok).To(BeTrue())

			Expect(actualSecret.GenerateName).To(Equal("private-registry-"))
			Expect(actualSecret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))
			Expect(actualSecret.StringData).To(
				HaveKeyWithValue(
					".dockerconfigjson",
					fmt.Sprintf(
						`{"auths":{"gcr.io":{"username":"username","password":"password","auth":"%s"}}}`,
						base64.StdEncoding.EncodeToString([]byte("username:password")),
					),
				),
			)
		})

		It("converts the task using the private registry secret", func() {
			_, actualSecret := taskToJobConverter.ConvertArgsForCall(0)
			Expect(actualSecret.Name).To(Equal("private-registry-1234"))
		})

		It("sets the ownership of the secret to the job", func() {
			Expect(client.PatchCallCount()).To(Equal(1))
			_, obj, _, _ := client.PatchArgsForCall(0)
			Expect(obj).To(BeAssignableToTypeOf(&corev1.Secret{}))

			patchedSecret, ok := obj.(*corev1.Secret)
			Expect(ok).To(BeTrue())

			Expect(patchedSecret.OwnerReferences).To(HaveLen(1))
			Expect(patchedSecret.OwnerReferences[0].Kind).To(Equal("Job"))
			Expect(patchedSecret.OwnerReferences[0].Name).To(Equal(job.Name))
		})

		When("creating the secret fails", func() {
			BeforeEach(func() {
				createSecretError = errors.New("create-secret-err")
			})

			It("returns an error", func() {
				Expect(desireErr).To(MatchError(ContainSubstring("create-secret-err")))
			})
		})

		When("creating the job fails", func() {
			BeforeEach(func() {
				createJobError = errors.New("create-failed")
			})

			It("returns an error", func() {
				Expect(desireErr).To(MatchError(ContainSubstring("create-failed")))
			})

			It("deletes the secret", func() {
				Expect(client.DeleteCallCount()).To(Equal(1))
				_, obj, _ := client.DeleteArgsForCall(0)
				deletedSecret, ok := obj.(*corev1.Secret)
				Expect(ok).To(BeTrue())
				Expect(deletedSecret.Name).To(Equal("private-registry-1234"))
			})

			When("deleting the secret fails", func() {
				BeforeEach(func() {
					client.DeleteReturns(errors.New("delete-secret-failed"))
				})

				It("returns a job creation error and a note that the secret is not cleaned up", func() {
					Expect(desireErr).To(MatchError(And(ContainSubstring("create-failed"), ContainSubstring("delete-secret-failed"))))
				})
			})
		})

		When("setting the ownership of the secret fails", func() {
			BeforeEach(func() {
				client.PatchReturns(errors.New("potato"))
			})

			It("returns an error", func() {
				Expect(desireErr).To(MatchError(ContainSubstring("potato")))
			})
		})
	})
})
