package authorization_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//counterfeiter:generate -o fake -fake-name WithWatch sigs.k8s.io/controller-runtime/pkg/client.WithWatch

var _ = Describe("RetryingK8sClient", func() {
	var (
		retryingClient client.WithWatch
		k8sClient      *fake.WithWatch
		ctx            context.Context
	)

	BeforeEach(func() {
		k8sClient = new(fake.WithWatch)
		backoff := wait.Backoff{
			Steps: 5,
		}
		retryingClient = authorization.NewAuthRetryingClient(k8sClient, backoff)
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

		When("it has a few forbidden errors", func() {
			BeforeEach(func() {
				k8sClient.GetReturns(k8serrors.NewForbidden(schema.GroupResource{}, "foo", errors.New("bar")))
				k8sClient.GetReturnsOnCall(3, nil)
			})

			It("calls the client multiple times before succeeding", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.GetCallCount()).To(Equal(4))
			})
		})
		When("it returns an error which isn't a forbidden error", func() {
			BeforeEach(func() {
				k8sClient.GetReturns(k8serrors.NewNotFound(schema.GroupResource{}, "foo"))
			})

			It("fails immediately", func() {
				Expect(k8serrors.IsNotFound(err)).To(BeTrue(), err)
				Expect(k8sClient.GetCallCount()).To(Equal(1))
			})
		})

		When("it always returns forbidden", func() {
			BeforeEach(func() {
				k8sClient.GetReturns(k8serrors.NewForbidden(schema.GroupResource{}, "foo", errors.New("bar")))
			})

			It("retries configured backoff steps times", func() {
				Expect(k8serrors.IsForbidden(err)).To(BeTrue(), err)
				Expect(k8sClient.GetCallCount()).To(Equal(5))
			})
		})

		When("it returns a webhook validation error", func() {
			var webhookValidationError k8serrors.StatusError
			BeforeEach(func() {
				validationErrBytes, marshalErr := json.Marshal(webhooks.ValidationError{
					Type:    "oh-no",
					Message: "anyway",
				})
				Expect(marshalErr).NotTo(HaveOccurred())

				webhookValidationError = k8serrors.StatusError{
					ErrStatus: metav1.Status{
						Reason: metav1.StatusReason(string(validationErrBytes)),
						Code:   http.StatusForbidden,
					},
				}
				k8sClient.GetReturns(&webhookValidationError)
			})

			It("fails immediately", func() {
				Expect(err).To(gstruct.PointTo(Equal(webhookValidationError)))
				Expect(k8sClient.GetCallCount()).To(Equal(1))
			})
		})
	})

	Describe("create with forbidden", func() {
		BeforeEach(func() {
			k8sClient.CreateReturns(k8serrors.NewForbidden(schema.GroupResource{}, "foo", errors.New("bar")))
		})

		It("retries", func() {
			err := retryingClient.Create(ctx, &corev1.Pod{})
			Expect(k8serrors.IsForbidden(err)).To(BeTrue(), err)
			Expect(k8sClient.CreateCallCount()).To(Equal(5))
		})
	})

	Describe("update with forbidden", func() {
		BeforeEach(func() {
			k8sClient.UpdateReturns(k8serrors.NewForbidden(schema.GroupResource{}, "foo", errors.New("bar")))
		})

		It("retries", func() {
			err := retryingClient.Update(ctx, &corev1.Pod{})
			Expect(k8serrors.IsForbidden(err)).To(BeTrue(), err)
			Expect(k8sClient.UpdateCallCount()).To(Equal(5))
		})
	})

	Describe("list with forbidden", func() {
		BeforeEach(func() {
			k8sClient.ListReturns(k8serrors.NewForbidden(schema.GroupResource{}, "foo", errors.New("bar")))
		})

		It("retries", func() {
			err := retryingClient.List(ctx, &corev1.PodList{})
			Expect(k8serrors.IsForbidden(err)).To(BeTrue(), err)
			Expect(k8sClient.ListCallCount()).To(Equal(5))
		})
	})

	Describe("patch with forbidden", func() {
		BeforeEach(func() {
			k8sClient.PatchReturns(k8serrors.NewForbidden(schema.GroupResource{}, "foo", errors.New("bar")))
		})

		It("retries", func() {
			err := retryingClient.Patch(ctx, &corev1.Pod{}, client.MergeFrom(&corev1.Pod{}))
			Expect(k8serrors.IsForbidden(err)).To(BeTrue(), err)
			Expect(k8sClient.PatchCallCount()).To(Equal(5))
		})
	})

	Describe("delete with forbidden", func() {
		BeforeEach(func() {
			k8sClient.DeleteReturns(k8serrors.NewForbidden(schema.GroupResource{}, "foo", errors.New("bar")))
		})

		It("retries", func() {
			err := retryingClient.Delete(ctx, &corev1.Pod{})
			Expect(k8serrors.IsForbidden(err)).To(BeTrue(), err)
			Expect(k8sClient.DeleteCallCount()).To(Equal(5))
		})
	})

	Describe("deleteAllOf with forbidden", func() {
		BeforeEach(func() {
			k8sClient.DeleteAllOfReturns(k8serrors.NewForbidden(schema.GroupResource{}, "foo", errors.New("bar")))
		})

		It("retries", func() {
			err := retryingClient.DeleteAllOf(ctx, &corev1.Pod{})
			Expect(k8serrors.IsForbidden(err)).To(BeTrue(), err)
			Expect(k8sClient.DeleteAllOfCallCount()).To(Equal(5))
		})
	})
})
