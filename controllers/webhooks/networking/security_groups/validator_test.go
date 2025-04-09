package security_groups_test

import (
	"context"
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking/security_groups"
	"code.cloudfoundry.org/korifi/tests/matchers"
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
			Expect(actualResource.UniqueValidationErrorMessage()).To(Equal("Security group with name '" + securityGroup.Spec.DisplayName + "' already exists."))
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

	Describe("Validating the security group rules when creating", func() {
		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateCreate(ctx, securityGroup)
		})

		When("the rule ports is invalid", func() {
			When("the protocol is invalid", func() {
				BeforeEach(func() {
					securityGroup.Spec.Rules[0].Protocol = "invalid"
				})

				It("returns an error", func() {
					Expect(retErr).To(matchers.BeValidationError(
						security_groups.InvalidSecurityGroupRuleErrorType,
						ContainSubstring("protocol must be 'tcp', 'udp', or 'all'"),
					))
				})
			})

			When("the protocol is ALL and has ports", func() {
				BeforeEach(func() {
					securityGroup.Spec.Rules[0].Protocol = "all"
				})

				It("returns an error", func() {
					Expect(retErr).To(matchers.BeValidationError(
						security_groups.InvalidSecurityGroupRuleErrorType,
						ContainSubstring("ports are not allowed for protocols of type all"),
					))
				})
			})

			When("the protocol is TCP and does not have ports", func() {
				BeforeEach(func() {
					securityGroup.Spec.Rules[0].Ports = ""
				})

				It("returns an error", func() {
					Expect(retErr).To(matchers.BeValidationError(
						security_groups.InvalidSecurityGroupRuleErrorType,
						ContainSubstring("ports are required for protocols of type TCP and UDP"),
					))
				})
			})

			When("the ports are invalid", func() {
				BeforeEach(func() {
					securityGroup.Spec.Rules[0].Ports = "invalid"
				})

				It("returns an error", func() {
					Expect(retErr).To(matchers.BeValidationError(
						security_groups.InvalidSecurityGroupRuleErrorType,
						ContainSubstring("ports must be a valid single port, comma separated list of ports, or range or ports, formatted as a string"),
					))
				})
			})

			When("the port is > 65535", func() {
				BeforeEach(func() {
					securityGroup.Spec.Rules[0].Ports = "67000"
				})

				It("returns an error", func() {
					Expect(retErr).To(matchers.BeValidationError(
						security_groups.InvalidSecurityGroupRuleErrorType,
						ContainSubstring("ports must be a valid single port, comma separated list of ports, or range or ports, formatted as a string"),
					))
				})
			})

			When("the port range is invalid", func() {
				BeforeEach(func() {
					securityGroup.Spec.Rules[0].Ports = "8080-invalid"
				})

				It("returns an error", func() {
					Expect(retErr).To(matchers.BeValidationError(
						security_groups.InvalidSecurityGroupRuleErrorType,
						ContainSubstring("ports must be a valid single port, comma separated list of ports, or range or ports, formatted as a string"),
					))
				})
			})
		})

		When("the rule destination is invalid", func() {
			When("the destination is not a valid IPV4", func() {
				BeforeEach(func() {
					securityGroup.Spec.Rules[0].Destination = "invalid"
				})

				It("returns an error", func() {
					Expect(retErr).To(matchers.BeValidationError(
						security_groups.InvalidSecurityGroupRuleErrorType,
						ContainSubstring("destination must contain valid CIDR(s), IP address(es), or IP address range(s)"),
					))
				})
			})

			When("the destination is not a valid CIDR", func() {
				BeforeEach(func() {
					securityGroup.Spec.Rules[0].Destination = "192.168.1.20/50"
				})

				It("returns an error", func() {
					Expect(retErr).To(matchers.BeValidationError(
						security_groups.InvalidSecurityGroupRuleErrorType,
						ContainSubstring("destination must contain valid CIDR(s), IP address(es), or IP address range(s)"),
					))
				})
			})

			When("the destination is not a valid ip range", func() {
				BeforeEach(func() {
					securityGroup.Spec.Rules[0].Destination = "192.168.1.20-invalid"
				})

				It("returns an error", func() {
					Expect(retErr).To(matchers.BeValidationError(
						security_groups.InvalidSecurityGroupRuleErrorType,
						ContainSubstring("destination IP address range is invalid"),
					))
				})
			})
		})
	})

	Describe("Validating the security group rules when updating", func() {
		var updatedSecurityGroup *korifiv1alpha1.CFSecurityGroup

		JustBeforeEach(func() {
			updatedSecurityGroup = securityGroup.DeepCopy()
			_, retErr = validatingWebhook.ValidateUpdate(ctx, securityGroup, updatedSecurityGroup)
		})

		When("the protocol is invalid", func() {
			BeforeEach(func() {
				securityGroup.Spec.Rules[0].Protocol = "invalid"
			})

			It("returns an error", func() {
				Expect(retErr).To(matchers.BeValidationError(
					security_groups.InvalidSecurityGroupRuleErrorType,
					ContainSubstring("protocol must be 'tcp', 'udp', or 'all'"),
				))
			})
		})

		When("the destination is not a valid IPV4", func() {
			BeforeEach(func() {
				securityGroup.Spec.Rules[0].Destination = "invalid"
			})

			It("returns an error", func() {
				Expect(retErr).To(matchers.BeValidationError(
					security_groups.InvalidSecurityGroupRuleErrorType,
					ContainSubstring("destination must contain valid CIDR(s), IP address(es), or IP address range(s)"),
				))
			})
		})
	})
})
