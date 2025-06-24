package bindings_test

import (
	"context"
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/services/bindings"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFServiceBindingValidatingWebhook", func() {
	const (
		defaultNamespace = "default"
	)

	var (
		appGUID             string
		serviceInstanceGUID string
		ctx                 context.Context
		duplicateValidator  *fake.NameValidator
		serviceBinding      *korifiv1alpha1.CFServiceBinding
		validatingWebhook   *bindings.CFServiceBindingValidator
		retErr              error
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := korifiv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		appGUID = uuid.NewString()
		serviceInstanceGUID = uuid.NewString()
		serviceBinding = &korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: defaultNamespace,
			},
			Spec: korifiv1alpha1.CFServiceBindingSpec{
				AppRef: v1.LocalObjectReference{
					Name: appGUID,
				},
				Service: v1.ObjectReference{
					Name:      serviceInstanceGUID,
					Namespace: defaultNamespace,
				},
			},
		}

		duplicateValidator = new(fake.NameValidator)
		validatingWebhook = bindings.NewCFServiceBindingValidator(duplicateValidator)
	})

	Describe("ValidateCreate", func() {
		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateCreate(ctx, serviceBinding)
		})

		It("allows the creation of a service binding with unique app and service instance references", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("tries to create a lock for the service binding", func() {
			Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(1))
			_, _, actualNamespace, actualResource := duplicateValidator.ValidateCreateArgsForCall(0)
			Expect(actualNamespace).To(Equal(defaultNamespace))
			Expect(actualResource).To(Equal(serviceBinding))
			Expect(actualResource.UniqueValidationErrorMessage()).To(Equal("Service binding already exists: App: " + appGUID + " Service Instance: " + serviceInstanceGUID))
		})

		When("a duplicate service binding already exists", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(errors.New("foo"))
			})

			It("prevents the creation of the duplicate service binding", func() {
				Expect(retErr).To(MatchError("foo"))
			})
		})
	})

	Describe("ValidateUpdate", func() {
		var updatedServiceBinding *korifiv1alpha1.CFServiceBinding

		BeforeEach(func() {
			updatedServiceBinding = serviceBinding.DeepCopy()
			updatedServiceBinding.Spec.DisplayName = tools.PtrTo("display-name")
		})

		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateUpdate(ctx, serviceBinding, updatedServiceBinding)
		})

		It("allows the DisplayName to change", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		When("the service binding is being deleted", func() {
			BeforeEach(func() {
				updatedServiceBinding.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			})

			It("does not return an error", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("the AppRef name changes", func() {
			BeforeEach(func() {
				updatedServiceBinding.Spec.AppRef.Name = "updated-app-name"
			})

			It("does not allow the change", func() {
				Expect(retErr).To(MatchError(ContainSubstring("AppRef.Name is immutable")))
			})
		})

		When("the Service Instance name changes", func() {
			BeforeEach(func() {
				updatedServiceBinding.Spec.Service.Name = "updated-service-instance"
			})

			It("does not allow the change", func() {
				Expect(retErr).To(MatchError(ContainSubstring("Service.Name is immutable")))
			})
		})

		When("the Service Instance namespace changes", func() {
			BeforeEach(func() {
				updatedServiceBinding.Spec.Service.Namespace = "other-ns"
			})

			It("does not allow the change", func() {
				Expect(retErr).To(MatchError(ContainSubstring("Service.Namespace is immutable")))
			})
		})
	})

	Describe("ValidateDelete", func() {
		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateDelete(ctx, serviceBinding)
		})

		It("allows the deletion of a service binding", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("tries to delete the lock for the service binding", func() {
			Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			_, _, actualNamespace, actualResource := duplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(actualNamespace).To(Equal(defaultNamespace))
			Expect(actualResource).To(Equal(serviceBinding))
		})

		When("the lock resource cannot be deleted", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateDeleteReturns(errors.New("foo"))
			})

			It("prevents the deletion of the service binding", func() {
				Expect(retErr).To(MatchError("foo"))
			})
		})
	})
})
