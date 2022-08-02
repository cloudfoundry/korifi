package controllers_test

import (
	"context"
	"errors"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/job-task-runner/controllers"
	"code.cloudfoundry.org/korifi/job-task-runner/controllers/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
		fakeClient *fake.Client

		reconciler           *controllers.TaskWorkloadReconciler
		reconcileResult      ctrl.Result
		reconcileErr         error
		req                  ctrl.Request
		taskWorkload         *korifiv1alpha1.TaskWorkload
		getTaskWorkloadError error
		createJobError       error
	)

	BeforeEach(func() {
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

		getTaskWorkloadError = nil
		createJobError = nil

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

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      taskWorkload.Name,
				Namespace: taskWorkload.Namespace,
			},
		}

		fakeClient = new(fake.Client)
		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.TaskWorkload:
				taskWorkload.DeepCopyInto(obj)
				return getTaskWorkloadError
			case *batchv1.Job:
				return k8serrors.NewNotFound(schema.GroupResource{}, "job")
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		fakeClient.CreateStub = func(ctx context.Context, obj client.Object, option ...client.CreateOption) error {
			switch obj.(type) {
			case *batchv1.Job:
				return createJobError
			default:
				panic("TestClient Create provided an unexpected object type")
			}
		}

		reconciler = controllers.NewTaskWorkloadReconciler(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)), fakeClient, scheme.Scheme)
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = reconciler.Reconcile(context.Background(), req)
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

	When("the job already exists", func() {
		BeforeEach(func() {
			createJobError = k8serrors.NewAlreadyExists(schema.GroupResource{}, "foo")
		})

		It("does not return an error", func() {
			Expect(reconcileErr).NotTo(HaveOccurred())
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
		})
	})

	When("creating the job fails", func() {
		BeforeEach(func() {
			createJobError = errors.New("create-job-error")
		})

		It("returns the error", func() {
			Expect(reconcileErr).To(Equal(createJobError))
		})
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
})
