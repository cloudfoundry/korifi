package integration_test

import (
	"context"
	"strconv"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CFAppMutatingWebhook Integration Tests", func() {
	When("a CFApp record is created", func() {
		const (
			cfAppLabelKey    = "workloads.cloudfoundry.org/app-guid"
			cfAppRevisionKey = "workloads.cloudfoundry.org/app-rev"
			namespace        = "default"
		)

		var cfAppGUID string

		It(" should add a metadata label on it and it matches metadata.name", func() {
			testCtx := context.Background()
			cfAppGUID = GenerateGUID()
			cfAppName := cfAppGUID + "-app"
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
					Name:         cfAppName,
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(k8sClient.Create(testCtx, cfApp)).To(Succeed())

			cfAppLookupKey := types.NamespacedName{Name: cfAppGUID, Namespace: namespace}
			createdCFApp := new(v1alpha1.CFApp)

			Eventually(func() map[string]string {
				err := k8sClient.Get(testCtx, cfAppLookupKey, createdCFApp)
				if err != nil {
					return nil
				}
				return createdCFApp.ObjectMeta.Labels
			}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "CFApp resource does not have any metadata.labels")

			Expect(createdCFApp.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppLabelKey, cfAppGUID))
			Expect(createdCFApp.Annotations).To(HaveKeyWithValue(cfAppRevisionKey, "0"))

			Expect(k8sClient.Delete(testCtx, cfApp)).To(Succeed())
		})
	})

	When("a CFApp is updated from desiredState STARTED -> STOPPED", func() {
		const (
			cfAppRevisionKey = "workloads.cloudfoundry.org/app-rev"
			namespace        = "default"
			revisionValue    = 8
		)

		var (
			cfAppGUID string
			cfApp     *v1alpha1.CFApp
		)

		BeforeEach(func() {
			testCtx := context.Background()
			cfAppGUID = GenerateGUID()
			cfAppName := cfAppGUID + "-app"
			cfApp = &v1alpha1.CFApp{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFApp",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespace,
					Annotations: map[string]string{
						cfAppRevisionKey: strconv.Itoa(revisionValue),
					},
				},
				Spec: v1alpha1.CFAppSpec{
					Name:         cfAppName,
					DesiredState: "STARTED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(k8sClient.Create(testCtx, cfApp)).To(Succeed())
			cfApp.Status.Conditions = []metav1.Condition{}
			cfApp.Status.ObservedDesiredState = cfApp.Spec.DesiredState
			Expect(k8sClient.Status().Update(testCtx, cfApp)).To(Succeed())

			Eventually(func() string {
				updatedCFApp := &v1alpha1.CFApp{}
				err := cfAppValidatingWebhookClient.Get(context.Background(), types.NamespacedName{Name: cfAppGUID, Namespace: namespace}, updatedCFApp)
				if err != nil {
					return ""
				}
				return string(updatedCFApp.Status.ObservedDesiredState)
			}).Should(Equal(string(cfApp.Spec.DesiredState)))

			cfApp.Spec.DesiredState = v1alpha1.StoppedState
			Expect(k8sClient.Update(testCtx, cfApp)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), cfApp)).To(Succeed())
		})

		It("should eventually increment the rev annotation by 1", func() {
			Eventually(func() string {
				updatedCFApp := &v1alpha1.CFApp{}
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfAppGUID, Namespace: namespace}, updatedCFApp)
				if err != nil {
					return ""
				}
				return getMapKeyValue(updatedCFApp.Annotations, v1alpha1.CFAppRevisionKey)
			}).Should(Equal(strconv.Itoa(revisionValue + 1)))
		})
	})
})
