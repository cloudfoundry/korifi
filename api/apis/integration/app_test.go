package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	. "code.cloudfoundry.org/cf-k8s-controllers/api/apis"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

var _ = Describe("POST /v3/apps endpoint", func() {
	BeforeEach(func() {
		appRepo := new(repositories.AppRepo)
		dropletRepo := new(repositories.DropletRepo)
		processRepo := new(repositories.ProcessRepo)
		routeRepo := new(repositories.RouteRepo)
		domainRepo := new(repositories.DomainRepo)
		podRepo := new(repositories.PodRepo)
		scaleProcess := actions.NewScaleProcess(processRepo).Invoke
		scaleAppProcess := actions.NewScaleAppProcess(appRepo, processRepo, scaleProcess).Invoke

		apiHandler := NewAppHandler(
			logf.Log.WithName("integration tests"),
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			podRepo,
			scaleAppProcess,
			repositories.BuildCRClient,
			k8sConfig,
		)
		apiHandler.RegisterRoutes(router)
	})

	When("on the happy path", func() {
		const (
			appName = "my-test-app"
		)
		var (
			namespace                *corev1.Namespace
			resp                     *http.Response
			testEnvironmentVariables map[string]string
		)

		BeforeEach(func() {
			namespaceGUID := generateGUID()
			namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceGUID}}
			Expect(
				k8sClient.Create(context.Background(), namespace),
			).To(Succeed())

			testEnvironmentVariables = map[string]string{"foo": "foo", "bar": "bar"}
			envJSON, _ := json.Marshal(&testEnvironmentVariables)
			requestBody := fmt.Sprintf(`{
				"name": %q,
				"relationships": {
					"space": {
						"data": {
							"guid": %q 
						}
					}
				},
				"environment_variables": %s
			}`, appName, namespaceGUID, envJSON)

			var err error
			req, err = http.NewRequest("POST", serverURI("/v3/apps"), strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("Content-type", "application/json")

			resp, err = new(http.Client).Do(req)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			k8sClient.Delete(context.Background(), namespace)
		})

		It("creates a CFApp and Secret, returns 201 and an App object as JSON", func() {
			Expect(resp.StatusCode).To(Equal(201))

			var parsedBody map[string]interface{}
			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			Expect(
				json.Unmarshal(body, &parsedBody),
			).To(Succeed())

			Expect(parsedBody).To(MatchKeys(IgnoreExtras, Keys{
				"guid":       Not(BeEmpty()),
				"name":       Equal("my-test-app"),
				"state":      Equal("STOPPED"),
				"created_at": Not(BeEmpty()),
				"relationships": Equal(map[string]interface{}{
					"space": map[string]interface{}{
						"data": map[string]interface{}{
							"guid": namespace.Name,
						},
					},
				}),
			}))

			appGUID := parsedBody["guid"].(string)
			appNSName := types.NamespacedName{
				Name:      appGUID,
				Namespace: namespace.Name,
			}
			var appRecord v1alpha1.CFApp
			Eventually(func() error {
				return k8sClient.Get(context.Background(), appNSName, &appRecord)
			}).Should(Succeed())

			Expect(appRecord.Spec.Name).To(Equal("my-test-app"))
			Expect(appRecord.Spec.DesiredState).To(BeEquivalentTo("STOPPED"))
			Expect(appRecord.Spec.EnvSecretName).NotTo(BeEmpty())

			secretNSName := types.NamespacedName{
				Name:      appRecord.Spec.EnvSecretName,
				Namespace: namespace.Name,
			}
			var secretCR corev1.Secret
			Eventually(func() error {
				return k8sClient.Get(context.Background(), secretNSName, &secretCR)
			}).Should(Succeed())

			Expect(secretCR.Data).To(MatchAllKeys(Keys{
				"foo": BeEquivalentTo(testEnvironmentVariables["foo"]),
				"bar": BeEquivalentTo(testEnvironmentVariables["bar"]),
			}))
		})
	})
})

