package webhooks_test

import (
	"context"
	"errors"

	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/fake"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("CFAppValidatingWebhook", func() {
	var (
		ctx                context.Context
		nameRegistry       *fake.NameRegistry
		logger             logr.Logger
		duplicateValidator *webhooks.DuplicateValidator
		err                error
		testName           string
		testNewName        string
		testNamespace      string
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		Expect(workloadsv1alpha1.AddToScheme(scheme)).To(Succeed())

		nameRegistry = new(fake.NameRegistry)
		duplicateValidator = webhooks.NewDuplicateValidator(nameRegistry)
		logger = logf.Log

		testName = "test-app"
		testNewName = "test-app-new"
		testNamespace = "default"
	})

	Describe("ValidateCreate", func() {
		JustBeforeEach(func() {
			err = duplicateValidator.ValidateCreate(ctx, logger, testNamespace, testName)
		})

		It("succeeds", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("uses the name registry to register the name", func() {
			Expect(nameRegistry.RegisterNameCallCount()).To(Equal(1))
			actualContext, namespace, name := nameRegistry.RegisterNameArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(testNamespace))
			Expect(name).To(Equal(testName))
		})

		When("the name is already registered", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "jim"))
			})

			It("fails", func() {
				Expect(err).To(Equal(webhooks.ErrorDuplicateName))
			})
		})

		When("registering the name fails", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(errors.New("register-err"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError("register-err"))
			})
		})
	})

	Describe("Update", func() {
		JustBeforeEach(func() {
			err = duplicateValidator.ValidateUpdate(ctx, logger, testNamespace, testName, testNewName)
		})

		It("succeeds", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("locks the old name", func() {
			Expect(nameRegistry.TryLockNameCallCount()).To(Equal(1))
			_, namespace, name := nameRegistry.TryLockNameArgsForCall(0)
			Expect(namespace).To(Equal(testNamespace))
			Expect(name).To(Equal(testName))
		})

		It("registers the new name", func() {
			Expect(nameRegistry.RegisterNameCallCount()).To(Equal(1))
			_, namespace, name := nameRegistry.RegisterNameArgsForCall(0)
			Expect(namespace).To(Equal(testNamespace))
			Expect(name).To(Equal(testNewName))
		})

		It("deregisters the old name", func() {
			Expect(nameRegistry.DeregisterNameCallCount()).To(Equal(1))
			_, namespace, name := nameRegistry.DeregisterNameArgsForCall(0)
			Expect(namespace).To(Equal(testNamespace))
			Expect(name).To(Equal(testName))
		})

		When("the name isn't changed", func() {
			BeforeEach(func() {
				testNewName = testName
			})

			It("is allowed without using the name registry", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(nameRegistry.TryLockNameCallCount()).To(Equal(0))
				Expect(nameRegistry.RegisterNameCallCount()).To(Equal(0))
				Expect(nameRegistry.DeregisterNameCallCount()).To(Equal(0))
			})
		})

		When("taking the lock on the old name fails", func() {
			BeforeEach(func() {
				nameRegistry.TryLockNameReturns(errors.New("nope"))
			})

			It("fails", func() {
				Expect(err).To(HaveOccurred())
			})

			It("does not register the new name", func() {
				Expect(nameRegistry.RegisterNameCallCount()).To(Equal(0))
			})
		})

		When("the new name is already used", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "foo"))
			})

			It("fails", func() {
				Expect(err).To(Equal(webhooks.ErrorDuplicateName))
			})
		})

		When("registering the new name fails", func() {
			BeforeEach(func() {
				nameRegistry.RegisterNameReturns(errors.New("register-err"))
			})

			It("denies the request", func() {
				Expect(err).To(MatchError("register-err"))
			})

			It("releases the lock on the old name", func() {
				Expect(nameRegistry.UnlockNameCallCount()).To(Equal(1))
				_, namespace, name := nameRegistry.UnlockNameArgsForCall(0)
				Expect(namespace).To(Equal(testNamespace))
				Expect(name).To(Equal(testName))
			})

			When("releasing the old name lock fails", func() {
				BeforeEach(func() {
					nameRegistry.UnlockNameReturns(errors.New("unlock-err"))
				})

				It("fails with the register error", func() {
					Expect(err).To(MatchError("register-err"))
				})
			})
		})

		When("releasing the old name fails", func() {
			BeforeEach(func() {
				nameRegistry.DeregisterNameReturns(errors.New("deregister-err"))
			})

			It("continues anyway and allows the request", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("Delete", func() {
		JustBeforeEach(func() {
			err = duplicateValidator.ValidateDelete(ctx, logger, testNamespace, testName)
		})

		It("succeeds", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("deregisters the old name", func() {
			Expect(nameRegistry.DeregisterNameCallCount()).To(Equal(1))
			_, namespace, name := nameRegistry.DeregisterNameArgsForCall(0)
			Expect(namespace).To(Equal(testNamespace))
			Expect(name).To(Equal(testName))
		})

		When("the old name doesn't exist", func() {
			BeforeEach(func() {
				nameRegistry.DeregisterNameReturns(k8serrors.NewNotFound(schema.GroupResource{}, "foo"))
			})

			It("succeeds", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("deregistering fails", func() {
			BeforeEach(func() {
				nameRegistry.DeregisterNameReturns(errors.New("deregister-err"))
			})

			It("fails", func() {
				Expect(err).To(MatchError("deregister-err"))
			})
		})
	})
})
