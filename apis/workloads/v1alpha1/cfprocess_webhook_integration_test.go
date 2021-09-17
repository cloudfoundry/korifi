package v1alpha1_test

import (
	"testing"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = AddToTestSuite("CFProcessWebhook", integrationTestCFProcessWebhook)

func integrationTestCFProcessWebhook(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	when("a CFProcess record is created", func() {
		const (
			cfAppGUID             = "test-app-guid"
			cfAppGUIDLabelKey     = "workloads.cloudfoundry.org/app-guid"
			cfProcessGUID         = "test-process-guid"
			cfProcessGUIDLabelKey = "workloads.cloudfoundry.org/process-guid"
			cfProcessType         = "test-process-type"
			cfProcessTypeLabelKey = "workloads.cloudfoundry.org/process-type"
			namespace             = "default"
		)

		it.Before(func() {
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
					AppRef: v1.LocalObjectReference{
						Name: cfAppGUID,
					},
					ProcessType: cfProcessType,
					HealthCheck: v1alpha1.HealthCheck{
						Type: "http",
					},
					Ports: []int32{},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfProcess)).To(Succeed())
		})

		it("should add the appropriate labels", func() {
			cfProcessLookupKey := types.NamespacedName{Name: cfProcessGUID, Namespace: namespace}
			createdCFProcess := new(v1alpha1.CFProcess)

			g.Eventually(func() map[string]string {
				err := k8sClient.Get(ctx, cfProcessLookupKey, createdCFProcess)
				if err != nil {
					return nil
				}
				return createdCFProcess.ObjectMeta.Labels
			}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "CFProcess resource does not have any metadata.labels")

			g.Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfProcessGUIDLabelKey, cfProcessGUID))
			g.Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfProcessTypeLabelKey, cfProcessType))
			g.Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})
	})
}