var _ = Describe("GET /v3/apps endpoint", func() {
	BeforeEach(func() {
		appRepo := new(repositories.AppRepo)
		dropletRepo := new(repositories.DropletRepo)
		processRepo := new(repositories.ProcessRepo)
		routeRepo := new(repositories.RouteRepo)
		domainRepo := new(repositories.DomainRepo)
		podRepo := new(repositories.PodRepo)
		scaleProcess := actions.NewScaleProcess(processRepo).Invoke
		scaleAppProcess := actions.NewScaleAppProcess(appRepo, processRepo, scaleProcess).Invoke

		apiHandler := NewAppHandler(
			logf.Log.WithName("integration tests"),
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			podRepo,
			scaleAppProcess,
			repositories.BuildCRClient,
			k8sConfig,
		)
		apiHandler.RegisterRoutes(router)
	})

	var (
		namespace1 *corev1.Namespace
		namespace2 *corev1.Namespace
		resp       *http.Response
	)

	BeforeEach(func() {
		namespace1GUID := generateGUID()
		namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1GUID}}
		Expect(
			k8sClient.Create(context.Background(), namespace1),
		).To(Succeed())

		namespace2GUID := generateGUID()
		namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2GUID}}
		Expect(
			k8sClient.Create(context.Background(), namespace2),
		).To(Succeed())

	})

	AfterEach(func() {
		k8sClient.Delete(context.Background(), namespace1)
		k8sClient.Delete(context.Background(), namespace2)
	})

	When("when the name filter is provided", func() {
		BeforeEach(func() {
			jacksApp1 := &v1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "jacks-app1-guid",
					Namespace: namespace1.Name,
				},
				Spec: v1alpha1.CFAppSpec{
					Name:         "jacks-app",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(
				k8sClient.Create(context.Background(), jacksApp1),
			).To(Succeed())

			jacksApp2 := &v1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "jacks-app2-guid",
					Namespace: namespace2.Name,
				},
				Spec: v1alpha1.CFAppSpec{
					Name:         "jacks-app",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(
				k8sClient.Create(context.Background(), jacksApp2),
			).To(Succeed())

			noMatch1 := &v1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-match-1",
					Namespace: namespace1.Name,
				},
				Spec: v1alpha1.CFAppSpec{
					Name:         "no-match",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(
				k8sClient.Create(context.Background(), noMatch1),
			).To(Succeed())

			noMatch2 := &v1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-match-2",
					Namespace: namespace1.Name,
				},
				Spec: v1alpha1.CFAppSpec{
					Name:         "no-match",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(
				k8sClient.Create(context.Background(), noMatch2),
			).To(Succeed())

			var err error
			req, err = http.NewRequest("GET", serverURI("/v3/apps?names=jacks-app,nonexistent"), nil)
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("Content-type", "application/json")

			resp, err = new(http.Client).Do(req)
			Expect(err).NotTo(HaveOccurred())

		})

		It("responds with filtered results", Focus, func() {
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var parsedBody map[string]interface{}
			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			Expect(
				json.Unmarshal(body, &parsedBody),
			).To(Succeed())

			Expect(parsedBody["resources"]).To(HaveLen(2))
		})
	})

	// TODO: test space guid
	When("when the space guid filter is provided", func() {
		BeforeEach(func() {
			jacksApp1 := &v1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "jacks-app1-guid",
					Namespace: namespace1.Name,
				},
				Spec: v1alpha1.CFAppSpec{
					Name:         "jacks-app",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(
				k8sClient.Create(context.Background(), jacksApp1),
			).To(Succeed())

			jacksApp2 := &v1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "jacks-app2-guid",
					Namespace: namespace2.Name,
				},
				Spec: v1alpha1.CFAppSpec{
					Name:         "jacks-app",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(
				k8sClient.Create(context.Background(), jacksApp2),
			).To(Succeed())

			noMatch1 := &v1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-match-1",
					Namespace: namespace1.Name,
				},
				Spec: v1alpha1.CFAppSpec{
					Name:         "no-match",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(
				k8sClient.Create(context.Background(), noMatch1),
			).To(Succeed())

			noMatch2 := &v1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-match-2",
					Namespace: namespace1.Name,
				},
				Spec: v1alpha1.CFAppSpec{
					Name:         "no-match",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(
				k8sClient.Create(context.Background(), noMatch2),
			).To(Succeed())

			var err error
			req, err = http.NewRequest("GET", serverURI("/v3/apps?space_guids="+namespace1.Name), nil)
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("Content-type", "application/json")

			resp, err = new(http.Client).Do(req)
			Expect(err).NotTo(HaveOccurred())

		})

		It("responds with filtered results", Focus, func() {
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var parsedBody map[string]interface{}
			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			Expect(
				json.Unmarshal(body, &parsedBody),
			).To(Succeed())

			Expect(parsedBody["resources"]).To(HaveLen(2))
		})
	})
})
