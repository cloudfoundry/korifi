package v1alpha1_test

import (
	"testing"

	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCFProcessWebhook(t *testing.T) {
	spec.Run(t, "CFProcess Webhook", testCFProcessWebhook, spec.Report(report.Terminal{}))

}

func testCFProcessWebhook(t *testing.T, when spec.G, it spec.S) {
	const (
		cfAppGUID             = "test-app-guid"
		cfAppGUIDLabelKey     = "workloads.cloudfoundry.org/app-guid"
		cfProcessGUID         = "test-process-guid"
		cfProcessGUIDLabelKey = "workloads.cloudfoundry.org/process-guid"
		cfProcessType         = "test-process-type"
		cfProcessTypeLabelKey = "workloads.cloudfoundry.org/process-type"
		namespace             = "default"
	)

	g := NewWithT(t)

	when("there are no existing labels on the CFProcess record", func() {
		it("should add a process-guid label matching metadata.name", func() {
			cfProcess := &v1alpha1.CFProcess{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFProcess",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfProcessGUID,
					Namespace: namespace,
				},
				Spec: v1alpha1.CFProcessSpec{},
			}

			cfProcess.Default()
			g.Expect(cfProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfProcessGUIDLabelKey, cfProcessGUID))
		})

		it("should add a process-type label matching spec.processType", func() {
			cfProcess := &v1alpha1.CFProcess{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFProcess",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfProcessGUID,
					Namespace: namespace,
				},
				Spec: v1alpha1.CFProcessSpec{
					ProcessType: cfProcessType,
				},
			}

			cfProcess.Default()
			g.Expect(cfProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfProcessTypeLabelKey, cfProcessType))
		})

		it("should add an app-guid label matching spec.appRef.name", func() {
			cfProcess := &v1alpha1.CFProcess{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFProcess",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfProcessGUID,
					Namespace: namespace,
				},
				Spec: v1alpha1.CFProcessSpec{
					AppRef: v1alpha1.ResourceReference{
						Name: cfAppGUID,
					},
				},
			}

			cfProcess.Default()
			g.Expect(cfProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})
	})

	when("there are other existing labels on the CFProcess record", func() {
		it("should preserve the other labels", func() {
			cfProcess := &v1alpha1.CFProcess{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFProcess",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfProcessGUID,
					Namespace: namespace,
					Labels: map[string]string{
						"anotherLabel": "process-label",
					},
				},
				Spec: v1alpha1.CFProcessSpec{},
			}

			cfProcess.Default()
			g.Expect(cfProcess.ObjectMeta.Labels).To(HaveLen(4))
			g.Expect(cfProcess.ObjectMeta.Labels).To(HaveKeyWithValue("anotherLabel", "process-label"))
		})
	})
}
