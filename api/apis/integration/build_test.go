package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
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
		packageRepo := repositories.NewPackageRepo(k8sClient, userClientFactory)
		decoderValidator, err := apis.NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		buildHandler = apis.NewBuildHandler(
			logf.Log.WithName("integration tests"),
			*serverURL,
			buildRepo,
			packageRepo,
			decoderValidator,
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

	Describe("create", func() {
		var cfPackage *workloads.CFPackage

		BeforeEach(func() {
			packageGUID := generateGUID()
			cfPackage = &workloads.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      packageGUID,
					Namespace: namespace.Name,
				},
				Spec: workloads.CFPackageSpec{
					Type: "bits",
				},
			}
			Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())
		})

		JustBeforeEach(func() {
			payload := payloads.BuildCreate{
				Package: &payloads.RelationshipData{
					GUID: cfPackage.Name,
				},
			}
			buildBody, err := json.Marshal(payload)
			Expect(err).NotTo(HaveOccurred())

			req, err = http.NewRequestWithContext(ctx, http.MethodPost, serverURI("/v3/builds"), bytes.NewReader(buildBody))
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		})

		When("the user is not authorized to create builds in the space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceManagerRole.Name, namespace.Name)
			})

			It("returns a forbidden error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusForbidden))
			})
		})

		When("the user is not authorized to get the package", func() {
			It("returns a not found error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
			})
		})
	})
})
