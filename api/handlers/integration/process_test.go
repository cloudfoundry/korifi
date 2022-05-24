package integration_test

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Process", func() {
	var (
		namespace      *corev1.Namespace
		processHandler *handlers.ProcessHandler
		appGUID        string
	)

	BeforeEach(func() {
		processRepo := repositories.NewProcessRepo(namespaceRetriever, clientFactory, nsPermissions)

		processHandler = handlers.NewProcessHandler(
			*serverURL,
			processRepo,
			nil,
			nil,
			nil,
		)
		processHandler.RegisterRoutes(router)

		appGUID = generateGUID()
		namespaceGUID := generateGUID()
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceGUID}}
		Expect(
			k8sClient.Create(ctx, namespace),
		).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	Describe("get", func() {
		var process *v1alpha1.CFProcess

		BeforeEach(func() {
			processGUID := generateGUID()
			process = &v1alpha1.CFProcess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      processGUID,
					Namespace: namespace.Name,
					Labels: map[string]string{
						"v1alpha1.cloudfoundry.org/app-guid": appGUID,
					},
				},
				Spec: v1alpha1.CFProcessSpec{
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
					ProcessType: "web",
					Command:     "",
					HealthCheck: v1alpha1.HealthCheck{
						Type: "process",
						Data: v1alpha1.HealthCheckData{
							InvocationTimeoutSeconds: 0,
							TimeoutSeconds:           0,
						},
					},
					DesiredInstances: 1,
					MemoryMB:         500,
					DiskQuotaMB:      512,
					Ports:            []int32{8080},
				},
			}
			Expect(k8sClient.Create(ctx, process)).To(Succeed())
		})

		JustBeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, serverURI("/v3/processes/"+process.Name), nil)
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		})

		When("the user is not authorized to get processes in the space", func() {
			It("returns a not found error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
			})
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, namespace.Name)
			})

			It("returns the process", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPBody(ContainSubstring(fmt.Sprintf(`"guid":"%s"`, process.Name))), rr.Body.String())
			})
		})
	})
})
