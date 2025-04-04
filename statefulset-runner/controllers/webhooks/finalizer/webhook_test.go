package finalizer_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers/webhooks/finalizer"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ApprevWebhook", func() {
	var workload *korifiv1alpha1.AppWorkload

	BeforeEach(func() {
		workload = &korifiv1alpha1.AppWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
		}
		Expect(adminClient.Create(ctx, workload)).To(Succeed())
	})

	It("sets the finalizer", func() {
		Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(workload), workload)).To(Succeed())
		Expect(workload.Finalizers).To(ConsistOf(finalizer.AppWorkloadFinalizerName))
	})
})
