package networking_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/networking"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/networking/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("CF Route Validation", func() {
	var (
		ctx                context.Context
		duplicateValidator *fake.NameValidator
		fakeClient         *fake.Client
		realDecoder        *admission.Decoder
		cfRoute            *networkingv1alpha1.CFRoute
		cfDomain           *networkingv1alpha1.CFDomain
		cfApp              *workloadsv1alpha1.CFApp
		request            admission.Request
		validatingWebhook  *networking.CFRouteValidation
		response           admission.Response
		cfRouteJSON        []byte

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
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := networkingv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		realDecoder, err = admission.NewDecoder(scheme)
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

		cfRouteJSON, err = json.Marshal(cfRoute)
		Expect(err).NotTo(HaveOccurred())

		cfDomain = &networkingv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name: testDomainGUID,
			},
			Spec: networkingv1alpha1.CFDomainSpec{
				Name: testDomainName,
			},
		}

		cfApp = &workloadsv1alpha1.CFApp{}

		duplicateValidator = new(fake.NameValidator)
		fakeClient = new(fake.Client)

		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *networkingv1alpha1.CFDomain:
				cfDomain.DeepCopyInto(obj)
				return getDomainError
			case *workloadsv1alpha1.CFApp:
				cfApp.DeepCopyInto(obj)
				return getAppError
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		validatingWebhook = networking.NewCFRouteValidation(duplicateValidator, rootNamespace, fakeClient)

		Expect(validatingWebhook.InjectDecoder(realDecoder)).To(Succeed())
	})

	Describe("validating path", func() {
		var (
			invalidRoutePath   string
			invalidCFRoute     *networkingv1alpha1.CFRoute
			invalidCFRouteJson []byte
			err                error
		)

		JustBeforeEach(func() {
			invalidCFRoute = initializeRouteCR(testRouteProtocol, testRouteHost, invalidRoutePath, testRouteGUID, testRouteNamespace, testDomainGUID, testDomainNamespace)
			invalidCFRouteJson, err = json.Marshal(invalidCFRoute)
			Expect(err).NotTo(HaveOccurred())
		})

		When("creating route", func() {
			JustBeforeEach(func() {
				request = initCreateAdmissionRequestObj(testRouteGUID, testRouteNamespace, admissionv1.Create, invalidCFRouteJson)
				response = validatingWebhook.Handle(ctx, request)
			})

			When("with an invalid URI", func() {
				BeforeEach(func() {
					invalidRoutePath = "/%"
				})
				It("denies the request", func() {
					Expect(response.AdmissionResponse.Allowed).To(BeFalse())
					err := webhooks.ValidationError{
						Code:    webhooks.PathValidationError,
						Message: networking.InvalidURIError,
					}
					Expect(string(response.AdmissionResponse.Result.Reason)).To(Equal(err.Marshal()))
				})
			})

			When("the path is a single slash", func() {
				BeforeEach(func() {
					invalidRoutePath = "/"
				})
				It("denies the request", func() {
					Expect(response.AdmissionResponse.Allowed).To(BeFalse())
					err := webhooks.ValidationError{
						Code:    webhooks.PathValidationError,
						Message: networking.PathIsSlashError,
					}
					Expect(string(response.AdmissionResponse.Result.Reason)).To(Equal(err.Marshal()))
				})
			})

			When("the path lacks a leading slash", func() {
				BeforeEach(func() {
					invalidRoutePath = "foo/bar"
				})
				It("denies the request", func() {
					Expect(response.AdmissionResponse.Allowed).To(BeFalse())
					err := webhooks.ValidationError{
						Code:    webhooks.PathValidationError,
						Message: networking.InvalidURIError,
					}
					Expect(string(response.AdmissionResponse.Result.Reason)).To(Equal(err.Marshal()))
				})
			})

			When("the path contains a '?'", func() {
				BeforeEach(func() {
					invalidRoutePath = "/foo?/bar"
				})
				It("denies the request", func() {
					Expect(response.AdmissionResponse.Allowed).To(BeFalse())
					err := webhooks.ValidationError{
						Code:    webhooks.PathValidationError,
						Message: networking.PathHasQuestionMarkError,
					}
					Expect(string(response.AdmissionResponse.Result.Reason)).To(Equal(err.Marshal()))
				})
			})

			When("the path is greater than 128 characters", func() {
				BeforeEach(func() {
					invalidRoutePath = fmt.Sprintf("/%s", strings.Repeat("a", 128))
				})
				It("denies the request", func() {
					Expect(response.AdmissionResponse.Allowed).To(BeFalse())
					err := webhooks.ValidationError{
						Code:    webhooks.PathValidationError,
						Message: networking.PathLengthExceededError,
					}
					Expect(string(response.AdmissionResponse.Result.Reason)).To(Equal(err.Marshal()))
				})
			})

			When("with an invalid URI", func() {
				BeforeEach(func() {
					invalidRoutePath = "/%?"
				})
				It("denies the request", func() {
					Expect(response.AdmissionResponse.Allowed).To(BeFalse())
					err := webhooks.ValidationError{
						Code:    webhooks.PathValidationError,
						Message: networking.InvalidURIError + ", " + networking.PathHasQuestionMarkError,
					}
					Expect(string(response.AdmissionResponse.Result.Reason)).To(Equal(err.Marshal()))
				})
			})
		})

		When("updating route", func() {
			JustBeforeEach(func() {
				request = initUpdateAdmissionRequestObj(testRouteGUID, testRouteNamespace, admissionv1.Update, invalidCFRouteJson, cfRouteJSON)
				response = validatingWebhook.Handle(ctx, request)
			})

			When("with an invalid URI", func() {
				BeforeEach(func() {
					invalidRoutePath = "/%"
				})
				It("denies the request", func() {
					Expect(response.AdmissionResponse.Allowed).To(BeFalse())
					err := webhooks.ValidationError{
						Code:    webhooks.PathValidationError,
						Message: networking.InvalidURIError,
					}
					Expect(string(response.AdmissionResponse.Result.Reason)).To(Equal(err.Marshal()))
				})
			})

			When("the path is a single slash", func() {
				BeforeEach(func() {
					invalidRoutePath = "/"
				})
				It("denies the request", func() {
					Expect(response.AdmissionResponse.Allowed).To(BeFalse())
					err := webhooks.ValidationError{
						Code:    webhooks.PathValidationError,
						Message: networking.PathIsSlashError,
					}
					Expect(string(response.AdmissionResponse.Result.Reason)).To(Equal(err.Marshal()))
				})
			})

			When("the path lacks a leading slash", func() {
				BeforeEach(func() {
					invalidRoutePath = "foo/bar"
				})
				It("denies the request", func() {
					Expect(response.AdmissionResponse.Allowed).To(BeFalse())
					err := webhooks.ValidationError{
						Code:    webhooks.PathValidationError,
						Message: networking.InvalidURIError,
					}
					Expect(string(response.AdmissionResponse.Result.Reason)).To(Equal(err.Marshal()))
				})
			})

			When("the path contains a '?'", func() {
				BeforeEach(func() {
					invalidRoutePath = "/foo?/bar"
				})
				It("denies the request", func() {
					Expect(response.AdmissionResponse.Allowed).To(BeFalse())
					err := webhooks.ValidationError{
						Code:    webhooks.PathValidationError,
						Message: networking.PathHasQuestionMarkError,
					}
					Expect(string(response.AdmissionResponse.Result.Reason)).To(Equal(err.Marshal()))
				})
			})

			When("the path is greater than 128 characters", func() {
				BeforeEach(func() {
					invalidRoutePath = fmt.Sprintf("/%s", strings.Repeat("a", 128))
				})
				It("denies the request", func() {
					Expect(response.AdmissionResponse.Allowed).To(BeFalse())
					err := webhooks.ValidationError{
						Code:    webhooks.PathValidationError,
						Message: networking.PathLengthExceededError,
					}
					Expect(string(response.AdmissionResponse.Result.Reason)).To(Equal(err.Marshal()))
				})
			})
		})
	})

	Describe("Create", func() {
		JustBeforeEach(func() {
			response = validatingWebhook.Handle(ctx, request)
		})

		BeforeEach(func() {
			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testRouteGUID,
					Namespace: testRouteNamespace,
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: cfRouteJSON,
					},
				},
			}
		})

		It("allows the request", func() {
			Expect(response.Allowed).To(BeTrue())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(1))
			actualContext, _, namespace, name := duplicateValidator.ValidateCreateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(rootNamespace))
			Expect(name).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
		})

		When("the Host has upper-case and `-` characters", func() {
			BeforeEach(func() {
				var err error
				cfRoute.Spec.Host = "tHis-is-a-vAlid-host-nAme"
				cfRouteJSON, err = json.Marshal(cfRoute)
				Expect(err).NotTo(HaveOccurred())

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      testRouteGUID,
						Namespace: testRouteNamespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: cfRouteJSON,
						},
					},
				}
			})

			It("allows the request", func() {
				Expect(response.Allowed).To(BeTrue())
			})

			It("invokes the validator with lower-cased hostname", func() {
				Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(1))
				_, _, _, name := duplicateValidator.ValidateCreateArgsForCall(0)
				Expect(name).To(Equal(strings.ToLower(cfRoute.Spec.Host) + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
			})
		})

		When("the Host has `_` characters", func() {
			BeforeEach(func() {
				var err error
				cfRoute.Spec.Host = "tHis-is-an_InvAlid-host_nAme"
				cfRouteJSON, err = json.Marshal(cfRoute)
				Expect(err).NotTo(HaveOccurred())

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      testRouteGUID,
						Namespace: testRouteNamespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: cfRouteJSON,
						},
					},
				}
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				err := webhooks.ValidationError{
					Code:    webhooks.RouteFQDNInvalidError,
					Message: "Route FQDN does not comply with RFC 1035 standards",
				}
				Expect(string(response.Result.Reason)).To(Equal(err.Marshal()))
			})
		})

		When("the app name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				err := webhooks.ValidationError{
					Code:    webhooks.DuplicateRouteError,
					Message: "Route already exists with host 'my-host' and path '/my-path' for domain 'test.domain.name'.",
				}
				Expect(string(response.Result.Reason)).To(Equal(err.Marshal()))
			})
		})

		When("there is an issue decoding the request", func() {
			BeforeEach(func() {
				var err error
				cfRouteJSON, err = json.Marshal(cfRoute)
				Expect(err).NotTo(HaveOccurred())
				badCFAppJSON := []byte("}" + string(cfRouteJSON))

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      testRouteGUID,
						Namespace: testRouteNamespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: badCFAppJSON,
						},
					},
				}
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})

			It("does not attempt to validate a name", func() {
				Expect(duplicateValidator.ValidateCreateCallCount()).To(Equal(0))
			})
		})

		When("validating the app name fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateCreateReturns(errors.New("boom"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.UnknownError.Marshal()))
			})
		})

		When("the Host on the route is empty", func() {
			BeforeEach(func() {
				var err error
				cfRoute.Spec.Host = ""
				cfRouteJSON, err = json.Marshal(cfRoute)
				Expect(err).NotTo(HaveOccurred())

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      testRouteGUID,
						Namespace: testRouteNamespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: cfRouteJSON,
						},
					},
				}
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})

		When("the Host is invalid with invalid characters", func() {
			BeforeEach(func() {
				var err error
				cfRoute.Spec.Host = "this-is-inv@lid-host-n@me"
				cfRouteJSON, err = json.Marshal(cfRoute)
				Expect(err).NotTo(HaveOccurred())

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      testRouteGUID,
						Namespace: testRouteNamespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: cfRouteJSON,
						},
					},
				}
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})

		When("the FQDN is invalid with invalid length", func() {
			BeforeEach(func() {
				cfDomain.Spec.Name = "a-very-looooooooooooong-invalid-domain-name-that-should-fail-validation"
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})

		When("the route has destinations", func() {
			BeforeEach(func() {
				cfRoute.Spec.Destinations = []networkingv1alpha1.Destination{
					{
						AppRef: v1.LocalObjectReference{
							Name: "some-name",
						},
					},
				}
				var err error
				cfRouteJSON, err = json.Marshal(cfRoute)
				Expect(err).NotTo(HaveOccurred())

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      testRouteGUID,
						Namespace: testRouteNamespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: cfRouteJSON,
						},
					},
				}
			})

			It("allows the request", func() {
				Expect(response.Allowed).To(BeTrue())
			})

			When("the destination contains an app not found in the route's namespace", func() {
				BeforeEach(func() {
					getAppError = k8serrors.NewNotFound(schema.GroupResource{}, "foo")
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
					Expect(string(response.Result.Reason)).To(Equal(webhooks.RouteDestinationNotInSpace.Marshal()))
				})
			})

			When("getting the destination app fails for another reason", func() {
				BeforeEach(func() {
					getAppError = errors.New("foo")
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
					Expect(string(response.Result.Reason)).To(Equal("foo"))
				})
			})
		})
	})

	Describe("Update", func() {
		var (
			updatedCFRoute   *networkingv1alpha1.CFRoute
			newTestRoutePath string
		)

		BeforeEach(func() {
			newTestRoutePath = "/new-path"
			updatedCFRoute = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testRouteGUID,
					Namespace: testRouteNamespace,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     testRouteHost,
					Path:     newTestRoutePath,
					Protocol: networkingv1alpha1.Protocol(testRouteProtocol),
					DomainRef: v1.ObjectReference{
						Name:      testDomainGUID,
						Namespace: testDomainNamespace,
					},
				},
			}
		})

		JustBeforeEach(func() {
			routeJSON, err := json.Marshal(cfRoute)
			Expect(err).NotTo(HaveOccurred())

			updatedRouteJSON, err := json.Marshal(updatedCFRoute)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testRouteGUID,
					Namespace: testRouteNamespace,
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw: updatedRouteJSON,
					},
					OldObject: runtime.RawExtension{
						Raw: routeJSON,
					},
				},
			}

			response = validatingWebhook.Handle(ctx, request)
		})

		It("allows the request", func() {
			Expect(response.Allowed).To(BeTrue())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
			actualContext, _, namespace, oldName, newName := duplicateValidator.ValidateUpdateArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(rootNamespace))
			Expect(oldName).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
			Expect(newName).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + newTestRoutePath))
		})

		When("the new app name is a duplicate", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(webhooks.ErrorDuplicateName)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				err := webhooks.ValidationError{
					Code:    webhooks.DuplicateRouteError,
					Message: "Route already exists with host 'my-host' and path '/new-path' for domain 'test.domain.name'.",
				}
				Expect(string(response.Result.Reason)).To(Equal(err.Marshal()))
			})
		})

		When("the update validation fails for another reason", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateUpdateReturns(errors.New("boom!"))
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.UnknownError.Marshal()))
			})
		})

		When("the HostName has upper-case and `-` characters", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Host = "tHis-is-a-vAlid-host-nAme"
			})

			It("allows the request", func() {
				Expect(response.Allowed).To(BeTrue())
			})

			It("invokes the validator with lower-cased hostname", func() {
				Expect(duplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
				_, _, _, _, name := duplicateValidator.ValidateUpdateArgsForCall(0)
				Expect(name).To(Equal(strings.ToLower(updatedCFRoute.Spec.Host) + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + newTestRoutePath))
			})
		})

		When("the Host has `_` characters", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Host = "tHis-is-an_InvAlid-host_nAme"
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
				err := webhooks.ValidationError{
					Code:    webhooks.RouteFQDNInvalidError,
					Message: "Route FQDN does not comply with RFC 1035 standards",
				}
				Expect(string(response.Result.Reason)).To(Equal(err.Marshal()))
			})
		})

		When("the new hostname is empty on the route", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Host = ""
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})

		When("the route has destinations", func() {
			BeforeEach(func() {
				updatedCFRoute.Spec.Destinations = []networkingv1alpha1.Destination{
					{
						AppRef: v1.LocalObjectReference{
							Name: "some-name",
						},
					},
				}
			})

			It("allows the request", func() {
				Expect(response.Allowed).To(BeTrue())
			})

			When("the destination contains an app not found in the route's namespace", func() {
				BeforeEach(func() {
					getAppError = k8serrors.NewNotFound(schema.GroupResource{}, "foo")
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
					Expect(string(response.Result.Reason)).To(Equal(webhooks.RouteDestinationNotInSpace.Marshal()))
				})
			})

			When("getting the destination app fails for another reason", func() {
				BeforeEach(func() {
					getAppError = errors.New("foo")
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
					Expect(string(response.Result.Reason)).To(Equal("foo"))
				})
			})
		})
	})

	Describe("Delete", func() {
		JustBeforeEach(func() {
			routeJSON, err := json.Marshal(cfRoute)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      testRouteGUID,
					Namespace: testRouteNamespace,
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{
						Raw: routeJSON,
					},
				},
			}

			response = validatingWebhook.Handle(ctx, request)
		})

		It("allows the request", func() {
			Expect(response.Allowed).To(BeTrue())
		})

		It("invokes the validator correctly", func() {
			Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
			actualContext, _, namespace, name := duplicateValidator.ValidateDeleteArgsForCall(0)
			Expect(actualContext).To(Equal(ctx))
			Expect(namespace).To(Equal(rootNamespace))
			Expect(name).To(Equal(testRouteHost + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
		})

		When("the HostName has upper-case and `-` characters", func() {
			BeforeEach(func() {
				cfRoute.Spec.Host = "tHis-is-a-vAlid-host-nAme"
			})

			It("allows the request", func() {
				Expect(response.Allowed).To(BeTrue())
			})

			It("invokes the validator with lower-cased hostname", func() {
				Expect(duplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
				_, _, _, name := duplicateValidator.ValidateDeleteArgsForCall(0)
				Expect(name).To(Equal(strings.ToLower(cfRoute.Spec.Host) + "::" + testDomainNamespace + "::" + testDomainGUID + "::" + testRoutePath))
			})
		})

		When("delete validation fails", func() {
			BeforeEach(func() {
				duplicateValidator.ValidateDeleteReturns(errors.New("boom!"))
			})

			It("disallows the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal(webhooks.UnknownError.Marshal()))
			})
		})
	})
})

