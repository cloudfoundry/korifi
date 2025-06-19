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
	Describe("CFApp", test(&korifiv1alpha1.CFApp{
		Spec: korifiv1alpha1.CFAppSpec{
			DisplayName:  "cfapp",
			DesiredState: "STOPPED",
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}))

	Describe("CFBuild", test(&korifiv1alpha1.CFBuild{
		Spec: korifiv1alpha1.CFBuildSpec{
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}))

	Describe("CFDomain", test(&korifiv1alpha1.CFDomain{
		Spec: korifiv1alpha1.CFDomainSpec{
			Name: "example.com",
		},
	}))

	Describe("CFOrg", test(&korifiv1alpha1.CFOrg{
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: "example-org",
		},
	}))

	Describe("CFPackage", test(&korifiv1alpha1.CFPackage{
		Spec: korifiv1alpha1.CFPackageSpec{
			Type: "bits",
		},
	}))

	Describe("CFProcess", test(&korifiv1alpha1.CFProcess{}))

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

	Describe("CFServiceBroker", test(&korifiv1alpha1.CFServiceBroker{}))

	Describe("CFServiceInstance", test(&korifiv1alpha1.CFServiceInstance{
		Spec: korifiv1alpha1.CFServiceInstanceSpec{
			Type: "user-provided",
		},
	}))

	Describe("CFServiceOffering", test(&korifiv1alpha1.CFServiceOffering{}))

	Describe("CFServicePlan", test(&korifiv1alpha1.CFServicePlan{
		Spec: korifiv1alpha1.CFServicePlanSpec{
			Visibility: korifiv1alpha1.ServicePlanVisibility{
				Type: korifiv1alpha1.PublicServicePlanVisibilityType,
			},
		},
	}))

	Describe("CFSpace", test(&korifiv1alpha1.CFSpace{
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: "asdf",
		},
	}))

	Describe("CFTask", test(&korifiv1alpha1.CFTask{}))
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
