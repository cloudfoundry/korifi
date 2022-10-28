package networking_test

import (
	"context"
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking"
	"code.cloudfoundry.org/korifi/tests/matchers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	rootNamespace = "cf"
)

var _ = Describe("CFDomainValidator", func() {
	var (
		ctx context.Context

		fakeClient        *fake.Client
		requestDomainCR   korifiv1alpha1.CFDomain
		requestDomainName string
		existingDomains   []korifiv1alpha1.CFDomain
		listDomainsErr    error
		retErr            error

		validatingWebhook *networking.CFDomainValidator
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := korifiv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		existingDomains = []korifiv1alpha1.CFDomain{}
		listDomainsErr = nil

		fakeClient = new(fake.Client)
		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			switch list := list.(type) {
			case *korifiv1alpha1.CFDomainList:
				existingDomainList := korifiv1alpha1.CFDomainList{
					Items: existingDomains,
				}
				existingDomainList.DeepCopyInto(list)
				return listDomainsErr
			default:
				panic("FakeClient List provided an unexpected object type")
			}
		}

		requestDomainName = "foo.example.com"

		validatingWebhook = networking.NewCFDomainValidator(fakeClient)
	})

	Describe("ValidateCreate", func() {
		BeforeEach(func() {
			requestDomainCR = createCFDomain(requestDomainName)
		})

		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateCreate(ctx, &requestDomainCR)
		})

		It("does not return an error when no domains exist", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		When("an existing not-quite matching subdomain exists", func() {
			BeforeEach(func() {
				existingDomains = []korifiv1alpha1.CFDomain{createCFDomain("ample.com")}
			})

			It("does not return an error", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("an existing matching top-level domain exists", func() {
			BeforeEach(func() {
				existingDomains = []korifiv1alpha1.CFDomain{createCFDomain("bar.example.com")}
			})

			It("does not return an error", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("the domain matches an existing top-level domain", func() {
			BeforeEach(func() {
				existingDomains = []korifiv1alpha1.CFDomain{createCFDomain("example.com")}
			})

			It("returns an error", func() {
				Expect(retErr).To(matchers.BeValidationError(
					networking.DuplicateDomainErrorType,
					Equal("Overlapping domain exists"),
				))
			})
		})

		When("the domain matches an existing subdomain", func() {
			BeforeEach(func() {
				existingDomains = []korifiv1alpha1.CFDomain{createCFDomain("bar.foo.example.com")}
			})

			It("returns an error", func() {
				Expect(retErr).To(matchers.BeValidationError(
					networking.DuplicateDomainErrorType,
					Equal("Overlapping domain exists"),
				))
			})
		})

		When("there is an issue listing shared CFDomains", func() {
			BeforeEach(func() {
				listDomainsErr = errors.New("boom")
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.UnknownErrorType,
					Equal(webhooks.UnknownErrorMessage),
				))
			})
		})

		When("the domain name is invalid", func() {
			BeforeEach(func() {
				requestDomainCR.Spec.Name = "#my.$domain"
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					networking.InvalidDomainErrorType,
					ContainSubstring("is not a valid domain"),
				))
			})
		})
	})

	Describe("ValidateUpdate", func() {
		var (
			updatedCFDomain *korifiv1alpha1.CFDomain
			oldCFDomain     korifiv1alpha1.CFDomain
		)
		BeforeEach(func() {
			oldCFDomain = createCFDomain(requestDomainName)
			updatedCFDomain = oldCFDomain.DeepCopy()
			updatedCFDomain.Spec.Name = "this-is-updated-domain-name"
		})

		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateUpdate(ctx, &oldCFDomain, updatedCFDomain)
		})

		It("returns an error", func() {
			Expect(retErr).To(matchers.BeValidationError(
				webhooks.ImmutableFieldErrorType,
				Equal("'CFDomain.Spec.Name' field is immutable"),
			))
		})

		When("the domain is being deleted", func() {
			BeforeEach(func() {
				updatedCFDomain.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			})

			It("does not return an error", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})
	})
})

func createCFDomain(name string) korifiv1alpha1.CFDomain {
	return korifiv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: rootNamespace,
		},
		Spec: korifiv1alpha1.CFDomainSpec{
			Name: name,
		},
	}
}
