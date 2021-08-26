package webhooks_test

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	"context"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

var _ = AddToTestSuite("CFAppReconciler", testCFAppValidation)

func testCFAppValidation(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)
	when("creating a new CFApp record", func() {
		when("no other CFApp exists", func() {
			const (
				cfAppGUID = "test-app-guid"
				namespace = "default"
			)

			it("should succeed", func() {
				ctx := context.Background()
				cfApp := &v1alpha1.CFApp{
					TypeMeta: metav1.TypeMeta{
						Kind:       "CFApp",
						APIVersion: v1alpha1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfAppGUID,
						Namespace: namespace,
					},
					Spec: v1alpha1.CFAppSpec{
						Name:         "test-app",
						DesiredState: "STOPPED",
						Lifecycle: v1alpha1.Lifecycle{
							Type: "buildpack",
						},
					},
				}
				g.Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
			})

		})
	})
}
