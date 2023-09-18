package k8sns_test

import (
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/k8sns"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("NamespaceFinalizer", func() {
	var (
		nsFinalizer       *k8sns.NamespaceFinalizer[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg]
		delegateFinalizer *mockFinalizer[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg]
		cfOrg             *korifiv1alpha1.CFOrg
		namespace         string

		result      ctrl.Result
		finalizeErr error
	)

	BeforeEach(func() {
		namespace = uuid.NewString()
		createNamespace(namespace)

		cfOrg = &korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:  rootNamespace,
				Name:       namespace,
				Finalizers: []string{"example-finalizer"},
			},
		}

		delegateFinalizer = &mockFinalizer[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg]{}
		nsFinalizer = k8sns.NewNamespaceFinalizer[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg](controllersClient, delegateFinalizer, "example-finalizer")
	})

	JustBeforeEach(func() {
		result, finalizeErr = nsFinalizer.Finalize(ctx, cfOrg)
	})

	It("deletes the object namespace and requeues until the namespace is gone", func() {
		Expect(finalizeErr).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{RequeueAfter: time.Second}))

		Expect(delegateFinalizer.finalizedObjects).To(ConsistOf(cfOrg))

		// testenv does not actually delete namespaces, it only sets the deletion timestamp
		Expect(getNamespace(cfOrg.Name).DeletionTimestamp.IsZero()).To(BeFalse())
	})

	When("the namespace is finally gone", func() {
		BeforeEach(func() {
			cfOrg.Name = "an-org-without-namespace"
		})

		It("removes the finalizer", func() {
			Expect(finalizeErr).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			Expect(cfOrg.Finalizers).To(BeEmpty())
		})
	})

	When("the delegate finalizer requeues the request", func() {
		BeforeEach(func() {
			delegateFinalizer.result = ctrl.Result{RequeueAfter: 123 * time.Second}
		})

		It("does not remove the finalizer and does not delete the namespace", func() {
			Expect(result).To(Equal(ctrl.Result{RequeueAfter: 123 * time.Second}))
			Expect(finalizeErr).NotTo(HaveOccurred())

			Expect(cfOrg.Finalizers).To(ConsistOf("example-finalizer"))

			Expect(getNamespace(cfOrg.Name).DeletionTimestamp.IsZero()).To(BeTrue())
		})
	})

	When("the delegate finalizer fails", func() {
		BeforeEach(func() {
			delegateFinalizer.finalizeErr = errors.New("finalize-err")
		})

		It("does not delete the object, nor its namespace", func() {
			Expect(finalizeErr).To(MatchError("finalize-err"))

			Expect(cfOrg.Finalizers).To(ConsistOf("example-finalizer"))

			Expect(getNamespace(cfOrg.Name).DeletionTimestamp.IsZero()).To(BeTrue())
		})
	})

	When("the CFOrg object does not have the finalizer", func() {
		BeforeEach(func() {
			cfOrg.Finalizers = []string{}
		})

		It("it does not delete the namespace", func() {
			Expect(finalizeErr).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(getNamespace(cfOrg.Name).DeletionTimestamp.IsZero()).To(BeTrue())
		})
	})
})
