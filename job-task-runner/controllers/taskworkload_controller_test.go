package controllers_test

import (
	"context"
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/job-task-runner/controllers"
	"code.cloudfoundry.org/korifi/job-task-runner/controllers/fake"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("TaskworkloadController", func() {
	var (
		statusGetter *fake.TaskStatusGetter

		reconciler           *k8s.PatchingReconciler[korifiv1alpha1.TaskWorkload, *korifiv1alpha1.TaskWorkload]
		reconcileResult      ctrl.Result
		reconcileErr         error
		req                  ctrl.Request
		taskWorkload         *korifiv1alpha1.TaskWorkload
		getTaskWorkloadError error
		createdJob           *batchv1.Job
		existingJob          *batchv1.Job
		getExistingJobError  error
		createJobError       error
	)

	BeforeEach(func() {
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

		taskWorkload = &korifiv1alpha1.TaskWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-task-workload",
				Namespace: "my-namespace",
			},
			Spec: korifiv1alpha1.TaskWorkloadSpec{
				Image:   "my-image",
				Command: []string{"my", "command"},
			},
		}
		getTaskWorkloadError = nil

		createdJob = &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-task-workload",
				Namespace: "my-namespace",
			},
			Spec:   batchv1.JobSpec{},
			Status: batchv1.JobStatus{},
		}
		existingJob = &batchv1.Job{}
		getExistingJobError = nil
		createJobError = nil

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.TaskWorkload:
				taskWorkload.DeepCopyInto(obj)
				return getTaskWorkloadError
			case *batchv1.Job:
				existingJob.DeepCopyInto(obj)
				return getExistingJobError
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		fakeClient.CreateStub = func(ctx context.Context, obj client.Object, option ...client.CreateOption) error {
			switch obj := obj.(type) {
			case *batchv1.Job:
				createdJob.DeepCopyInto(obj)
				return createJobError
			default:
				panic("TestClient Create provided an unexpected object type")
			}
		}

		statusGetter = new(fake.TaskStatusGetter)
		statusGetter.GetStatusConditionsReturns([]metav1.Condition{{
			Type:               "foo",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "something",
		}}, nil)

		reconciler = controllers.NewTaskWorkloadReconciler(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)), fakeClient, scheme.Scheme, statusGetter, time.Hour)

		req = ctrl.Request{NamespacedName: client.ObjectKeyFromObject(taskWorkload)}
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = reconciler.Reconcile(context.Background(), req)
	})

	When("getting the TaskWorkload fails", func() {
		BeforeEach(func() {
			getTaskWorkloadError = errors.New("get-task-workload-failed")
		})

		It("returns the error", func() {
			Expect(reconcileErr).To(Equal(getTaskWorkloadError))
		})
	})

	When("the TaskWorkload is not found", func() {
		BeforeEach(func() {
			getTaskWorkloadError = k8serrors.NewNotFound(schema.GroupResource{}, "foo")
		})

		It("does not return an error", func() {
			Expect(reconcileErr).NotTo(HaveOccurred())
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
		})
	})

	When("getting the job fails", func() {
		BeforeEach(func() {
			getExistingJobError = errors.New("get-existing-job-error")
		})

		It("returns the error", func() {
			Expect(reconcileErr).To(Equal(getExistingJobError))
		})
	})

	When("the job doesn't yet exist", func() {
		BeforeEach(func() {
			getExistingJobError = k8serrors.NewNotFound(schema.GroupResource{}, "job")
		})

		It("creates a job", func() {
			Expect(reconcileErr).NotTo(HaveOccurred())
			Expect(reconcileResult).To(Equal(ctrl.Result{}))

			Expect(fakeClient.CreateCallCount()).To(Equal(1))
			_, createdObject, _ := fakeClient.CreateArgsForCall(0)

			job, ok := createdObject.(*batchv1.Job)
			Expect(ok).To(BeTrue())
			Expect(job.Namespace).To(Equal(taskWorkload.Namespace))
			Expect(job.Name).To(Equal(taskWorkload.Name))
		})

		When("the job already exists while creating", func() {
			BeforeEach(func() {
				createJobError = k8serrors.NewAlreadyExists(schema.GroupResource{}, "foo")
			})

			It("returns an error", func() {
				Expect(reconcileErr).To(Equal(createJobError))
			})
		})

		When("creating the job fails for another reason", func() {
			BeforeEach(func() {
				createJobError = errors.New("create-job-error")
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(Equal(createJobError))
			})
		})
	})

	When("the job already exists", func() {
		BeforeEach(func() {
			existingJob = &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-task-workload",
					Namespace: "my-namespace",
				},
				Spec:   batchv1.JobSpec{},
				Status: batchv1.JobStatus{},
			}
		})
	})

	It("sets the task status conditions", func() {
		Expect(fakeStatusWriter.PatchCallCount()).To(Equal(1))
		_, object, _, _ := fakeStatusWriter.PatchArgsForCall(0)
		patchedTaskWorkload, ok := object.(*korifiv1alpha1.TaskWorkload)
		Expect(ok).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(patchedTaskWorkload.Status.Conditions, "foo")).To(BeTrue())
	})

	When("getting the status conditions fails", func() {
		BeforeEach(func() {
			statusGetter.GetStatusConditionsReturns(nil, errors.New("get-conditions-error"))
		})

		It("returns the error", func() {
			Expect(reconcileErr).To(MatchError(ContainSubstring("get-conditions-error")))
		})
	})
})
