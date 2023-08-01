package networking_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	controllerfake "code.cloudfoundry.org/korifi/controllers/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFRouteValidator", func() {
	var (
		ctx                context.Context
		duplicateValidator *fake.NameValidator
		fakeClient         *controllerfake.Client
		cfRoute            *korifiv1alpha1.CFRoute
		cfDomain           *korifiv1alpha1.CFDomain
		cfApp              *korifiv1alpha1.CFApp
		validatingWebhook  *networking.CFRouteValidator

		testRouteGUID       string
		testRouteNamespace  string
		testRouteProtocol   string
		testRouteHost       string
		testRoutePath       string
		testDomainGUID      string
		testDomainName      string
		testDomainNamespace string
		rootNamespace       string

		getDomainError error
		getAppError    error
		retErr         error

		getDomainCallCount int
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := korifiv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		testRouteGUID = "my-guid"
		testRouteNamespace = "my-ns"
		testRouteProtocol = "http"
		testRouteHost = "my-host"
		testRoutePath = "/my-path"
		testDomainGUID = "domain-guid"
		testDomainName = "test.domain.name"
		testDomainNamespace = "domain-ns"
		rootNamespace = "root-ns"
		getDomainError = nil
		getAppError = nil
		getDomainCallCount = 0

		cfRoute = initializeRouteCR(testRouteProtocol, testRouteHost, testRoutePath, testRouteGUID, testRouteNamespace, testDomainGUID, testDomainNamespace)

		cfDomain = &korifiv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name: testDomainGUID,
			},
			Spec: korifiv1alpha1.CFDomainSpec{
				Name: testDomainName,
			},
		}

		cfApp = &korifiv1alpha1.CFApp{}

		duplicateValidator = new(fake.NameValidator)
		fakeClient = new(controllerfake.Client)

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFDomain:
				getDomainCallCount++
				cfDomain.DeepCopyInto(obj)
				return getDomainError
			case *korifiv1alpha1.CFApp:
				cfApp.DeepCopyInto(obj)
				return getAppError
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		validatingWebhook = networking.NewCFRouteValidator(duplicateValidator, rootNamespace, fakeClient)
	})

	Describe("ValidateCreate", func() {
		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateCreate(ctx, cfRoute)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the duplicate validator correctly", func() {
			Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, actualResource := duplicateValidator.ValidateCreateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(rootNamespace))
			Expect(actualResource).To(Equal(cfRoute))
			Expect(actualResource.UniqueValidationErrorMessage()).To(Equal("Route already exists with host 'my-host' and path '/my-path' for domain 'test.domain.name'."))
		})

		It("validates that the domain exists", func() {
			Expect(getDomainCallCount).To(Equal(1), "Expected get domain call count mismatch")
		})

		When("the host is '*'", func() {
			BeforeEach(func() {
				cfRoute.Spec.Host = "*"
			})

			It("allows the request", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("retrieving the domain record fails", func() {
			BeforeEach(func() {
				getDomainError = errors.New("nope")
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.UnknownErrorType,
					ContainSubstring("Error while retrieving CFDomain object"),
				))
			})
		})

		When("the host is invalid", func() {
			BeforeEach(func() {
				cfRoute.Spec.Host = "inVAlidnAme"
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					networking.RouteHostNameValidationErrorType,
					ContainSubstring("Host \"inVAlidnAme\" is not valid"),
				))
			})
		})

		When("the route name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(errors.New("foo"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError("foo"))
			})
		})

		When("the FQDN is too long", func() {
			BeforeEach(func() {
				cfRoute.Spec.Host = "a-very-looooooooooooong-invalid-host-name-that-should-fail-validation"
				for i := 0; i < 5; i++ {
					cfRoute.Spec.Host = cfRoute.Spec.Host + cfRoute.Spec.Host
				}
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					networking.RouteSubdomainValidationErrorType,
					ContainSubstring("subdomain must not exceed 253 characters"),
				))
			})
		})

		When("the path is invalid", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = "/%"
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					networking.RoutePathValidationErrorType,
					Equal(networking.InvalidURIError),
				))
			})
		})

		When("the path is a single slash", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = "/"
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					networking.RoutePathValidationErrorType,
					Equal(networking.PathIsSlashError),
				))
			})
		})

		When("the path lacks a leading slash", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = "foo/bar"
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					networking.RoutePathValidationErrorType,
					Equal(networking.InvalidURIError),
				))
			})
		})

		When("the path contains a '?'", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = "/foo?/bar"
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					networking.RoutePathValidationErrorType,
					Equal(networking.PathHasQuestionMarkError),
				))
			})
		})

		When("the path is longer than 128 characters", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = fmt.Sprintf("/%s", strings.Repeat("a", 128))
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					networking.RoutePathValidationErrorType,
					Equal(networking.PathLengthExceededError),
				))
			})
		})

		When("the route has destinations", func() {
			BeforeEach(func() {
				cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{
					{
						AppRef: v1.LocalObjectReference{
							Name: "some-name",
						},
					},
				}
			})

			It("allows the request", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})

			When("the destination contains an app not found in the route's namespace", func() {
				BeforeEach(func() {
					getAppError = k8serrors.NewNotFound(schema.GroupResource{}, "foo")
				})

				It("denies the request", func() {
					Expect(retErr).To(matchers.BeValidationError(
						networking.RouteDestinationNotInSpaceErrorType,
						Equal(networking.RouteDestinationNotInSpaceErrorMessage),
					))
				})
			})

			When("getting the destination app fails for another reason", func() {
				BeforeEach(func() {
					getAppError = errors.New("foo")
				})

				It("denies the request", func() {
					Expect(retErr).To(matchers.BeValidationError(
						webhooks.UnknownErrorType,
						Equal(webhooks.UnknownErrorMessage),
					))
				})
			})
		})
	})

	Describe("ValidateUpdate", func() {
		var updatedCFRoute *korifiv1alpha1.CFRoute

		BeforeEach(func() {
			updatedCFRoute = cfRoute.DeepCopy()
			updatedCFRoute.Spec.Destinations = []korifiv1alpha1.Destination{
				{
					AppRef: v1.LocalObjectReference{
						Name: "some-name",
					},
				},
			}
		})

		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateUpdate(ctx, cfRoute, updatedCFRoute)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, oldResource, newResource := duplicateValidator.ValidateUpdateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(rootNamespace))
			Expect(oldResource).To(Equal(cfRoute))
			Expect(newResource).To(Equal(updatedCFRoute))
		})

		It("does not validate that the domain exists", func() {
			Expect(getDomainCallCount).To(Equal(0), "Expected get domain call count mismatch")
		})

		When("the route is being deleted", func() {
			BeforeEach(func() {
				updatedCFRoute.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			})

			It("does not return an error", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
		})

		When("the hostname is updated", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Host = "valid-name"
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.ImmutableFieldErrorType,
					Equal("'CFRoute.Spec.Host' field is immutable"),
				))
			})
		})

		When("the new route name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(errors.New("foo"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError("foo"))
			})
		})

		When("the path is updated", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Path = "/%"
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.ImmutableFieldErrorType,
					Equal("'CFRoute.Spec.Path' field is immutable"),
				))
			})
		})

		When("the protocol is updated", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Protocol = "https"
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.ImmutableFieldErrorType,
					Equal("'CFRoute.Spec.Protocol' field is immutable"),
				))
			})
		})

		When("the DomainRef is updated", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.DomainRef = v1.ObjectReference{Name: "newDomainRef"}
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.ImmutableFieldErrorType,
					Equal("'CFRoute.Spec.DomainRef.Name' field is immutable"),
				))
			})
		})

		When("the destination contains an app not found in the route's namespace", func() {
			BeforeEach(func() {
				getAppError = k8serrors.NewNotFound(schema.GroupResource{}, "foo")
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					networking.RouteDestinationNotInSpaceErrorType,
					Equal(networking.RouteDestinationNotInSpaceErrorMessage),
				))
			})
		})

		When("getting the destination app fails for another reason", func() {
			BeforeEach(func() {
				getAppError = errors.New("foo")
			})

			It("denies the request", func() {
				Expect(retErr).To(matchers.BeValidationError(
					webhooks.UnknownErrorType,
					Equal(webhooks.UnknownErrorMessage),
				))
			})
		})
	})

	Describe("ValidateDelete", func() {
		JustBeforeEach(func() {
			_, retErr = validatingWebhook.ValidateDelete(ctx, cfRoute)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			actualContext, _, actualNamespace, actualResource := duplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(actualNamespace).To(Equal(rootNamespace))
			Expect(actualResource).To(Equal(cfRoute))
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

func initializeRouteCR(routeProtocol, routeHost, routePath, routeGUID, routeSpaceGUID, domainGUID, domainSpaceGUID string) *korifiv1alpha1.CFRoute {
	return &korifiv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeGUID,
			Namespace: routeSpaceGUID,
		},
		Spec: korifiv1alpha1.CFRouteSpec{
			Host:     routeHost,
			Path:     routePath,
			Protocol: korifiv1alpha1.Protocol(routeProtocol),
			DomainRef: v1.ObjectReference{
				Name:      domainGUID,
				Namespace: domainSpaceGUID,
			},
		},
	}
}
