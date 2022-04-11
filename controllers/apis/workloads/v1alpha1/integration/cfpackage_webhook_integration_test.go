package integration_test

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CFPackageMutatingWebhook Integration Tests", func() {
	When("a CFApp record exists", func() {
		const (
			cfAppGUIDLabelKey = "workloads.cloudfoundry.org/app-guid"
			cfPackageType     = "bits"
			namespace         = "default"
		)

		var (
			cfApp         *v1alpha1.CFApp
			cfAppGUID     string
			cfPackageGUID string
		)

		BeforeEach(func() {
			cfAppGUID = GenerateGUID()
			cfPackageGUID = GenerateGUID()
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
			Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), cfApp)).To(Succeed())
		})

		When("a CFPackage record referencing the CFAPP is created", func() {
			BeforeEach(func() {
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
						AppRef: v1.LocalObjectReference{
							Name: cfAppGUID,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), cfPackage)).To(Succeed())
			})

			AfterEach(func() {
				cfPackage := &v1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfPackageGUID,
						Namespace: namespace,
					},
				}
				Expect(k8sClient.Delete(context.Background(), cfPackage)).To(Succeed())
			})

			It("should have CFAppGUID metadata label on it and its value should matches spec.appRef", func() {
				cfPackageLookupKey := types.NamespacedName{Name: cfPackageGUID, Namespace: namespace}
				createdCFPackage := new(v1alpha1.CFPackage)

				Eventually(func() map[string]string {
					err := k8sClient.Get(context.Background(), cfPackageLookupKey, createdCFPackage)
					if err != nil {
						return nil
					}
					return createdCFPackage.ObjectMeta.Labels
				}).ShouldNot(BeEmpty(), "CFPackage resource does not have any metadata.labels")

				Expect(createdCFPackage.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppGUIDLabelKey, cfAppGUID))
			})
		})
	})
})
