package workloads_test

import (
	"context"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/webhooks/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/webhooks/workloads/webhooksfakes"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"errors"
	"testing"
)

func TestUnitValidatingWebhooks(t *testing.T) {
	spec.Run(t, "object", testCFAppValidatingWebhook, spec.Report(report.Terminal{}))

}

func testCFAppValidatingWebhook(t *testing.T, when spec.G, it spec.S) {
	//logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true)))
	g := NewWithT(t)

	const (
		testAppGUID      = "test-app-guid"
		testAppName      = "test-app"
		testAppNamespace = "default"
	)

	var (
		ctx           context.Context
		fakeClient    *webhooksfakes.FakeCFAppClient
		realDecoder   *admission.Decoder
		dummyCFApp    *workloadsv1alpha1.CFApp
		createRequest admission.Request
	)

	it.Before(func() {
		ctx = context.Background()

		fakeClient = new(webhooksfakes.FakeCFAppClient)
		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			cast := list.(*workloadsv1alpha1.CFAppList)
			cast.Items = []workloadsv1alpha1.CFApp{}
			return nil
		}

		scheme := runtime.NewScheme()
		err := workloadsv1alpha1.AddToScheme(scheme)
		g.Expect(err).NotTo(HaveOccurred())

		realDecoder, err = admission.NewDecoder(scheme)
		g.Expect(err).NotTo(HaveOccurred())

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
		g.Expect(err).NotTo(HaveOccurred())

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

	when("CFAppValidation Handle is run", func() {
		var validatingWebhook *CFAppValidation

		it.Before(func() {
			validatingWebhook = &CFAppValidation{
				Client: fakeClient,
			}

			g.Expect(validatingWebhook.InjectDecoder(realDecoder)).To(Succeed())
		})

		when("no other apps are present in the api and Handle is called", func() {
			it("should return \"Allowed\"", func() {
				response := validatingWebhook.Handle(ctx, createRequest)
				g.Expect(response.Allowed).To(BeTrue())
			})
		})

		when("there is an issue decoding the request", func() {
			it.Before(func() {
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

			it("should return \"Allowed\" false", func() {
				response := validatingWebhook.Handle(ctx, createRequest)
				g.Expect(response.Allowed).To(BeFalse())
			})
		})

		when("there is an issue listing the CFApps", func() {
			it.Before(func() {
				fakeClient.ListReturns(errors.New("this fails on purpose"))
			})

			it("should return \"Allowed\" false", func() {
				response := validatingWebhook.Handle(ctx, createRequest)
				g.Expect(response.Allowed).To(BeFalse())
			})
		})
	})

}