func initializeRouteCR(routeProtocol, routeHost, routePath, routeGUID, routeSpaceGUID, domainGUID, domainSpaceGUID string) *networkingv1alpha1.CFRoute {
	return &networkingv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeGUID,
			Namespace: routeSpaceGUID,
		},
		Spec: networkingv1alpha1.CFRouteSpec{
			Host:     routeHost,
			Path:     routePath,
			Protocol: networkingv1alpha1.Protocol(routeProtocol),
			DomainRef: v1.ObjectReference{
				Name:      domainGUID,
				Namespace: domainSpaceGUID,
			},
		},
	}
}

func initAdmissionRequestObj(objName, objNamespace string, operation admissionv1.Operation) admission.Request {
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Name:      objName,
			Namespace: objNamespace,
			Operation: operation,
		},
	}
}

func initCreateAdmissionRequestObj(objName, objNamespace string, operation admissionv1.Operation, objBytes []byte) admission.Request {
	obj := initAdmissionRequestObj(objName, objNamespace, operation)
	obj.AdmissionRequest.Object = runtime.RawExtension{
		Raw: objBytes,
	}
	return obj
}

func initUpdateAdmissionRequestObj(objName, objNamespace string, operation admissionv1.Operation, objBytes, oldObjBytes []byte) admission.Request {
	obj := initAdmissionRequestObj(objName, objNamespace, operation)
	obj.AdmissionRequest.Object = runtime.RawExtension{
		Raw: objBytes,
	}
	obj.AdmissionRequest.OldObject = runtime.RawExtension{
		Raw: oldObjBytes,
	}
	return obj
}
