package integration_test

import (
	"context"
	"strconv"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CFAppMutatingWebhook Integration Tests", func() {
	When("a CFApp record is created", func() {
		const (
			cfAppLabelKey    = "korifi.cloudfoundry.org/app-guid"
			cfAppRevisionKey = "korifi.cloudfoundry.org/app-rev"
			namespace        = "default"
		)

		var cfAppGUID string

		It(" should add a metadata label on it and it matches metadata.name", func() {
			testCtx := context.Background()
			cfAppGUID = GenerateGUID()
			cfAppName := cfAppGUID + "-app"
			cfApp := &korifiv1alpha1.CFApp{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFApp",
					APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFAppSpec{
					DisplayName:  cfAppName,
					DesiredState: "STOPPED",
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(k8sClient.Create(testCtx, cfApp)).To(Succeed())

			cfAppLookupKey := types.NamespacedName{Name: cfAppGUID, Namespace: namespace}
			createdCFApp := new(korifiv1alpha1.CFApp)

			Eventually(func() map[string]string {
				err := k8sClient.Get(testCtx, cfAppLookupKey, createdCFApp)
				if err != nil {
					return nil
				}
				return createdCFApp.ObjectMeta.Labels
			}).ShouldNot(BeEmpty(), "CFApp resource does not have any metadata.labels")

			Expect(createdCFApp.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppLabelKey, cfAppGUID))
			Expect(createdCFApp.Annotations).To(HaveKeyWithValue(cfAppRevisionKey, "0"))

			Expect(k8sClient.Delete(testCtx, cfApp)).To(Succeed())
		})
	})

	When("a CFApp is updated from desiredState STARTED -> STOPPED", func() {
		const (
			cfAppRevisionKey = "korifi.cloudfoundry.org/app-rev"
			namespace        = "default"
			revisionValue    = 8
		)

		var (
			cfAppGUID string
			cfApp     *korifiv1alpha1.CFApp
		)

		BeforeEach(func() {
			testCtx := context.Background()
			cfAppGUID = GenerateGUID()
			cfAppName := cfAppGUID + "-app"
			cfApp = &korifiv1alpha1.CFApp{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFApp",
					APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespace,
					Annotations: map[string]string{
						cfAppRevisionKey: strconv.Itoa(revisionValue),
					},
				},
				Spec: korifiv1alpha1.CFAppSpec{
					DisplayName:  cfAppName,
					DesiredState: "STARTED",
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(k8sClient.Create(testCtx, cfApp)).To(Succeed())
			Expect(k8s.Patch(testCtx, k8sClient, cfApp, func() {
				cfApp.Status.VCAPServicesSecretName = "vcap-services-secret"
				cfApp.Status.Conditions = []metav1.Condition{}
				cfApp.Status.ObservedDesiredState = cfApp.Spec.DesiredState
			})).To(Succeed())

			Eventually(func() string {
				updatedCFApp := &korifiv1alpha1.CFApp{}
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfAppGUID, Namespace: namespace}, updatedCFApp)
				if err != nil {
					return ""
				}
				return string(updatedCFApp.Status.ObservedDesiredState)
			}).Should(Equal(string(cfApp.Spec.DesiredState)))

			Expect(k8s.Patch(testCtx, k8sClient, cfApp, func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StoppedState
			})).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), cfApp)).To(Succeed())
		})

		It("should eventually increment the rev annotation by 1", func() {
			Eventually(func() string {
				updatedCFApp := &korifiv1alpha1.CFApp{}
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfAppGUID, Namespace: namespace}, updatedCFApp)
				if err != nil {
					return ""
				}
				return getMapKeyValue(updatedCFApp.Annotations, korifiv1alpha1.CFAppRevisionKey)
			}).Should(Equal(strconv.Itoa(revisionValue + 1)))
		})
	})
})
