package webhooks_test

import (
	"context"
	"errors"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("AppExistsValidator", func() {
	var (
		client       *fake.Client
		validator    *webhooks.AppExistsValidator
		appExistsErr *webhooks.ValidationError
	)

	BeforeEach(func() {
		client = new(fake.Client)
		validator = webhooks.NewAppExistsValidator(client)
	})

	JustBeforeEach(func() {
		appExistsErr = validator.EnsureCFApp(context.Background(), "the-namespace", "the-app")
	})

	It("ensures that the app exists", func() {
		Expect(appExistsErr).NotTo(HaveOccurred())

		Expect(client.GetCallCount()).To(Equal(1))
		_, namespacedName, object := client.GetArgsForCall(0)
		Expect(namespacedName).To(Equal(types.NamespacedName{
			Namespace: "the-namespace",
			Name:      "the-app",
		}))
		Expect(object).To(gstruct.PointTo(BeAssignableToTypeOf(korifiv1alpha1.CFApp{})))
	})

	When("the app does not exist", func() {
		BeforeEach(func() {
			client.GetReturns(k8serrors.NewNotFound(schema.GroupResource{}, "jim"))
		})

		It("returns an error", func() {
			Expect(*appExistsErr).To(MatchError(webhooks.ValidationError{
				Type:    webhooks.AppDoesNotExistErrorType,
				Message: "CFApp the-namespace:the-app does not exist",
			}))
		})
	})

	When("getting the app fails", func() {
		BeforeEach(func() {
			client.GetReturns(errors.New("whatever"))
		})

		It("returns an unknown error", func() {
			Expect(*appExistsErr).To(MatchError(webhooks.ValidationError{
				Type:    webhooks.UnknownErrorType,
				Message: webhooks.UnknownErrorMessage,
			}))
		})
	})
})
