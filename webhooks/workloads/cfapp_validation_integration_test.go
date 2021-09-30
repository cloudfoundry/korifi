package workloads_test

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CFAppValidatingWebhook Integration Tests", func() {
	const (
		testAppGUID        = "test-app-guid"
		anotherTestAppGUID = "another-test-app-guid"
		testAppName        = "test-app"
		anotherTestAppName = "another-test-app"
		namespace          = "default"
		anotherNSName      = "another"
	)
	var ctx context.Context
	BeforeEach(func() {
		ctx = context.Background()
	})

	When("creating a new CFApp record", func() {
		var cfApp *v1alpha1.CFApp

		BeforeEach(func() {
			cfApp = cfAppInstance(testAppGUID, namespace, testAppName)
		})

		When("no other CFApp exists", func() {
			It("should succeed", func() {
				Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
				Expect(k8sClient.Delete(ctx, cfApp)).To(Succeed())
			})
		})

		When("another CFApp exists with a different name in the same namespace", func() {
			var anotherCFApp *v1alpha1.CFApp

			BeforeEach(func() {
				anotherCFApp = cfAppInstance(anotherTestAppGUID, namespace, anotherTestAppName)
				Expect(k8sClient.Create(ctx, anotherCFApp)).To(Succeed())
			})
			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, anotherCFApp)).To(Succeed())
			})

			It("should succeed", func() {
				Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
				Expect(k8sClient.Delete(ctx, cfApp)).To(Succeed())
			})

		})

		When("another CFApp exists with a same name in a different namespace", func() {
			var (
				anotherNS    *v1.Namespace
				anotherCFApp *v1alpha1.CFApp
			)

			BeforeEach(func() {
				anotherNS = &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: anotherNSName,
					},
				}
				anotherCFApp = cfAppInstance(anotherTestAppGUID, anotherNSName, testAppName)
				Expect(k8sClient.Create(ctx, anotherNS)).To(Succeed())
				Expect(k8sClient.Create(ctx, anotherCFApp)).To(Succeed())
			})
			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, anotherCFApp)).To(Succeed())
				Expect(k8sClient.Delete(ctx, anotherNS)).To(Succeed())
			})

			It("should succeed", func() {
				Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
				Expect(k8sClient.Delete(ctx, cfApp)).To(Succeed())
			})

		})

		When("another CFApp exists with a same name in the same namespace", func() {
			var anotherCFApp *v1alpha1.CFApp

			BeforeEach(func() {
				anotherCFApp = cfAppInstance(anotherTestAppGUID, namespace, testAppName)
				Expect(k8sClient.Create(ctx, anotherCFApp)).To(Succeed())
			})
			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, anotherCFApp)).To(Succeed())
			})

			It("should fail", func() {
				Expect(k8sClient.Create(ctx, cfApp)).NotTo(Succeed())
			})

		})

	})

	When("updating an existing CFApp record", func() {
		var cfApp *v1alpha1.CFApp

		BeforeEach(func() {
			cfApp = cfAppInstance(testAppGUID, namespace, testAppName)
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
		})
		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, cfApp)).To(Succeed())
		})

		When("modifing desiredState", func() {
			It("should succeed", func() {
				desiredStateValue := v1alpha1.StartedState
				cfApp.Spec.DesiredState = desiredStateValue

				Expect(k8sClient.Update(context.Background(), cfApp)).To(Succeed())

				cfAppReturned := v1alpha1.CFApp{}
				namespacedName := types.NamespacedName{
					Namespace: cfApp.Namespace,
					Name:      cfApp.Name,
				}
				Expect(k8sClient.Get(context.Background(), namespacedName, &cfAppReturned)).To(Succeed())
				Expect(cfAppReturned.Spec.DesiredState).To(Equal(desiredStateValue))
			})
		})

		When("modifying spec.Name to match another CFApp spec.Name", func() {
			var anotherCFApp *v1alpha1.CFApp

			BeforeEach(func() {
				anotherCFApp = cfAppInstance(anotherTestAppGUID, namespace, anotherTestAppName)
				Expect(k8sClient.Create(ctx, anotherCFApp)).To(Succeed())
			})
			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, anotherCFApp)).To(Succeed())
			})

			It("should fail", func() {
				cfApp.Spec.Name = anotherTestAppName

				Expect(k8sClient.Update(context.Background(), cfApp)).NotTo(Succeed())
			})
		})
	})
})

func cfAppInstance(cfAppGUID string, namespace string, name string) *v1alpha1.CFApp {
	return &v1alpha1.CFApp{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CFApp",
			APIVersion: v1alpha1.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfAppGUID,
			Namespace: namespace,
		},
		Spec: v1alpha1.CFAppSpec{
			Name:         name,
			DesiredState: "STOPPED",
			Lifecycle: v1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}
}
