package integration_test

import (
	"net/http"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloads "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Droplet", Serial, func() {
	var (
		namespace      *corev1.Namespace
		dropletHandler *apis.DropletHandler
	)

	BeforeEach(func() {
		clientFactory := repositories.NewUnprivilegedClientFactory(k8sConfig)
		dropletRepo := repositories.NewDropletRepo(k8sClient, clientFactory)

		dropletHandler = apis.NewDropletHandler(
			logf.Log.WithName("integration tests"),
			*serverURL,
			dropletRepo,
		)
		dropletHandler.RegisterRoutes(router)

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
		var droplet *workloads.CFBuild

		BeforeEach(func() {
			dropletGUID := generateGUID()
			droplet = &workloads.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dropletGUID,
					Namespace: namespace.Name,
				},
				Spec: workloads.CFBuildSpec{
					Lifecycle: workloads.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(k8sClient.Create(ctx, droplet)).To(Succeed())
			droplet.Status = workloads.CFBuildStatus{
				Conditions: []metav1.Condition{
					{
						Type:               "Staging",
						Status:             metav1.ConditionFalse,
						Reason:             "foo",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               "Succeeded",
						Status:             metav1.ConditionTrue,
						Reason:             "foo",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
				BuildDropletStatus: &workloads.BuildDropletStatus{
					ProcessTypes: []workloads.ProcessType{},
					Ports:        []int32{},
				},
			}
			Expect(k8sClient.Status().Update(ctx, droplet)).To(Succeed())
		})

		JustBeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, serverURI("/v3/droplets/"+droplet.Name), nil)
			Expect(err).NotTo(HaveOccurred())

			router.ServeHTTP(rr, req)
		})

		When("the user is not authorized to get droplets in the space", func() {
			It("returns a not found error", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusNotFound))
			})
		})
	})
})
