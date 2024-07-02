package brokers_test

import (
	"context"
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/services/brokers"
	"code.cloudfoundry.org/korifi/model/services"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFServiceBrokerValidatingWebhook", func() {
	const (
		defaultNamespace = "default"
	)

	var (
		ctx                context.Context
		duplicateValidator *fake.NameValidator
		serviceBroker      *korifiv1alpha1.CFServiceBroker
		validatingWebhook  *brokers.Validator
		retErr             error
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := korifiv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		serviceBroker = &korifiv1alpha1.CFServiceBroker{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: defaultNamespace,
			},
			Spec: korifiv1alpha1.CFServiceBrokerSpec{
				ServiceBroker: services.ServiceBroker{
					Name: uuid.NewString(),
				},
			},
		}

		duplicateValidator = new(fake.NameValidator)
		validatingWebhook = brokers.NewValidator(duplicateValidator)
	})

	Describe("ValidateCreate", func() {
		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateCreate(ctx, serviceBroker)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, actualResource := duplicateValidator.ValidateCreateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(serviceBroker.Namespace))
			Expect(actualResource).To(Equal(serviceBroker))
			Expect(actualResource.UniqueValidationErrorMessage()).To(Equal("Name must be unique"))
		})

		When("the broker name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(errors.New("foo"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError("foo"))
			})
		})
	})

	Describe("ValidateUpdate", func() {
		var updatedServiceBroker *korifiv1alpha1.CFServiceBroker

		BeforeEach(func() {
			updatedServiceBroker = serviceBroker.DeepCopy()
			updatedServiceBroker.Spec.Name = "the-new-name"
		})

		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateUpdate(ctx, serviceBroker, updatedServiceBroker)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, oldResource, newResource := duplicateValidator.ValidateUpdateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(serviceBroker.Namespace))
			Expect(oldResource).To(Equal(serviceBroker))
			Expect(newResource).To(Equal(updatedServiceBroker))
		})

		When("the service broker is being deleted", func() {
			BeforeEach(func() {
				updatedServiceBroker.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			})

			It("does not return an error", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("the new service broker name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(errors.New("foo"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError("foo"))
			})
		})
	})

	Describe("ValidateDelete", func() {
		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateDelete(ctx, serviceBroker)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, actualResource := duplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(serviceBroker.Namespace))
			Expect(actualResource).To(Equal(serviceBroker))
		})

		When("delete validation fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateDeleteReturns(errors.New("foo"))
			})

			It("disallows the request", func() {
				Expect(retErr).To(MatchError("foo"))
			})
		})
	})
})
