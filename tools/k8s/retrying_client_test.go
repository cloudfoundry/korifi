package k8s_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/tools/k8s"
	"code.cloudfoundry.org/korifi/tools/k8s/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o fake -fake-name WithWatch sigs.k8s.io/controller-runtime/pkg/client.WithWatch

var _ = Describe("RetryingK8sClient", func() {
	var (
		retriableError error
		retryingClient client.WithWatch
		k8sClient      *fake.WithWatch
		ctx            context.Context
		backoff        wait.Backoff
	)

	BeforeEach(func() {
		retriableError = errors.New("retriable-error")
		k8sClient = new(fake.WithWatch)
		backoff = wait.Backoff{
			Steps: 5,
		}

		retryingClient = k8s.NewRetryingClient(k8sClient, func(err error) bool { return err == retriableError }, backoff)
		ctx = context.Background()
	})

	Describe("get", func() {
		var err error

		JustBeforeEach(func() {
			err = retryingClient.Get(ctx, types.NamespacedName{}, &corev1.Pod{})
		})

		It("calls the client once", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.GetCallCount()).To(Equal(1))
		})

		When("it has a few retriable errors", func() {
			BeforeEach(func() {
				k8sClient.GetReturns(retriableError)
				k8sClient.GetReturnsOnCall(3, nil)
			})

			It("calls the client multiple times before succeeding", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.GetCallCount()).To(Equal(4))
			})
		})

		When("it always fails with retriable errror", func() {
			BeforeEach(func() {
				k8sClient.GetReturns(retriableError)
			})

			It("retries configured backoff steps times", func() {
				Expect(err).To(Equal(retriableError))
				Expect(k8sClient.GetCallCount()).To(Equal(backoff.Steps))
			})
		})

		When("it returns a non-retriable error", func() {
			BeforeEach(func() {
				k8sClient.GetReturns(errors.New("bar"))
			})

			It("fails immediately", func() {
				Expect(err).To(MatchError("bar"))
				Expect(k8sClient.GetCallCount()).To(Equal(1))
			})
		})
	})

	Describe("create with error", func() {
		BeforeEach(func() {
			k8sClient.CreateReturns(retriableError)
		})

		It("retries", func() {
			err := retryingClient.Create(ctx, &corev1.Pod{})
			Expect(err).To(Equal(retriableError))
			Expect(k8sClient.CreateCallCount()).To(Equal(backoff.Steps))
		})
	})

	Describe("update with error", func() {
		BeforeEach(func() {
			k8sClient.UpdateReturns(retriableError)
		})

		It("retries", func() {
			err := retryingClient.Update(ctx, &corev1.Pod{})
			Expect(err).To(Equal(retriableError))
			Expect(k8sClient.UpdateCallCount()).To(Equal(backoff.Steps))
		})
	})

	Describe("list with error", func() {
		BeforeEach(func() {
			k8sClient.ListReturns(retriableError)
		})

		It("retries", func() {
			err := retryingClient.List(ctx, &corev1.PodList{})
			Expect(err).To(Equal(retriableError))
			Expect(k8sClient.ListCallCount()).To(Equal(backoff.Steps))
		})
	})

	Describe("patch with error", func() {
		BeforeEach(func() {
			k8sClient.PatchReturns(retriableError)
		})

		It("retries", func() {
			err := retryingClient.Patch(ctx, &corev1.Pod{}, client.MergeFrom(&corev1.Pod{}))
			Expect(err).To(Equal(retriableError))
			Expect(k8sClient.PatchCallCount()).To(Equal(backoff.Steps))
		})
	})

	Describe("delete with error", func() {
		BeforeEach(func() {
			k8sClient.DeleteReturns(retriableError)
		})

		It("retries", func() {
			err := retryingClient.Delete(ctx, &corev1.Pod{})
			Expect(err).To(Equal(retriableError))
			Expect(k8sClient.DeleteCallCount()).To(Equal(backoff.Steps))
		})
	})

	Describe("deleteAllOf with error", func() {
		BeforeEach(func() {
			k8sClient.DeleteAllOfReturns(retriableError)
		})

		It("retries", func() {
			err := retryingClient.DeleteAllOf(ctx, &corev1.Pod{})
			Expect(err).To(Equal(retriableError))
			Expect(k8sClient.DeleteAllOfCallCount()).To(Equal(backoff.Steps))
		})
	})
})
