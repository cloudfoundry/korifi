package networking_test

import (
	"context"
	"errors"
	"fmt"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking"
	networkingfakes "code.cloudfoundry.org/korifi/controllers/webhooks/networking/fake"

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

var _ = Describe("CF Route Validation", func() {
	var (
		ctx                context.Context
		duplicateValidator *networkingfakes.NameValidator
		fakeClient         *fake.Client
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

		duplicateValidator = new(networkingfakes.NameValidator)
		fakeClient = new(fake.Client)

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFDomain:
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
			retErr = validatingWebhook.ValidateCreate(ctx, cfRoute)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the duplicate validator correctly", func() {
			Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(1))
			actualContext, _, namespace, name := duplicateValidator.ValidateCreateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(rootNamespace))
			Expect(name).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
		})

		When("the host contains upper-case characters", func() {
			BeforeEach(func() {
				cfRoute.Spec.Host = "vAlidnAme"
			})

			It("allows the request", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})

			It("invokes the validator with lower-case host correctly", func() {
				Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(1))
				_, _, _, name := duplicateValidator.ValidateCreateArgsForCall(0)
				Expect(name).To(Equal(strings.ToLower(cfRoute.Spec.Host) + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
			})
		})

		When("the app name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.DuplicateRouteErrorType,
					Message: "Route already exists with host 'my-host' and path '/my-path' for domain 'test.domain.name'.",
				}.Marshal()))
			})
		})

		When("validating the app name fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(errors.New("boom"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: "Unknown error while checking Route Name Duplicate",
				}.Marshal()))
			})
		})

		When("host is empty on the route", func() {
			BeforeEach(func() {
				cfRoute.Spec.Host = ""
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RouteHostNameValidationErrorType,
					Message: "host cannot be empty",
				}.Marshal()))
			})
		})

		When("the host contains invalid characters", func() {
			BeforeEach(func() {
				cfRoute.Spec.Host = "this-is-inv@lid-host-n@me"
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RouteHostNameValidationErrorType,
					Message: `host must be either "*" or contain only alphanumeric characters, "_", or "-"`,
				}.Marshal()))
			})
		})

		When("the FQDN is too long", func() {
			BeforeEach(func() {
				cfDomain.Spec.Name = "a-very-looooooooooooong-invalid-domain-name-that-should-fail-validation"
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RouteSubdomainValidationErrorType,
					Message: "Subdomains must each be at most 63 characters",
				}.Marshal()))
			})
		})

		When("the FQDN does not match the domain regex", func() {
			BeforeEach(func() {
				cfDomain.Spec.Name = "foo..bar"
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RouteFQDNValidationErrorType,
					Message: "FQDN 'my-host.foo..bar' does not comply with RFC 1035 standards",
				}.Marshal()))
			})
		})

		When("the path is invalid", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = "/%"
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RoutePathValidationErrorType,
					Message: networking.InvalidURIError,
				}.Marshal()))
			})
		})

		When("the path is a single slash", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = "/"
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RoutePathValidationErrorType,
					Message: networking.PathIsSlashError,
				}.Marshal()))
			})
		})

		When("the path lacks a leading slash", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = "foo/bar"
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RoutePathValidationErrorType,
					Message: networking.InvalidURIError,
				}.Marshal()))
			})
		})

		When("the path contains a '?'", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = "/foo?/bar"
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RoutePathValidationErrorType,
					Message: networking.PathHasQuestionMarkError,
				}.Marshal()))
			})
		})

		When("the path is longer than 128 characters", func() {
			BeforeEach(func() {
				cfRoute.Spec.Path = fmt.Sprintf("/%s", strings.Repeat("a", 128))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RoutePathValidationErrorType,
					Message: networking.PathLengthExceededError,
				}.Marshal()))
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
					Expect(retErr).To(MatchError(webhooks.ValidationError{
						Type:    networking.RouteDestinationNotInSpaceErrorType,
						Message: networking.RouteDestinationNotInSpaceErrorMessage,
					}.Marshal()))
				})
			})

			When("getting the destination app fails for another reason", func() {
				BeforeEach(func() {
					getAppError = errors.New("foo")
				})

				It("denies the request", func() {
					Expect(retErr).To(MatchError(webhooks.ValidationError{
						Type:    webhooks.UnknownErrorType,
						Message: "Error while checking Route Destinations in Namespace",
					}.Marshal()))
				})
			})
		})
	})

	Describe("ValidateUpdate", func() {
		var (
			updatedCFRoute   *korifiv1alpha1.CFRoute
			newTestRoutePath string
		)

		BeforeEach(func() {
			newTestRoutePath = "/new-path"
			updatedCFRoute = cfRoute.DeepCopy()
			updatedCFRoute.Spec.Path = newTestRoutePath
		})

		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateUpdate(ctx, cfRoute, updatedCFRoute)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
			actualContext, _, namespace, oldName, newName := duplicateValidator.ValidateUpdateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(rootNamespace))
			Expect(oldName).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
			Expect(newName).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + newTestRoutePath))
		})

		When("the new hostname contains upper-case characters", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Host = "vAlidnAme"
			})

			It("allows the request", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})

			It("invokes the validator with lower-case host correctly", func() {
				Expect(duplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
				_, _, _, _, newName := duplicateValidator.ValidateUpdateArgsForCall(0)
				Expect(newName).To(Equal(strings.ToLower(updatedCFRoute.Spec.Host) + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + newTestRoutePath))
			})
		})

		When("the new app name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.DuplicateRouteErrorType,
					Message: "Route already exists with host 'my-host' and path '/new-path' for domain 'test.domain.name'.",
				}.Marshal()))
			})
		})

		When("the update validation fails for another reason", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(errors.New("boom!"))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: "Unknown error while checking Route Name Duplicate",
				}.Marshal()))
			})
		})

		When("the new hostname is empty on the route", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Host = ""
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RouteHostNameValidationErrorType,
					Message: "host cannot be empty",
				}.Marshal()))
			})
		})

		When("the path is invalid", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Path = "/%"
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RoutePathValidationErrorType,
					Message: networking.InvalidURIError,
				}.Marshal()))
			})
		})

		When("the path is a single slash", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Path = "/"
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RoutePathValidationErrorType,
					Message: networking.PathIsSlashError,
				}.Marshal()))
			})
		})

		When("the path lacks a leading slash", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Path = "foo/bar"
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RoutePathValidationErrorType,
					Message: networking.InvalidURIError,
				}.Marshal()))
			})
		})

		When("the path contains a '?'", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Path = "/foo?/bar"
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RoutePathValidationErrorType,
					Message: networking.PathHasQuestionMarkError,
				}.Marshal()))
			})
		})

		When("the path is longer than 128 characters", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Path = fmt.Sprintf("/%s", strings.Repeat("a", 128))
			})

			It("denies the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    networking.RoutePathValidationErrorType,
					Message: networking.PathLengthExceededError,
				}.Marshal()))
			})
		})

		When("the route has destinations", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Destinations = []korifiv1alpha1.Destination{
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
					Expect(retErr).To(MatchError(webhooks.ValidationError{
						Type:    networking.RouteDestinationNotInSpaceErrorType,
						Message: networking.RouteDestinationNotInSpaceErrorMessage,
					}.Marshal()))
				})
			})

			When("getting the destination app fails for another reason", func() {
				BeforeEach(func() {
					getAppError = errors.New("foo")
				})

				It("denies the request", func() {
					Expect(retErr).To(MatchError(webhooks.ValidationError{
						Type:    webhooks.UnknownErrorType,
						Message: "Error while checking Route Destinations in Namespace",
					}.Marshal()))
				})
			})
		})
	})

	Describe("ValidateDelete", func() {
		JustBeforeEach(func() {
			retErr = validatingWebhook.ValidateDelete(ctx, cfRoute)
		})

		It("allows the request", func() {
			Expect(retErr).NotTo(HaveOccurred())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			actualContext, _, namespace, name := duplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(rootNamespace))
			Expect(name).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
		})

		When("the host contains upper-case characters", func() {
			BeforeEach(func() {
				testRouteHost = "vAlidnAme"
				cfRoute.Spec.Host = testRouteHost
			})

			It("allows the request", func() {
				Expect(retErr).NotTo(HaveOccurred())
			})
			It("invokes the validator with lower-case host correctly", func() {
				Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
				_, _, _, name := duplicateValidator.ValidateDeleteArgsForCall(0)
				Expect(name).To(Equal(strings.ToLower(testRouteHost) + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
			})
		})

		When("delete validation fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateDeleteReturns(errors.New("boom!"))
			})

			It("disallows the request", func() {
				Expect(retErr).To(MatchError(webhooks.ValidationError{
					Type:    webhooks.UnknownErrorType,
					Message: "Unknown error while checking Route Name Duplicate",
				}.Marshal()))
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
