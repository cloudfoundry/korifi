package integration_test

import (
	"context"

	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CFProcessMutatingWebhook Integration Tests", func() {
	When("a CFProcess record is created", func() {
		const (
			cfAppGUIDLabelKey     = "korifi.cloudfoundry.org/app-guid"
			cfProcessGUIDLabelKey = "korifi.cloudfoundry.org/process-guid"
			cfProcessType         = "test-process-type"
			cfProcessTypeLabelKey = "korifi.cloudfoundry.org/process-type"
			namespace             = "default"
		)

		var (
			cfAppGUID     string
			cfProcessGUID string
		)

		BeforeEach(func() {
			beforeCtx := context.Background()
			cfAppGUID = GenerateGUID()
			cfProcessGUID = GenerateGUID()
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
			Expect(k8sClient.Create(beforeCtx, cfProcess)).To(Succeed())
		})

		AfterEach(func() {
			matchCFProcess := &v1alpha1.CFProcess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfProcessGUID,
					Namespace: namespace,
				},
			}
			Expect(k8sClient.Delete(context.Background(), matchCFProcess)).To(Succeed())
		})

		It("should add the appropriate labels", func() {
			testCtx := context.Background()
			cfProcessLookupKey := types.NamespacedName{Name: cfProcessGUID, Namespace: namespace}
			createdCFProcess := new(v1alpha1.CFProcess)

			Eventually(func() map[string]string {
				err := k8sClient.Get(testCtx, cfProcessLookupKey, createdCFProcess)
				if err != nil {
					return nil
				}
				return createdCFProcess.ObjectMeta.Labels
			}).ShouldNot(BeEmpty(), "CFProcess resource does not have any metadata.labels")

			Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfProcessGUIDLabelKey, cfProcessGUID))
			Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfProcessTypeLabelKey, cfProcessType))
			Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})
	})
})
