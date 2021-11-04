package integration_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

var _ = Describe("POST /v3/spaces/<space-guid>/actions/apply_manifest endpoint", func() {
	BeforeEach(func() {
		appRepo := new(repositories.AppRepo)
		apiHandler := NewSpaceManifestHandler(
			logf.Log.WithName("integration tests"),
			*serverURL,
			actions.NewApplyManifest(appRepo).Invoke,
			repositories.NewOrgRepo("cf", k8sClient, 1*time.Minute),
			repositories.BuildCRClient,
			k8sConfig,
		)
		apiHandler.RegisterRoutes(router)
	})

	When("on the happy path", func() {
		var (
			namespace *corev1.Namespace
			resp      *http.Response
		)

		const appName = "app1"

		BeforeEach(func() {
			namespaceGUID := generateGUID()
			namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceGUID}}
			Expect(
				k8sClient.Create(context.Background(), namespace),
			).To(Succeed())

			requestBody := fmt.Sprintf(`---
                version: 1
                applications:
                - name: %s`, appName)

			var err error
			req, err = http.NewRequest(
				"POST",
				serverURI("/v3/spaces/", namespaceGUID, "/actions/apply_manifest"),
				strings.NewReader(requestBody),
			)
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("Content-type", "application/x-yaml")
		})

		AfterEach(func() {
			Expect(
				k8sClient.Delete(context.Background(), namespace),
			).To(Succeed())
		})

		JustBeforeEach(func() {
			var err error
			resp, err = new(http.Client).Do(req)
			Expect(err).NotTo(HaveOccurred())
		})

		When("no app with that name exists", func() {
			It("creates the applications in the manifest, returns 202 and a job URI", func() {
				Expect(resp.StatusCode).To(Equal(202))

				body, err := ioutil.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(body).To(BeEmpty())

				Expect(resp.Header.Get("Location")).To(Equal(serverURI("/v3/jobs/sync-space.apply_manifest-", namespace.Name)))

				var appList v1alpha1.CFAppList
				Eventually(func() []v1alpha1.CFApp {
					Expect(
						k8sClient.List(context.Background(), &appList, client.InNamespace(namespace.Name)),
					).To(Succeed())
					return appList.Items
				}).Should(HaveLen(1))

				app1 := appList.Items[0]
				Expect(app1.Spec.Name).To(Equal(appName))
				Expect(app1.Spec.DesiredState).To(BeEquivalentTo("STOPPED"))
				Expect(app1.Spec.Lifecycle.Type).To(BeEquivalentTo("buildpack"))
			})
		})

		When("an app with that name already exists", func() {
			const appGUID = "my-app-guid"

			BeforeEach(func() {
				Expect(
					k8sClient.Create(context.Background(), &v1alpha1.CFApp{
						ObjectMeta: metav1.ObjectMeta{Name: appGUID, Namespace: namespace.Name},
						Spec: v1alpha1.CFAppSpec{
							Name:         appName,
							DesiredState: v1alpha1.StoppedState,
							Lifecycle: v1alpha1.Lifecycle{
								Type: v1alpha1.BuildpackLifecycle,
							},
						},
					}),
				).To(Succeed())

				nsName := types.NamespacedName{Name: appGUID, Namespace: namespace.Name}
				Eventually(func() error {
					return k8sClient.Get(context.Background(), nsName, new(v1alpha1.CFApp))
				}).Should(Succeed())
			})

			It("doesn't change the App, but it returns 202 with a Location", func() {
				Expect(resp.StatusCode).To(Equal(202))

				body, err := ioutil.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(body).To(BeEmpty())

				Expect(resp.Header.Get("Location")).To(Equal(serverURI("/v3/jobs/sync-space.apply_manifest-", namespace.Name)))

				var appList v1alpha1.CFAppList
				Eventually(func() []v1alpha1.CFApp {
					Expect(
						k8sClient.List(context.Background(), &appList, client.InNamespace(namespace.Name)),
					).To(Succeed())
					return appList.Items
				}).Should(HaveLen(1))

				app1 := appList.Items[0]
				Expect(app1.Spec.Name).To(Equal(appName))
				Expect(app1.Spec.DesiredState).To(BeEquivalentTo("STOPPED"))
				Expect(app1.Spec.Lifecycle.Type).To(BeEquivalentTo("buildpack"))
			})
		})
	})
})
