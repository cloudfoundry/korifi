package k8sns_test

import (
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/k8sns"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("SpaceAppsFinalizer", func() {
	var (
		appsFinalizer *k8sns.SpaceAppsFinalizer
		cfSpace       *korifiv1alpha1.CFSpace
		namespace     string

		result      ctrl.Result
		finalizeErr error
	)

	BeforeEach(func() {
		namespace = uuid.NewString()
		createNamespace(namespace)

		cfSpace = &korifiv1alpha1.CFSpace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:         rootNamespace,
				Name:              namespace,
				DeletionTimestamp: tools.PtrTo(metav1.Now()),
			},
		}

		appsFinalizer = k8sns.NewSpaceAppsFinalizer(controllersClient, 1000)
	})

	JustBeforeEach(func() {
		result, finalizeErr = appsFinalizer.Finalize(ctx, cfSpace)
	})

	It("succeeds", func() {
		Expect(finalizeErr).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})

	When("there are apps in the space", func() {
		BeforeEach(func() {
			Expect(controllersClient.Create(ctx, &korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFAppSpec{
					DisplayName:  uuid.NewString(),
					DesiredState: "STOPPED",
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			})).To(Succeed())
		})

		It("deletes the apps in foreground and requeues", func() {
			Expect(finalizeErr).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{RequeueAfter: 500 * time.Millisecond}))

			appsList := &korifiv1alpha1.CFAppList{}
			Expect(controllersClient.List(ctx, appsList, client.InNamespace(namespace))).To(Succeed())
			Expect(appsList.Items).To(HaveLen(1))

			cfApp := appsList.Items[0]
			Expect(cfApp.DeletionTimestamp.IsZero()).To(BeFalse())
			Expect(cfApp.Finalizers).To(ConsistOf(metav1.FinalizerDeleteDependents))
		})

		When("finalization has taken more than app deletion timeout", func() {
			BeforeEach(func() {
				cfSpace.DeletionTimestamp = tools.PtrTo(metav1.NewTime(time.Now().Add(-2000 * time.Second)))
			})

			It("succeeds and gives up", func() {
				Expect(finalizeErr).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				appsList := &korifiv1alpha1.CFAppList{}
				Expect(controllersClient.List(ctx, appsList, client.InNamespace(namespace))).To(Succeed())
				Expect(appsList.Items).To(HaveLen(1))
				Expect(appsList.Items[0].DeletionTimestamp.IsZero()).To(BeTrue())
			})
		})
	})

	When("the namespace has been already marked for deletion", func() {
		BeforeEach(func() {
			Expect(controllersClient.Delete(ctx, getNamespace(cfSpace.Name))).To(Succeed())
		})

		It("succeeds", func() {
			Expect(finalizeErr).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	When("the namespace does not exist", func() {
		BeforeEach(func() {
			cfSpace.Name = "cf-space-without-namespace"
		})

		It("succeeds", func() {
			Expect(finalizeErr).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})
