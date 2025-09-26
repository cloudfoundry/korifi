package migration_test

import (
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/migration/migration"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Executor", func() {
	Describe("CFRoute", test(&korifiv1alpha1.CFRoute{
		Spec: korifiv1alpha1.CFRouteSpec{
			Host: "example",
			Path: "/example",
			DomainRef: corev1.ObjectReference{
				Name: "example.com",
			},
		},
	}))

	Describe("CFServiceBinding", test(&korifiv1alpha1.CFServiceBinding{
		Spec: korifiv1alpha1.CFServiceBindingSpec{
			Type: "key",
		},
	}))

	Describe("CFServiceBinding Uniqueness", func() {
		var binding *korifiv1alpha1.CFServiceBinding

		BeforeEach(func() {
			binding = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: testNamespace,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					DisplayName: tools.PtrTo("my-binding"),
					Service: corev1.ObjectReference{
						Kind:       "CFServiceInstance",
						APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
						Name:       "my-service",
					},
					AppRef: corev1.LocalObjectReference{
						Name: "my-app",
					},
					Type: korifiv1alpha1.CFServiceBindingTypeApp,
				},
			}
			Expect(
				k8sClient.Create(ctx, binding),
			).To(Succeed())

			Expect(k8sClient.Create(ctx, &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      binding.Name,
					Namespace: testNamespace,
					Annotations: map[string]string{
						coordination.NameAnnotation: "sb::my-ap::::my-service",
					},
				},
				Spec: coordinationv1.LeaseSpec{},
			})).To(Succeed())
		})

		It("deletes the old lease", func() {
			Eventually(func(g Gomega) {
				leases := &coordinationv1.LeaseList{}

				g.Expect(k8sClient.List(ctx, leases, client.MatchingLabels{
					coordination.NameAnnotation: "sb::my-app::::my-service",
				})).To(Succeed())
				g.Expect(leases.Items).To(BeEmpty())
			}).Should(Succeed())
		})

		It("creates a new lease in the new format", func() {
			Eventually(func(g Gomega) {
				leases := &coordinationv1.LeaseList{}

				g.Expect(k8sClient.List(ctx, leases, client.MatchingLabels{
					coordination.NameAnnotation: fmt.Sprintf("sb::my-app::%s::my-service::my-binding", testNamespace),
				})).To(Succeed())
				g.Expect(leases.Items).To(HaveLen(1))
			}).Should(Succeed())
		})
	})
})

func test(testObj client.Object) func() {
	GinkgoHelper()

	return func() {
		var (
			obj      client.Object
			migrator *migration.Migrator
		)

		BeforeEach(func() {
			migrator = migration.New(k8sClient, "abc123", 1)

			obj = testObj.DeepCopyObject().(client.Object)

			obj.SetName(uuid.NewString())
			obj.SetNamespace(testNamespace)
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		})

		JustBeforeEach(func() {
			Expect(migrator.Run(ctx)).To(Succeed())
		})

		It("sets the migrated_by label", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
				g.Expect(obj.GetLabels()).To(HaveKeyWithValue(migration.MigratedByLabelKey, "abc123"))
			}).Should(Succeed())
		})
	}
}
