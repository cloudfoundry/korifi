package workloads_test

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	"code.cloudfoundry.org/cf-k8s-controllers/webhooks/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/webhooks/workloads/fake"
)

var _ = Describe("SubnamespaceanchorValidation", func() {
	var (
		ctx               context.Context
		validatingWebhook *workloads.SubnamespaceAnchorValidation
		namespace         string
		lister            *fake.SubnamespaceAnchorLister
		listResult        []hnsv1alpha2.SubnamespaceAnchor
		listerError       error
		anchor            *hnsv1alpha2.SubnamespaceAnchor
		anchorJSON        []byte
		request           admission.Request
		response          admission.Response
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "my-namespace"
		lister = new(fake.SubnamespaceAnchorLister)
		listResult = []hnsv1alpha2.SubnamespaceAnchor{}
		listerError = nil
		validatingWebhook = workloads.NewSubnamespaceAnchorValidation(lister)

		scheme := runtime.NewScheme()
		err := hnsv1alpha2.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		decoder, err := admission.NewDecoder(scheme)
		Expect(err).NotTo(HaveOccurred())
		Expect(validatingWebhook.InjectDecoder(decoder)).To(Succeed())

		anchor = &hnsv1alpha2.SubnamespaceAnchor{}
		anchorJSON = []byte{}
	})

	JustBeforeEach(func() {
		lister.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			cast := list.(*hnsv1alpha2.SubnamespaceAnchorList)
			cast.Items = listResult
			return listerError
		}

		if len(anchorJSON) == 0 {
			var err error
			anchorJSON, err = json.Marshal(anchor)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("subnamespace anchor creation", func() {
		JustBeforeEach(func() {
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

			It("searches for matching org labels in the namespace", func() {
				Expect(lister.ListCallCount()).To(Equal(1))
				_, _, options := lister.ListArgsForCall(0)
				Expect(options).To(ConsistOf(
					client.InNamespace(namespace),
					client.MatchingLabels{workloads.OrgNameLabel: "my-org"},
				))
			})

			When("the org name is unique in the namespace", func() {
				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("the org name already exists in the namespace", func() {
				BeforeEach(func() {
					listResult = []hnsv1alpha2.SubnamespaceAnchor{{}}
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

			It("searches for matching space labels in the namespace", func() {
				Expect(lister.ListCallCount()).To(Equal(1))
				_, _, options := lister.ListArgsForCall(0)
				Expect(options).To(ConsistOf(
					client.InNamespace(namespace),
					client.MatchingLabels{workloads.SpaceNameLabel: "my-space"},
				))
			})

			When("the space name is unique in the namespace", func() {
				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("the space name already exists in the namespace", func() {
				BeforeEach(func() {
					listResult = []hnsv1alpha2.SubnamespaceAnchor{{}}
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
				BeforeEach(func() {
					anchorJSON = []byte(`"[1,`)
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})

			When("listing fails", func() {
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
					listerError = errors.New("oops")
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

			It("searches for matching org labels in the namespace", func() {
				Expect(lister.ListCallCount()).To(Equal(1))
				_, _, options := lister.ListArgsForCall(0)
				Expect(options).To(ConsistOf(
					client.InNamespace(namespace),
					client.MatchingLabels{workloads.OrgNameLabel: "another-org"},
				))
			})

			When("the new org name is unique in the namespace", func() {
				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("the new org name already exists in the namespace", func() {
				BeforeEach(func() {
					listResult = []hnsv1alpha2.SubnamespaceAnchor{{}}
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})

			When("the org name hasn't changed", func() {
				BeforeEach(func() {
					newAnchor.Labels[workloads.OrgNameLabel] = "my-org"
					newAnchor.Labels["something"] = "else"
					listResult = []hnsv1alpha2.SubnamespaceAnchor{*anchor}
				})

				It("succeeds", func() {
					Expect(response.Allowed).To(BeTrue())
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

			It("searches for matching space labels in the namespace", func() {
				Expect(lister.ListCallCount()).To(Equal(1))
				_, _, options := lister.ListArgsForCall(0)
				Expect(options).To(ConsistOf(
					client.InNamespace(namespace),
					client.MatchingLabels{workloads.SpaceNameLabel: "another-space"},
				))
			})

			When("the new space name is unique in the namespace", func() {
				It("allows the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("the new space name already exists in the namespace", func() {
				BeforeEach(func() {
					listResult = []hnsv1alpha2.SubnamespaceAnchor{{}}
				})

				It("denies the request", func() {
					Expect(response.Allowed).To(BeFalse())
				})
			})

			When("the space name hasn't changed", func() {
				BeforeEach(func() {
					newAnchor.Labels[workloads.SpaceNameLabel] = "my-space"
					newAnchor.Labels["something"] = "else"
					listResult = []hnsv1alpha2.SubnamespaceAnchor{*anchor}
				})

				It("succeeds", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})
		})
	})
})
