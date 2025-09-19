package migration_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/migration/migration"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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
