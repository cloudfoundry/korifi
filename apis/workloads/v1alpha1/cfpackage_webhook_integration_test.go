package v1alpha1_test

import (
	"context"
	"testing"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = AddToTestSuite("CFPackageWebhook", integrationTestCFPackageWebhook)

func integrationTestCFPackageWebhook(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)
	when("a CFApp record exists", func() {
		var cfApp *v1alpha1.CFApp
		const (
			cfAppGUID     = "test-app-guid"
			cfPackageGUID = "test-package-guid"
			cfPackageType = "bits"
			namespace     = "default"
			cfAppLabelKey = "workloads.cloudfoundry.org/app-guid"
		)
		it.Before(func() {
			cfApp = &v1alpha1.CFApp{
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
			g.Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())
		})
		when("a CFPackage record referencing the CFAPP is created", func() {
			it.Before(func() {
				cfPackage := &v1alpha1.CFPackage{
					TypeMeta: metav1.TypeMeta{
						Kind:       "CFPackage",
						APIVersion: v1alpha1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfPackageGUID,
						Namespace: namespace,
					},
					Spec: v1alpha1.CFPackageSpec{
						Type: cfPackageType,
						AppRef: v1alpha1.ResourceReference{
							Name: cfAppGUID,
						},
					},
				}
				g.Expect(k8sClient.Create(context.Background(), cfPackage)).To(Succeed())
			})

			it("should have CFAppGUID metadata label on it and its value should matches spec.appRef", func() {
				//Fetching the created CFPackage resource
				cfPackageLookupKey := types.NamespacedName{Name: cfPackageGUID, Namespace: namespace}
				createdCFPackage := new(v1alpha1.CFPackage)

				g.Eventually(func() map[string]string {
					err := k8sClient.Get(context.Background(), cfPackageLookupKey, createdCFPackage)
					if err != nil {
						return nil
					}
					return createdCFPackage.ObjectMeta.Labels
				}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty())

				g.Expect(createdCFPackage.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppLabelKey, cfAppGUID))
			})

		})

	})
}
