package integration_test

import (
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

var _ = Describe("Build", func() {
	var (
		namespace    *corev1.Namespace
		buildHandler *apis.BuildHandler
	)

	BeforeEach(func() {
		userClientFactory := repositories.NewUnprivilegedClientFactory(k8sConfig)
		buildRepo := repositories.NewBuildRepo(k8sClient, userClientFactory)
		packageRepo := repositories.NewPackageRepo(k8sClient)

		buildHandler = apis.NewBuildHandler(
			logf.Log.WithName("integration tests"),
			*serverURL,
			buildRepo,
			packageRepo,
		)
		buildHandler.RegisterRoutes(router)

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
		var build *workloads.CFBuild

		BeforeEach(func() {
			buildGUID := generateGUID()
			build = &workloads.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildGUID,
					Namespace: namespace.Name,
				},
				Spec: workloads.CFBuildSpec{
					Lifecycle: workloads.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(k8sClient.Create(ctx, build)).To(Succeed())
		})

		JustBeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, serverURI("/v3/builds/"+build.Name), nil)
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		})

		When("the user is not authorized to get builds in the space", func() {
			It("returns a not found error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
			})
		})
	})
})
