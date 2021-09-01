package repositories_test

import (
	"context"
	"testing"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
)

var _ = SuiteDescribe("API Shim", func(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		cfAppGUID = "test-app-guid"
		namespace = "default"
	)

	when("multiple Apps exist", func() {
		var (
			cfApp1 *workloadsv1alpha1.CFApp
			cfApp2 *workloadsv1alpha1.CFApp
		)
		it.Before(func() {
			ctx := context.Background()
			cfApp1 = &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-other-app",
					Namespace: namespace,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "test-app1",
					DesiredState: "STOPPED",
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfApp1)).To(Succeed())

			cfApp2 = &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespace,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "test-app2",
					DesiredState: "STOPPED",
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{"java"},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfApp2)).To(Succeed())
		})

		it.After(func() {
			ctx := context.Background()
			g.Expect(k8sClient.Delete(ctx, cfApp1)).To(Succeed())
			g.Expect(k8sClient.Delete(ctx, cfApp2)).To(Succeed())
		})

		it("can fetch the AppRecord CR we're looking for", func() {
			appRepo := repositories.AppRepo{}
			client, err := appRepo.ConfigureClient(k8sConfig)

			app, err := appRepo.FetchApp(client, cfAppGUID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(app.GUID).To(Equal(cfAppGUID))
			g.Expect(app.Name).To(Equal("test-app2"))
			g.Expect(app.SpaceGUID).To(Equal(namespace))
			g.Expect(app.State).To(Equal(repositories.DesiredState("STOPPED")))

			expectedLifecycle := repositories.Lifecycle{
				Data: repositories.LifecycleData{
					Buildpacks: []string{"java"},
					Stack:      "",
				},
			}
			g.Expect(app.Lifecycle).To(Equal(expectedLifecycle))
		})
	})

	when("duplicate Apps exist across namespaces with the same name", func() {
		var (
			cfApp1 *workloadsv1alpha1.CFApp
			cfApp2 *workloadsv1alpha1.CFApp
		)

		it.Before(func() {
			ctx := context.Background()
			g.Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other-namespace"}})).To(Succeed())

			cfApp1 = &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespace,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "test-app1",
					DesiredState: "STOPPED",
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfApp1)).To(Succeed())

			cfApp2 = &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: "other-namespace",
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "test-app2",
					DesiredState: "STOPPED",
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(ctx, cfApp2)).To(Succeed())
		})

		it.After(func() {
			ctx := context.Background()
			g.Expect(k8sClient.Delete(ctx, cfApp1)).To(Succeed())
			g.Expect(k8sClient.Delete(ctx, cfApp2)).To(Succeed())
		})

		it("returns an error", func() {
			appRepo := repositories.AppRepo{}
			client, err := appRepo.ConfigureClient(k8sConfig)

			_, err = appRepo.FetchApp(client, cfAppGUID)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError("duplicate apps exist"))
		})
	})

	when("no Apps exist", func() {
		it("returns an error", func() {
			appRepo := repositories.AppRepo{}
			client, err := appRepo.ConfigureClient(k8sConfig)

			_, err = appRepo.FetchApp(client, "i don't exist")
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError("not found"))
		})
	})
})
