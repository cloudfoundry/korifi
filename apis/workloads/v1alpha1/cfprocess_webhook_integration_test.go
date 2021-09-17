package v1alpha1_test

import (
	"testing"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
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

		it("should add a process-guid label to match metadata.name", func() {
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
						Name: "",
					},
					ProcessType: "",
					HealthCheck: v1alpha1.HealthCheck{
						Type: "http",
					},
					Ports: []int32{},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfProcess)).To(Succeed())

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
		})

		it("should add a process-type label to match spec.processType", func() {
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
						Name: "",
					},
					ProcessType: cfProcessType,
					HealthCheck: v1alpha1.HealthCheck{
						Type: "http",
					},
					Ports: []int32{},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfProcess)).To(Succeed())

			cfProcessLookupKey := types.NamespacedName{Name: cfProcessGUID, Namespace: namespace}
			createdCFProcess := new(v1alpha1.CFProcess)

			g.Eventually(func() map[string]string {
				err := k8sClient.Get(ctx, cfProcessLookupKey, createdCFProcess)
				if err != nil {
					return nil
				}
				return createdCFProcess.ObjectMeta.Labels
			}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "CFProcess resource does not have any metadata.labels")

			g.Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfProcessTypeLabelKey, cfProcessType))
		})

		it("should add an app-guid label to match spec.appRef.name", func() {
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
					ProcessType: "",
					HealthCheck: v1alpha1.HealthCheck{
						Type: "http",
					},
					Ports: []int32{},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfProcess)).To(Succeed())

			cfProcessLookupKey := types.NamespacedName{Name: cfProcessGUID, Namespace: namespace}
			createdCFProcess := new(v1alpha1.CFProcess)

			g.Eventually(func() map[string]string {
				err := k8sClient.Get(ctx, cfProcessLookupKey, createdCFProcess)
				if err != nil {
					return nil
				}
				return createdCFProcess.ObjectMeta.Labels
			}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "CFProcess resource does not have any metadata.labels")

			g.Expect(createdCFProcess.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
		})
	})
}
