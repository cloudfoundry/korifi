package workloads_test

import (
	"context"
	"encoding/json"
	"errors"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/webhooks/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/webhooks/workloads/fake"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("CFAppValidatingWebhook Unit Tests", func() {
	const (
		testAppGUID      = "test-app-guid"
		testAppName      = "test-app"
		testAppNamespace = "default"
	)

	var (
		ctx           context.Context
		fakeClient    *fake.CFAppClient
		realDecoder   *admission.Decoder
		dummyCFApp    *workloadsv1alpha1.CFApp
		createRequest admission.Request
	)

	BeforeEach(func() {
		ctx = context.Background()

		fakeClient = new(fake.CFAppClient)
		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			cast := list.(*workloadsv1alpha1.CFAppList)
			cast.Items = []workloadsv1alpha1.CFApp{}
			return nil
		}

		scheme := runtime.NewScheme()
		err := workloadsv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		realDecoder, err = admission.NewDecoder(scheme)
		Expect(err).NotTo(HaveOccurred())

		dummyCFApp = &workloadsv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testAppGUID,
				Namespace: testAppNamespace,
			},
			Spec: workloadsv1alpha1.CFAppSpec{
				Name:         testAppName,
				DesiredState: workloadsv1alpha1.StoppedState,
			},
		}

		cfAppJSON, err := json.Marshal(dummyCFApp)
		Expect(err).NotTo(HaveOccurred())

		createRequest = admission.Request{
			AdmissionRequest: v1.AdmissionRequest{
				Name:      testAppGUID,
				Namespace: testAppNamespace,
				Operation: v1.Create,
				Object: runtime.RawExtension{
					Raw: cfAppJSON,
				},
			},
		}
	})

	When("CFAppValidation Handle is run", func() {
		var validatingWebhook *CFAppValidation

		BeforeEach(func() {
			validatingWebhook = &CFAppValidation{
				Client: fakeClient,
			}

			Expect(validatingWebhook.InjectDecoder(realDecoder)).To(Succeed())
		})

		When("no other apps are present in the api and Handle is called", func() {
			It("should return \"Allowed\"", func() {
				response := validatingWebhook.Handle(ctx, createRequest)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("there is an issue decoding the request", func() {
			BeforeEach(func() {
				cfAppJSON, _ := json.Marshal(dummyCFApp)
				badCFAppJSON := []byte("}" + string(cfAppJSON))

				createRequest = admission.Request{
					AdmissionRequest: v1.AdmissionRequest{
						Name:      testAppGUID,
						Namespace: testAppNamespace,
						Operation: v1.Create,
						Object: runtime.RawExtension{
							Raw: badCFAppJSON,
						},
					},
				}
			})

			It("should return \"Allowed\" false", func() {
				response := validatingWebhook.Handle(ctx, createRequest)
				Expect(response.Allowed).To(BeFalse())
			})
		})

		When("there is an issue listing the CFApps", func() {
			BeforeEach(func() {
				fakeClient.ListReturns(errors.New("this fails on purpose"))
			})

			It("should return \"Allowed\" false", func() {
				response := validatingWebhook.Handle(ctx, createRequest)
				Expect(response.Allowed).To(BeFalse())
			})
		})
	})
})
