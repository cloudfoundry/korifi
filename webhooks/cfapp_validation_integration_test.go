package webhooks_test

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	"context"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
)

const (
	testAppGUID = "test-app-guid"
	testAppName = "test-app"
	namespace   = "default"
	kind        = "CFApp"
)

var _ = AddToTestSuite("CFAppReconciler", testCFAppValidation)

func cfAppInstance(cfAppGUID string, namespace string, name string) *v1alpha1.CFApp {
	return &v1alpha1.CFApp{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
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

func testCFAppValidation(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)
	ctx := context.Background()
	when("creating a new CFApp record", func() {

		cfApp := cfAppInstance(testAppGUID, namespace, testAppName)

		when("no other CFApp exists", func() {
			it("should succeed", func() {
				g.Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
			})
			it.After(func() {
				g.Expect(k8sClient.Delete(ctx, cfApp)).To(Succeed())
			})
		})

		when("Another CFApp exists with a different name in the same namespace", func() {

			anotherCFApp := cfAppInstance("another-test-app-guid", namespace, "another-test-app")

			it.Before(func() {
				g.Expect(k8sClient.Create(ctx, anotherCFApp)).To(Succeed())
			})

			it("should succeed", func() {
				g.Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
			})

			it.After(func() {
				g.Expect(k8sClient.Delete(ctx, anotherCFApp)).To(Succeed())
				g.Expect(k8sClient.Delete(ctx, cfApp)).To(Succeed())
			})
		})

		when("Another CFApp exists with a same name in a different namespace", func() {

			anotherNSName := "another"
			anotherNS := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: anotherNSName,
				},
			}

			anotherCFApp := cfAppInstance("another-test-app-guid", anotherNSName, "another-test-app")

			it.Before(func() {
				g.Expect(k8sClient.Create(ctx, anotherNS)).To(Succeed())
				g.Expect(k8sClient.Create(ctx, anotherCFApp)).To(Succeed())
			})

			it("should succeed", func() {
				g.Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
			})

			it.After(func() {
				g.Expect(k8sClient.Delete(ctx, anotherCFApp)).To(Succeed())
				g.Expect(k8sClient.Delete(ctx, anotherNS)).To(Succeed())
				g.Expect(k8sClient.Delete(ctx, cfApp)).To(Succeed())
			})
		})

		when("Another CFApp exists with a same name in the same namespace", func() {

			anotherCFApp := cfAppInstance("another-test-app-guid", namespace, testAppName)

			it.Before(func() {
				g.Expect(k8sClient.Create(ctx, anotherCFApp)).To(Succeed())
			})

			it("should fail", func() {
				g.Expect(k8sClient.Create(ctx, cfApp)).NotTo(Succeed())
			})

			it.After(func() {
				g.Expect(k8sClient.Delete(ctx, anotherCFApp)).To(Succeed())
			})
		})

	})

	when("updating an existing CFApp record", func() {
		cfApp := cfAppInstance(testAppGUID, namespace, testAppName)

		it.Before(func() {
			g.Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
		})

		it.After(func() {
			g.Expect(k8sClient.Delete(ctx, cfApp)).To(Succeed())
		})
		when ("modifing desiredState", func() {

			it("should succeed", func() {
				cfAppUpdated := v1alpha1.CFApp{}
				cfApp.DeepCopyInto(&cfAppUpdated)
				cfAppUpdated.Spec.DesiredState = v1alpha1.StartedState

				g.Expect(k8sClient.Update(context.Background(), &cfAppUpdated)).To(Succeed())

				cfAppReturned := v1alpha1.CFApp{}
				namespacedName := types.NamespacedName{
					Namespace: cfAppUpdated.Namespace,
					Name:      cfAppUpdated.Name,
				}
				g.Expect(k8sClient.Get(context.Background(), namespacedName, &cfAppReturned)).To(Succeed())
				g.Expect(cfAppReturned.Spec.DesiredState).To(Equal(v1alpha1.StoppedState))
			})
		})

		when("modifying spec.Name to match another CFApp spec.Name", func() {
			anotherCFApp := cfAppInstance("another-test-app-guid", namespace, "another-test-app")

			it.Before(func() {
				g.Expect(k8sClient.Create(ctx, anotherCFApp)).To(Succeed())
			})

			it("should succeed", func() {
				cfAppUpdated := v1alpha1.CFApp{}
				cfApp.DeepCopyInto(&cfAppUpdated)
				cfAppUpdated.Spec.Name = "another-test-app"

				g.Expect(k8sClient.Update(context.Background(), &cfAppUpdated)).To(Succeed())

			})

			it.After(func() {
				g.Expect(k8sClient.Delete(ctx, anotherCFApp)).To(Succeed())
				g.Expect(k8sClient.Delete(ctx, cfApp)).To(Succeed())
			})
		})


	})
}
