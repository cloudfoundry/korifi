package networking_test

import (
	"context"
	"encoding/json"
	"errors"

	networkingv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/networking/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	rootNamespace = "cf"
)

var _ = Describe("CF Domain Validation", func() {
	var (
		ctx context.Context

		realDecoder     *admission.Decoder
		request         admission.Request
		response        admission.Response
		fakeClient      *fake.Client
		existingDomains []networkingv1alpha1.CFDomain
		listDomainsErr  error

		validatingWebhook *networking.CFDomainValidation
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		err := networkingv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())
		realDecoder, err = admission.NewDecoder(scheme)
		Expect(err).NotTo(HaveOccurred())

		listDomainsErr = nil

		fakeClient = new(fake.Client)
		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			switch list := list.(type) {
			case *networkingv1alpha1.CFDomainList:
				existingDomainList := networkingv1alpha1.CFDomainList{
					Items: existingDomains,
				}
				existingDomainList.DeepCopyInto(list)
				return listDomainsErr
			default:
				panic("FakeClient List provided an unexpected object type")
			}
		}

		validatingWebhook = networking.NewCFDomainValidation(fakeClient)
		Expect(validatingWebhook.InjectDecoder(realDecoder)).To(Succeed())
	})

	Describe("Create", func() {
		DescribeTable("returns validation",
			func(requestDomainName string, existingDomainNames []string, allowed bool) {
				existingDomains = make([]networkingv1alpha1.CFDomain, len(existingDomainNames))
				for _, existingDomainName := range existingDomainNames {
					existingDomains = append(existingDomains, createCFDomain(existingDomainName))
				}

				requestDomainCR := createCFDomain(requestDomainName)
				cfDomainJSON, err := json.Marshal(requestDomainCR)
				Expect(err).NotTo(HaveOccurred())
				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      requestDomainCR.Name,
						Namespace: requestDomainCR.Namespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: cfDomainJSON,
						},
					},
				}

				Expect(validatingWebhook.Handle(ctx, request).Allowed).To(Equal(allowed))
			},
			Entry("no domains exist", `foo.example.com`, []string{}, true),
			Entry("an existing not-quite matching subdomain exists", `foo.example.com`, []string{"ample.com"}, true),
			Entry("an existing matching domain exists", `foo.example.com`, []string{"bar.example.com"}, true),
			Entry("an existing matching domain exists(2)", `foo.example.com`, []string{"foo.bar.example.com"}, true),
			Entry("an existing matching subdomain exists", `foo.example.com`, []string{"example.com"}, false),
			Entry("an existing matching subdomain exists(2)", `foo.example.com`, []string{"bar.foo.example.com"}, false),
		)

		When("there is an issue decoding the request", func() {
			BeforeEach(func() {
				cfDomain := createCFDomain("foo.example.com")
				cfDomainJSON, err := json.Marshal(cfDomain)
				Expect(err).NotTo(HaveOccurred())
				badCFDomainJSON := []byte("}" + string(cfDomainJSON))

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      cfDomain.Name,
						Namespace: cfDomain.Namespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: badCFDomainJSON,
						},
					},
				}

				response = validatingWebhook.Handle(ctx, request)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})

		When("there is an issue listing shared CFDomains", func() {
			BeforeEach(func() {
				cfDomain := createCFDomain("foo.example.com")
				cfDomainJSON, err := json.Marshal(cfDomain)
				Expect(err).NotTo(HaveOccurred())

				request = admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Name:      cfDomain.Name,
						Namespace: cfDomain.Namespace,
						Operation: admissionv1.Create,
						Object: runtime.RawExtension{
							Raw: cfDomainJSON,
						},
					},
				}
				listDomainsErr = errors.New("boom")
				response = validatingWebhook.Handle(ctx, request)
			})

			It("denies the request", func() {
				Expect(response.Allowed).To(BeFalse())
			})
		})
	})
})

func createCFDomain(name string) networkingv1alpha1.CFDomain {
	return networkingv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: rootNamespace,
		},
		Spec: networkingv1alpha1.CFDomainSpec{
			Name: name,
		},
	}
}
