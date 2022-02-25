package workloads_test

import (
	"context"
	"encoding/json"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads/integration/helpers"
)

var _ = Describe("SubnamespaceanchorValidation", func() {
	var (
		ctx                     context.Context
		validatingWebhook       *workloads.SubnamespaceAnchorValidation
		namespace               string
		anchor                  *hnsv1alpha2.SubnamespaceAnchor
		orgDuplicateValidator   *fake.NameValidator
		spaceDuplicateValidator *fake.NameValidator
		request                 admission.Request
		response                admission.Response
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "my-namespace"
		orgDuplicateValidator = new(fake.NameValidator)
		spaceDuplicateValidator = new(fake.NameValidator)
		validatingWebhook = workloads.NewSubnamespaceAnchorValidation(orgDuplicateValidator, spaceDuplicateValidator)

		scheme := runtime.NewScheme()
		err := hnsv1alpha2.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		decoder, err := admission.NewDecoder(scheme)
		Expect(err).NotTo(HaveOccurred())
		Expect(validatingWebhook.InjectDecoder(decoder)).To(Succeed())

		anchor = &hnsv1alpha2.SubnamespaceAnchor{}
	})

	Describe("subnamespace anchor creation", func() {
		JustBeforeEach(func() {
			anchorJSON, err := json.Marshal(anchor)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      anchor.Name,
					Namespace: namespace,
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: anchorJSON,
					},
				},
			}
			response = validatingWebhook.Handle(ctx, request)
		})

		Context("orgs", func() {
			BeforeEach(func() {
				anchor = helpers.MakeOrg(namespace, "my-org")
			})

			It("validates the name", func() {
				Expect(spaceDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
				Expect(orgDuplicateValidator.ValidateCreateCallCount()).To(Equal(1))
				_, _, actualNamespace, name := orgDuplicateValidator.ValidateCreateArgsForCall(0)
				Expect(actualNamespace).To(Equal(namespace))
				Expect(name).To(Equal("my-org"))
			})

			When("the org name is unique in the namespace", func() {
				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("the org name already exists in the namespace", func() {
				BeforeEach(func() {
					orgDuplicateValidator.ValidateCreateReturns(workloads.ErrorDuplicateName)
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})
		})

		Context("spaces", func() {
			BeforeEach(func() {
				anchor = helpers.MakeSpace(namespace, "my-space")
			})

			It("validates the space name", func() {
				Expect(spaceDuplicateValidator.ValidateCreateCallCount()).To(Equal(1))
				Expect(orgDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
				_, _, actualNamespace, name := spaceDuplicateValidator.ValidateCreateArgsForCall(0)
				Expect(actualNamespace).To(Equal(namespace))
				Expect(name).To(Equal("my-space"))
			})

			When("the space name is unique in the namespace", func() {
				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("the space name already exists in the namespace", func() {
				BeforeEach(func() {
					spaceDuplicateValidator.ValidateCreateReturns(workloads.ErrorDuplicateName)
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})
		})

		Context("malformed orgs and spaces", func() {
			When("a subnamespace anchor has neither org nor space label", func() {
				BeforeEach(func() {
					anchor = helpers.MakeSubnamespaceAnchor(namespace, map[string]string{
						"something": "else",
					})
				})

				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})

				It("does not attempt to register any names", func() {
					Expect(spaceDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
					Expect(orgDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
				})
			})

			When("a subnamespace anchor has both org and space labels", func() {
				BeforeEach(func() {
					anchor = helpers.MakeSubnamespaceAnchor(namespace, map[string]string{
						workloads.OrgNameLabel:   "my-org",
						workloads.SpaceNameLabel: "my-space",
					})
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})

				It("does not attempt to register any names", func() {
					Expect(spaceDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
					Expect(orgDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
				})
			})
		})

		Context("failures", func() {
			When("decoding fails", func() {
				It("denies the request", func() {
					request.Object.Raw = []byte(`"[1,`)
					response = validatingWebhook.Handle(ctx, request)
					Expect(response.Allowed).To(BeFalse())
				})

				It("does not attempt to register any names", func() {
					Expect(spaceDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
					Expect(orgDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
				})
			})

			When("create validate throws another error", func() {
				BeforeEach(func() {
					anchor = helpers.MakeOrg(namespace, "my-org")
					orgDuplicateValidator.ValidateCreateReturns(errors.New("another error"))
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})
		})
	})

	Describe("subnamespace anchor updates", func() {
		var newAnchor *hnsv1alpha2.SubnamespaceAnchor

		JustBeforeEach(func() {
			anchorJSON, err := json.Marshal(anchor)
			Expect(err).NotTo(HaveOccurred())

			newAnchorJSON, err := json.Marshal(newAnchor)
			Expect(err).NotTo(HaveOccurred())

			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      anchor.Name,
					Namespace: namespace,
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw: newAnchorJSON,
					},
					OldObject: runtime.RawExtension{
						Raw: anchorJSON,
					},
				},
			}
			response = validatingWebhook.Handle(ctx, request)
		})

		Context("orgs", func() {
			BeforeEach(func() {
				anchor = helpers.MakeOrg(namespace, "my-org")
				newAnchor = helpers.MakeOrg(namespace, "another-org")
			})

			When("the org name hasn't changed", func() {
				BeforeEach(func() {
					newAnchor.Labels[workloads.OrgNameLabel] = "my-org"
					newAnchor.Labels["something"] = "else"
				})

				It("succeeds", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			It("validates the update", func() {
				Expect(orgDuplicateValidator.ValidateUpdateCallCount()).To(Equal(1))
				_, _, actualNamespace, oldName, newName := orgDuplicateValidator.ValidateUpdateArgsForCall(0)
				Expect(actualNamespace).To(Equal(namespace))
				Expect(oldName).To(Equal("my-org"))
				Expect(newName).To(Equal("another-org"))
			})

			When("the new org name is unique in the namespace", func() {
				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("the new org name already exists in the namespace", func() {
				BeforeEach(func() {
					orgDuplicateValidator.ValidateUpdateReturns(workloads.ErrorDuplicateName)
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})

			When("registering the new name fails", func() {
				BeforeEach(func() {
					orgDuplicateValidator.ValidateUpdateReturns(errors.New("boom!"))
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})
		})

		Context("spaces", func() {
			BeforeEach(func() {
				anchor = helpers.MakeSpace(namespace, "my-space")
				newAnchor = helpers.MakeSpace(namespace, "another-space")
			})

			When("the space name hasn't changed", func() {
				BeforeEach(func() {
					newAnchor.Labels[workloads.SpaceNameLabel] = "my-space"
					newAnchor.Labels["something"] = "else"
				})

				It("succeeds", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("the new space name is unique in the namespace", func() {
				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("the new space name already exists in the namespace", func() {
				BeforeEach(func() {
					spaceDuplicateValidator.ValidateUpdateReturns(workloads.ErrorDuplicateName)
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})

			When("validate fails for another reason", func() {
				BeforeEach(func() {
					spaceDuplicateValidator.ValidateUpdateReturns(errors.New("boom!"))
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})
		})

		Context("malformed orgs and spaces", func() {
			When("a subnamespace anchor has neither org nor space label", func() {
				BeforeEach(func() {
					newAnchor = helpers.MakeSubnamespaceAnchor(namespace, map[string]string{
						"something": "else",
					})
				})

				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})

				It("does not attempt any validation", func() {
					Expect(spaceDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
					Expect(orgDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
				})
			})

			When("a subnamespace anchor has both org and space labels", func() {
				BeforeEach(func() {
					newAnchor = helpers.MakeSubnamespaceAnchor(namespace, map[string]string{
						workloads.OrgNameLabel:   "my-org",
						workloads.SpaceNameLabel: "my-space",
					})
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})

				It("does not attempt any validation", func() {
					Expect(spaceDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
					Expect(orgDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
				})
			})
		})

		Context("failures", func() {
			When("decoding fails", func() {
				It("denies the request", func() {
					request.Object.Raw = []byte(`"[1,`)
					response = validatingWebhook.Handle(ctx, request)
					Expect(response.Allowed).To(BeFalse())
				})

				It("does not attempt to lock any names", func() {
					// ignore the calls from the JustBeforeEach()
					orgValidateUpdateCount := orgDuplicateValidator.ValidateUpdateCallCount()
					spaceValidateUpdateCount := spaceDuplicateValidator.ValidateUpdateCallCount()

					request.Object.Raw = []byte(`"[1,`)
					response = validatingWebhook.Handle(ctx, request)
					Expect(orgDuplicateValidator.ValidateUpdateCallCount()).To(Equal(orgValidateUpdateCount))
					Expect(spaceDuplicateValidator.ValidateUpdateCallCount()).To(Equal(spaceValidateUpdateCount))
				})
			})
		})
	})

	Describe("subnamespaceanchor deletion", func() {
		JustBeforeEach(func() {
			anchorJSON, err := json.Marshal(anchor)
			Expect(err).NotTo(HaveOccurred())
			request = admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Name:      anchor.Name,
					Namespace: namespace,
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{
						Raw: anchorJSON,
					},
				},
			}
			response = validatingWebhook.Handle(ctx, request)
		})

		Context("orgs", func() {
			BeforeEach(func() {
				anchor = helpers.MakeOrg(namespace, "my-org")
			})

			It("validates the delete", func() {
				Expect(orgDuplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
				Expect(spaceDuplicateValidator.ValidateDeleteCallCount()).To(Equal(0))
				_, _, actualNamespace, name := orgDuplicateValidator.ValidateDeleteArgsForCall(0)
				Expect(actualNamespace).To(Equal(namespace))
				Expect(name).To(Equal("my-org"))
			})

			It("allows the deletion", func() {
				Expect(response.Allowed).To(BeTrue())
			})

			When("validation fails", func() {
				BeforeEach(func() {
					orgDuplicateValidator.ValidateDeleteReturns(errors.New("boom!"))
				})

				It("denies the deletion", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})

			When("the anchor has no org or space label", func() {
				BeforeEach(func() {
					delete(anchor.Labels, workloads.OrgNameLabel)
				})

				It("allows the deletion anyway", func() {
					Expect(response.Allowed).To(BeTrue())
				})

				It("does not attempt any duplicate validation", func() {
					Expect(spaceDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
					Expect(orgDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
				})
			})

			When("the org has a space label as well", func() {
				BeforeEach(func() {
					anchor.Labels[workloads.SpaceNameLabel] = "my-space"
				})

				It("allows the deletion anyway", func() {
					Expect(response.Allowed).To(BeTrue())
				})

				It("does not attempt any duplicate validation", func() {
					Expect(spaceDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
					Expect(orgDuplicateValidator.ValidateCreateCallCount()).To(Equal(0))
				})
			})
		})

		Context("spaces", func() {
			BeforeEach(func() {
				anchor = helpers.MakeSpace(namespace, "my-space")
			})

			It("removes the name from the registry", func() {
				Expect(spaceDuplicateValidator.ValidateDeleteCallCount()).To(Equal(1))
				_, _, namespace, name := spaceDuplicateValidator.ValidateDeleteArgsForCall(0)
				Expect(namespace).To(Equal(namespace))
				Expect(name).To(Equal("my-space"))
			})
		})
	})
})
