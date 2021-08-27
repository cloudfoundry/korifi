// +build !integration

package webhooks_test

import (
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/webhooks"
	"code.cloudfoundry.org/cf-k8s-controllers/webhooks/webhooksfakes"
	"context"
	"fmt"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"testing"
)

const (
	testAppGUID      = "test-app-guid"
	testAppName      = "test-app"
	testAppNamespace = "default"
)

func TestValidatingWebhooks(t *testing.T) {
	spec.Run(t, "object", testCFAppValidatingWebhook, spec.Report(report.Terminal{}))

}

func testCFAppValidatingWebhook(t *testing.T, when spec.G, it spec.S) {
	//logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true)))
	Expect := NewWithT(t).Expect

	var (
		ctx           context.Context
		fakeClient    *webhooksfakes.FakeCFAppClient
		realDecoder	*admission.Decoder
		fakeDecoder   *webhooksfakes.FakeCFAppDecoder
		dummyCFApp    *workloadsv1alpha1.CFApp
		createRequest admission.Request
	)
	it.Before(func() {
		//ctx = context.Background()
		// Create a mock CFAppClient
		fakeClient = new(webhooksfakes.FakeCFAppClient)
		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			cast := list.(*workloadsv1alpha1.CFAppList)
			cast.Items = []workloadsv1alpha1.CFApp{}
			return nil
		}

		// TODO: do a better job mocking the decoder so there isn't a nil pointer!
		fakeDecoder = new(webhooksfakes.FakeCFAppDecoder)
		realDecoder = &admission.Decoder{}

		// TODO: confirm that you actually use scheme for something
		//err := workloadsv1alpha1.AddToScheme(scheme.Scheme)
		//Expect(err).NotTo(HaveOccurred())

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
		// TODO: something is wrong with decoder fix on Monday!
		cfAppJSON, _ := json.Marshal(dummyCFApp)
		createRequest = admission.Request{
			AdmissionRequest: v1.AdmissionRequest{
				Name:      testAppGUID,
				Namespace: testAppNamespace,
				Operation: v1.Create,
				Object: runtime.RawExtension{
					Raw:  cfAppJSON,
					Object: nil,
				},
				OldObject: runtime.RawExtension{
					Raw:    nil,
					Object: nil,
				},
			},
		}

	})

	when("CFAppValidation Handle is run", func() {
		var validatingWebhook *CFAppValidation
		validatingWebhook = &CFAppValidation{
			Client: fakeClient,
		}
		validatingWebhook.InjectDecoder(realDecoder)
		//Expect().To(Succeed())

		when("no other apps are present in the api and Handle is called", func() {
			it("should return \"Allowed\"", func() {
				fmt.Println(fakeDecoder)
				Expect(true).To(BeTrue())
				response := validatingWebhook.Handle(ctx, createRequest)
				fmt.Println(response)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})

}
