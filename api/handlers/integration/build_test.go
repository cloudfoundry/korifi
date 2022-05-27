package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Build", func() {
	var (
		namespace    *corev1.Namespace
		buildHandler *handlers.BuildHandler
	)

	BeforeEach(func() {
		buildRepo := repositories.NewBuildRepo(namespaceRetriever, clientFactory)
		packageRepo := repositories.NewPackageRepo(clientFactory, namespaceRetriever, nsPermissions)
		decoderValidator, err := handlers.NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		buildHandler = handlers.NewBuildHandler(
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
		var build *korifiv1alpha1.CFBuild

		BeforeEach(func() {
			buildGUID := generateGUID()
			build = &korifiv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildGUID,
					Namespace: namespace.Name,
				},
				Spec: korifiv1alpha1.CFBuildSpec{
					Lifecycle: korifiv1alpha1.Lifecycle{
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
		var cfPackage *korifiv1alpha1.CFPackage

		BeforeEach(func() {
			packageGUID := generateGUID()
			cfPackage = &korifiv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      packageGUID,
					Namespace: namespace.Name,
				},
				Spec: korifiv1alpha1.CFPackageSpec{
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
			It("returns an unprocessable entity error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
			})
		})
	})
})
