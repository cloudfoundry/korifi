package cleanup_test

import (
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/cleanup"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("BuildCleaner", func() {
	var (
		cleaner                                                       cleanup.BuildCleaner
		appGUID                                                       string
		cfApp                                                         *korifiv1alpha1.CFApp
		namespace                                                     string
		bldCurrent, bldDeletable, bldNotStaged, bldReady, bldOtherApp *korifiv1alpha1.CFBuild
		cleanErr                                                      error
	)

	BeforeEach(func() {
		cleaner = cleanup.NewBuildCleaner(k8sClient, 1)

		namespace = GenerateGUID()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).To(Succeed())

		appGUID = GenerateGUID()

		// sleeps are needed as creation timestamps can't be manipulated
		// directly, and they have a 1 second granularity
		bldCurrent = createSucceededBuild(namespace, appGUID, "current")
		time.Sleep(time.Second)
		bldOtherApp = createSucceededBuild(namespace, "other-app-guid", "other-app")
		time.Sleep(time.Second)
		bldDeletable = createSucceededBuild(namespace, appGUID, "deletable")
		time.Sleep(time.Second)
		bldNotStaged = createBuild(namespace, appGUID, "not-staged")
		time.Sleep(time.Second)
		bldReady = createSucceededBuild(namespace, appGUID, "ready")

		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appGUID,
				Namespace: namespace,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName: "an-app",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
				DesiredState: "STOPPED",
				CurrentDropletRef: corev1.LocalObjectReference{
					Name: bldCurrent.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
	})

	JustBeforeEach(func() {
		cleanErr = cleaner.Clean(ctx, types.NamespacedName{Name: appGUID, Namespace: namespace})
	})

	It("only deletes the expected build", func() {
		Expect(cleanErr).NotTo(HaveOccurred())

		Expect(bldCurrent).To(BeFound())
		Expect(bldOtherApp).To(BeFound())
		Expect(bldNotStaged).To(BeFound())
		Expect(bldReady).To(BeFound())

		Expect(bldDeletable).To(BeNotFound())
	})

	When("the current droplet is not set on the app", func() {
		BeforeEach(func() {
			cfApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{}
			Expect(k8sClient.Update(ctx, cfApp)).To(Succeed())
		})

		It("deletes the two oldest builds", func() {
			Expect(cleanErr).NotTo(HaveOccurred())

			Expect(bldOtherApp).To(BeFound())
			Expect(bldNotStaged).To(BeFound())
			Expect(bldReady).To(BeFound())

			Expect(bldCurrent).To(BeNotFound())
			Expect(bldDeletable).To(BeNotFound())
		})
	})
})

func createBuild(namespace, appGUID, name string) *korifiv1alpha1.CFBuild {
	bld := &korifiv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				controllers.LabelAppGUID: appGUID,
			},
		},
		Spec: korifiv1alpha1.CFBuildSpec{Lifecycle: korifiv1alpha1.Lifecycle{Type: "buildpack"}},
	}
	Expect(k8sClient.Create(ctx, bld)).To(Succeed())
	return bld
}

func createSucceededBuild(namespace, appGUID, name string) *korifiv1alpha1.CFBuild {
	bld := createBuild(namespace, appGUID, name)
	meta.SetStatusCondition(&bld.Status.Conditions, metav1.Condition{
		Type:   korifiv1alpha1.SucceededConditionType,
		Status: metav1.ConditionTrue,
		Reason: "Staged",
	})
	Expect(k8sClient.Status().Update(ctx, bld)).To(Succeed())
	return bld
}
