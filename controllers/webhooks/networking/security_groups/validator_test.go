package security_groups_test

import (
	"context"
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking/security_groups"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFSecurityGroupValidatingWebhook", func() {
	const (
		defaultNamespace = "default"
	)

	var (
		ctx                context.Context
		duplicateValidator *fake.NameValidator
		securityGroup      *korifiv1alpha1.CFSecurityGroup
		validatingWebhook  *security_groups.Validator
		retErr             error
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := korifiv1alpha1.AddToScheme(scheme)
		Expect(err).ToNot(HaveOccurred())

		securityGroup = &korifiv1alpha1.CFSecurityGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: defaultNamespace,
			},
			Spec: korifiv1alpha1.CFSecurityGroupSpec{
				DisplayName: uuid.NewString(),
				Rules: []korifiv1alpha1.SecurityGroupRule{
					{
						Protocol:    "tcp",
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				},
			},
		}

		duplicateValidator = new(fake.NameValidator)
		validatingWebhook = security_groups.NewValidator(duplicateValidator, defaultNamespace)
	})

	Describe("ValidateCreate", func() {
		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateCreate(ctx, securityGroup)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, actualResource := duplicateValidator.ValidateCreateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(securityGroup.Namespace))
			Expect(actualResource).To(Equal(securityGroup))
			Expect(actualResource.UniqueValidationErrorMessage()).To(Equal("The security group name is taken: " + securityGroup.Spec.DisplayName))
		})

		When("the security group name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(errors.New("foo"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError("foo"))
			})
		})
	})

	Describe("ValidateUpdate", func() {
		var updatedSecurityGroup *korifiv1alpha1.CFSecurityGroup

		BeforeEach(func() {
			updatedSecurityGroup = securityGroup.DeepCopy()
			updatedSecurityGroup.Spec.DisplayName = "the-new-name"
		})

		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateUpdate(ctx, securityGroup, updatedSecurityGroup)
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, oldResource, newResource := duplicateValidator.ValidateUpdateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(securityGroup.Namespace))
			Expect(oldResource).To(Equal(securityGroup))
			Expect(newResource).To(Equal(updatedSecurityGroup))
		})

		When("the security group is being deleted", func() {
			BeforeEach(func() {
				updatedSecurityGroup.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			})

			It("does not return an error", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("the new security group name is a duplicate", func() {
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
			_, retErr = validatingWebhook.ValidateDelete(ctx, securityGroup)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, actualResource := duplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(securityGroup.Namespace))
			Expect(actualResource).To(Equal(securityGroup))
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
