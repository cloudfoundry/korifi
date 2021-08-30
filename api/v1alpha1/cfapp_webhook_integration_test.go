// +build integration

package v1alpha1_test

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	"context"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

var _ = AddToTestSuite("CFAppWebhook", integrationTestCFAppWebhook)

func integrationTestCFAppWebhook(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)
	when("a CFApp record is created", func() {
		const (
			cfAppGUID     = "test-app-guid"
			namespace     = "default"
			cfAppLabelKey = "apps.cloudfoundry.org/appGuid"
		)
		it(" should add a metadata label on it and it matches metadata.name", func() {
			ctx := context.Background()
			//Creating a CFApp resource
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

			//Fetching the created CFApp resource
			cfAppLookupKey := types.NamespacedName{Name: cfAppGUID, Namespace: namespace}
			createdCFApp := new(v1alpha1.CFApp)

			g.Eventually(func() map[string]string {
				err := k8sClient.Get(ctx, cfAppLookupKey, createdCFApp)
				if err != nil {
					return nil
				}
				return createdCFApp.ObjectMeta.Labels
			}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty())

			g.Expect(createdCFApp.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppLabelKey, cfAppGUID))

		})
	})

}
