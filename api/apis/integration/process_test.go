package integration_test

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloads "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Process", func() {
	var (
		namespace      *corev1.Namespace
		processHandler *apis.ProcessHandler
		appGUID        string
	)

	BeforeEach(func() {
		processRepo := repositories.NewProcessRepo(k8sClient, namespaceRetriever, clientFactory, nsPermissions)

		processHandler = apis.NewProcessHandler(
			logf.Log.WithName("integration tests"),
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
		var process *workloads.CFProcess

		BeforeEach(func() {
			processGUID := generateGUID()
			process = &workloads.CFProcess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      processGUID,
					Namespace: namespace.Name,
					Labels: map[string]string{
						"workloads.cloudfoundry.org/app-guid": appGUID,
					},
				},
				Spec: workloads.CFProcessSpec{
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
					ProcessType: "web",
					Command:     "",
					HealthCheck: workloads.HealthCheck{
						Type: "process",
						Data: workloads.HealthCheckData{
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
