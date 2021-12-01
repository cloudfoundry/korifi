package workloads_test

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads/fake"
)

var _ = Describe("SubnamespaceanchorValidation", func() {
	var (
		ctx               context.Context
		validatingWebhook *workloads.SubnamespaceAnchorValidation
		namespace         string
		orgNameRegistry   *fake.NameRegistry
		spaceNameRegistry *fake.NameRegistry
		anchor            *hnsv1alpha2.SubnamespaceAnchor
		request           admission.Request
		response          admission.Response
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "my-namespace"
		orgNameRegistry = new(fake.NameRegistry)
		spaceNameRegistry = new(fake.NameRegistry)
		validatingWebhook = workloads.NewSubnamespaceAnchorValidation(orgNameRegistry, spaceNameRegistry)

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
				anchor = &hnsv1alpha2.SubnamespaceAnchor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: namespace,
						Labels: map[string]string{
							workloads.OrgNameLabel: "my-org",
						},
					},
				}
			})

			It("registers the name", func() {
				Expect(spaceNameRegistry.RegisterNameCallCount()).To(Equal(0))
				Expect(orgNameRegistry.RegisterNameCallCount()).To(Equal(1))
				_, actualNamespace, name := orgNameRegistry.RegisterNameArgsForCall(0)
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
					orgNameRegistry.RegisterNameReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "foo"))
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})
		})

		Context("spaces", func() {
			BeforeEach(func() {
				anchor = &hnsv1alpha2.SubnamespaceAnchor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: namespace,
						Labels: map[string]string{
							workloads.SpaceNameLabel: "my-space",
						},
					},
				}
			})

			It("registers the space name", func() {
				Expect(orgNameRegistry.RegisterNameCallCount()).To(Equal(0))
				Expect(spaceNameRegistry.RegisterNameCallCount()).To(Equal(1))
				_, actualNamespace, name := spaceNameRegistry.RegisterNameArgsForCall(0)
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
					spaceNameRegistry.RegisterNameReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "foo"))
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})
		})

		Context("malformed orgs and spaces", func() {
			When("a subnamespace anchor has neither org nor space label", func() {
				BeforeEach(func() {
					anchor = &hnsv1alpha2.SubnamespaceAnchor{
						ObjectMeta: metav1.ObjectMeta{
							Name:      uuid.NewString(),
							Namespace: namespace,
							Labels: map[string]string{
								"something": "else",
							},
						},
					}
				})

				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("a subnamespace anchor has both org and space labels", func() {
				BeforeEach(func() {
					anchor = &hnsv1alpha2.SubnamespaceAnchor{
						ObjectMeta: metav1.ObjectMeta{
							Name:      uuid.NewString(),
							Namespace: namespace,
							Labels: map[string]string{
								workloads.OrgNameLabel:   "my-org",
								workloads.SpaceNameLabel: "my-space",
							},
						},
					}
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
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
			})

			When("the name registry fails", func() {
				BeforeEach(func() {
					anchor = &hnsv1alpha2.SubnamespaceAnchor{
						ObjectMeta: metav1.ObjectMeta{
							Name:      uuid.NewString(),
							Namespace: namespace,
							Labels: map[string]string{
								workloads.OrgNameLabel: "my-org",
							},
						},
					}
					orgNameRegistry.RegisterNameReturns(errors.New("another error"))
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
				anchor = &hnsv1alpha2.SubnamespaceAnchor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: namespace,
						Labels: map[string]string{
							workloads.OrgNameLabel: "my-org",
						},
					},
				}
				newAnchor = anchor.DeepCopy()
				newAnchor.Labels[workloads.OrgNameLabel] = "another-org"
			})

			When("the org name hasn't changed", func() {
				BeforeEach(func() {
					newAnchor.Labels[workloads.OrgNameLabel] = "my-org"
					newAnchor.Labels["something"] = "else"
				})

				It("succeeds without consulting the registry", func() {
					Expect(response.Allowed).To(BeTrue())
					Expect(orgNameRegistry.RegisterNameCallCount()).To(Equal(0))
					Expect(orgNameRegistry.TryLockNameCallCount()).To(Equal(0))
					Expect(spaceNameRegistry.RegisterNameCallCount()).To(Equal(0))
					Expect(spaceNameRegistry.TryLockNameCallCount()).To(Equal(0))
				})
			})

			It("takes a lock on the old name in the registry", func() {
				Expect(orgNameRegistry.TryLockNameCallCount()).To(Equal(1))

				_, actualNamespace, name := orgNameRegistry.TryLockNameArgsForCall(0)
				Expect(actualNamespace).To(Equal(namespace))
				Expect(name).To(Equal("my-org"))
			})

			When("it fails to get the lock on the old name", func() {
				BeforeEach(func() {
					orgNameRegistry.TryLockNameReturns(errors.New("boom!"))
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})

			It("registers the new name", func() {
				Expect(orgNameRegistry.RegisterNameCallCount()).To(Equal(1))
				_, actualNamespace, name := orgNameRegistry.RegisterNameArgsForCall(0)
				Expect(actualNamespace).To(Equal(namespace))
				Expect(name).To(Equal("another-org"))
			})

			When("the new org name is unique in the namespace", func() {
				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})

				It("deletes the old name in the registry", func() {
					Expect(orgNameRegistry.DeregisterNameCallCount()).To(Equal(1))
				})
			})

			When("the new org name already exists in the namespace", func() {
				BeforeEach(func() {
					orgNameRegistry.RegisterNameReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "foo"))
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})

				It("releases the lock on the old name", func() {
					Expect(orgNameRegistry.UnlockNameCallCount()).To(Equal(1))
					_, actualNamespace, name := orgNameRegistry.UnlockNameArgsForCall(0)
					Expect(actualNamespace).To(Equal(namespace))
					Expect(name).To(Equal("my-org"))
				})
			})

			When("registering the new name fails", func() {
				BeforeEach(func() {
					orgNameRegistry.RegisterNameReturns(errors.New("boom!"))
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})

				It("releases the lock on the old name", func() {
					Expect(orgNameRegistry.UnlockNameCallCount()).To(Equal(1))
				})
			})
		})

		Context("spaces", func() {
			BeforeEach(func() {
				anchor = &hnsv1alpha2.SubnamespaceAnchor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: namespace,
						Labels: map[string]string{
							workloads.SpaceNameLabel: "my-space",
						},
					},
				}
				newAnchor = anchor.DeepCopy()
				newAnchor.Labels[workloads.SpaceNameLabel] = "another-space"
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

			It("takes a lock on the old name in the registry", func() {
				Expect(spaceNameRegistry.TryLockNameCallCount()).To(Equal(1))

				_, actualNamespace, name := spaceNameRegistry.TryLockNameArgsForCall(0)
				Expect(actualNamespace).To(Equal(namespace))
				Expect(name).To(Equal("my-space"))
			})

			When("it fails to get the lock on the old name", func() {
				BeforeEach(func() {
					spaceNameRegistry.TryLockNameReturns(errors.New("boom!"))
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})

			It("registers the new name", func() {
				Expect(spaceNameRegistry.RegisterNameCallCount()).To(Equal(1))
				_, actualNamespace, name := spaceNameRegistry.RegisterNameArgsForCall(0)
				Expect(actualNamespace).To(Equal(namespace))
				Expect(name).To(Equal("another-space"))
			})

			When("the new space name is unique in the namespace", func() {
				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})

				It("deletes the old name in the registry", func() {
					Expect(spaceNameRegistry.DeregisterNameCallCount()).To(Equal(1))
				})
			})

			When("the new space name already exists in the namespace", func() {
				BeforeEach(func() {
					spaceNameRegistry.RegisterNameReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "foo"))
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})

				It("releases the lock on the old name", func() {
					Expect(spaceNameRegistry.UnlockNameCallCount()).To(Equal(1))
				})
			})

			When("registering the new name fails", func() {
				BeforeEach(func() {
					spaceNameRegistry.RegisterNameReturns(errors.New("boom!"))
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})

				It("releases the lock on the old name", func() {
					Expect(spaceNameRegistry.UnlockNameCallCount()).To(Equal(1))
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
				anchor = &hnsv1alpha2.SubnamespaceAnchor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: namespace,
						Labels: map[string]string{
							workloads.OrgNameLabel: "my-org",
						},
					},
				}
			})

			It("removes the name from the registry", func() {
				Expect(orgNameRegistry.DeregisterNameCallCount()).To(Equal(1))
				_, actualNamespace, name := orgNameRegistry.DeregisterNameArgsForCall(0)
				Expect(actualNamespace).To(Equal(namespace))
				Expect(name).To(Equal("my-org"))
			})

			When("deregistering fails", func() {
				BeforeEach(func() {
					orgNameRegistry.DeregisterNameReturns(errors.New("boom!"))
				})

				It("denies the deletion", func() {
					Expect(response.Allowed).To(BeFalse())
				})

				When("the failure is a not found error", func() {
					BeforeEach(func() {
						orgNameRegistry.DeregisterNameReturns(k8serrors.NewNotFound(schema.GroupResource{}, "jim"))
					})

					It("allows the deletion", func() {
						Expect(response.Allowed).To(BeTrue())
					})
				})
			})

			When("the anchor has no org or space label", func() {
				BeforeEach(func() {
					delete(anchor.Labels, workloads.OrgNameLabel)
				})

				It("allows the deletion anyway", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("the org has a space label as well", func() {
				BeforeEach(func() {
					anchor.Labels[workloads.SpaceNameLabel] = "my-space"
				})

				It("allows the deletion anyway", func() {
					Expect(response.Allowed).To(BeTrue())
				})

				It("does not attempt to deregister any names", func() {
					Expect(orgNameRegistry.DeregisterNameCallCount()).To(Equal(0))
					Expect(spaceNameRegistry.DeregisterNameCallCount()).To(Equal(0))
				})
			})
		})

		Context("spaces", func() {
			BeforeEach(func() {
				anchor = &hnsv1alpha2.SubnamespaceAnchor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: namespace,
						Labels: map[string]string{
							workloads.SpaceNameLabel: "my-space",
						},
					},
				}
			})

			It("removes the name from the registry", func() {
				Expect(spaceNameRegistry.DeregisterNameCallCount()).To(Equal(1))
				_, namespace, name := spaceNameRegistry.DeregisterNameArgsForCall(0)
				Expect(namespace).To(Equal(namespace))
				Expect(name).To(Equal("my-space"))
			})
		})
	})
})
