package cleanup_test

import (
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/cleanup"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("PackageCleaner", func() {
	var (
		cleaner                                                      cleanup.PackageCleaner
		appGUID                                                      string
		cfApp                                                        *korifiv1alpha1.CFApp
		namespace                                                    string
		pkgCurrent, pkgDeletable, pkgNotReady, pkgReady, pkgOtherApp *korifiv1alpha1.CFPackage
		cleanErr                                                     error
	)

	BeforeEach(func() {
		cleaner = cleanup.NewPackageCleaner(controllersClient, 1)

		namespace = GenerateGUID()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})).To(Succeed())

		appGUID = GenerateGUID()
		buildGUID := GenerateGUID()

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
					Name: buildGUID,
				},
			},
		}
		Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())

		// sleeps are needed as creation timestamps can't be manipulated
		// directly, and they have a 1 second granularity
		pkgCurrent = createReadyPackage(namespace, appGUID, "current")
		time.Sleep(time.Second)
		pkgOtherApp = createReadyPackage(namespace, "other-app-guid", "other-app")
		time.Sleep(time.Second)
		pkgDeletable = createReadyPackage(namespace, appGUID, "deletable")
		time.Sleep(time.Second)
		pkgNotReady = createPackage(namespace, appGUID, "not-ready")
		time.Sleep(time.Second)
		pkgReady = createReadyPackage(namespace, appGUID, "ready")

		Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: buildGUID},
			Spec: korifiv1alpha1.CFBuildSpec{
				PackageRef: corev1.LocalObjectReference{Name: pkgCurrent.Name},
				AppRef:     corev1.LocalObjectReference{Name: appGUID},
				Lifecycle:  korifiv1alpha1.Lifecycle{Type: "buildpack"},
			},
		})).To(Succeed())
	})

	JustBeforeEach(func() {
		cleanErr = cleaner.Clean(ctx, types.NamespacedName{Name: appGUID, Namespace: namespace})
	})

	It("only deletes the expected package", func() {
		Expect(cleanErr).NotTo(HaveOccurred())

		Expect(pkgCurrent).To(BeFound())
		Expect(pkgOtherApp).To(BeFound())
		Expect(pkgNotReady).To(BeFound())
		Expect(pkgReady).To(BeFound())

		Expect(pkgDeletable).To(BeNotFound())
	})

	When("the current droplet is not set on the app", func() {
		BeforeEach(func() {
			cfApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{}
			Expect(k8sClient.Update(ctx, cfApp)).To(Succeed())
		})

		It("deletes the two oldest packages", func() {
			Expect(cleanErr).NotTo(HaveOccurred())

			Expect(pkgOtherApp).To(BeFound())
			Expect(pkgNotReady).To(BeFound())
			Expect(pkgReady).To(BeFound())

			Expect(pkgCurrent).To(BeNotFound())
			Expect(pkgDeletable).To(BeNotFound())
		})
	})
})

func createPackage(namespace, appGUID, name string) *korifiv1alpha1.CFPackage {
	pkg := &korifiv1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				controllers.LabelAppGUID: appGUID,
			},
		},
		Spec: korifiv1alpha1.CFPackageSpec{Type: "bits"},
	}
	Expect(k8sClient.Create(ctx, pkg)).To(Succeed())
	return pkg
}

func createReadyPackage(namespace, appGUID, name string) *korifiv1alpha1.CFPackage {
	pkg := createPackage(namespace, appGUID, name)
	meta.SetStatusCondition(&pkg.Status.Conditions, metav1.Condition{
		Type:   shared.StatusConditionReady,
		Status: metav1.ConditionTrue,
		Reason: "SourceImageSet",
	})
	Expect(k8sClient.Status().Update(ctx, pkg)).To(Succeed())
	return pkg
}
