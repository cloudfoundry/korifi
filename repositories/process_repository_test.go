package repositories_test

import (
	"context"

	. "code.cloudfoundry.org/cf-k8s-api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ProcessRepository", func() {
	var (
		testCtx     context.Context
		processRepo *ProcessRepository
		client      client.Client
	)

	BeforeEach(func() {
		testCtx = context.Background()

		processRepo = new(ProcessRepository)
		var err error
		client, err = BuildCRClient(k8sConfig)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("FetchProcess", func() {
		var (
			namespace1 *corev1.Namespace
			namespace2 *corev1.Namespace
		)

		BeforeEach(func() {
			namespace1Name := generateGUID()
			namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
			Expect(k8sClient.Create(context.Background(), namespace1)).To(Succeed())

			namespace2Name := generateGUID()
			namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
			Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())
		})

		When("on the happy path", func() {
			var (
				app1GUID string
				app2GUID string
				cfApp1   *workloadsv1alpha1.CFApp
				cfApp2   *workloadsv1alpha1.CFApp

				process1GUID string
				process2GUID string
				cfProcess1   *workloadsv1alpha1.CFProcess
				cfProcess2   *workloadsv1alpha1.CFProcess
			)

			BeforeEach(func() {
				app1GUID = generateGUID()
				app2GUID = generateGUID()
				cfApp1 = initializeAppCR("test-app1", app1GUID, namespace1.Name)
				Expect(k8sClient.Create(context.Background(), cfApp1)).To(Succeed())

				cfApp2 = initializeAppCR("test-app2", app2GUID, namespace2.Name)
				Expect(k8sClient.Create(context.Background(), cfApp2)).To(Succeed())

				process1GUID = generateGUID()
				cfProcess1 = initializeProcessCR(process1GUID, namespace1.Name, app1GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess1)).To(Succeed())

				process2GUID = generateGUID()
				cfProcess2 = initializeProcessCR(process2GUID, namespace2.Name, app2GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess2)).To(Succeed())

			})

			AfterEach(func() {
				k8sClient.Delete(context.Background(), cfApp1)
				k8sClient.Delete(context.Background(), cfApp2)
				k8sClient.Delete(context.Background(), cfProcess1)
				k8sClient.Delete(context.Background(), cfProcess2)
			})

			It("can fetch the Process CR we're looking for", func() {
				process, err := processRepo.FetchProcess(testCtx, client, process1GUID)
				Expect(err).NotTo(HaveOccurred())
				Expect(process.GUID).To(Equal(process1GUID))
				Expect(process.SpaceGUID).To(Equal(namespace1.Name))
				Expect(process.AppGUID).To(Equal(app1GUID))
			})
		})

		When("duplicate Processes exist across namespaces with the same GUIDs", func() {
			var (
				app1GUID string
				app2GUID string
				cfApp1   *workloadsv1alpha1.CFApp
				cfApp2   *workloadsv1alpha1.CFApp

				processGUID string
				cfProcess1  *workloadsv1alpha1.CFProcess
				cfProcess2  *workloadsv1alpha1.CFProcess
			)

			BeforeEach(func() {
				app1GUID = generateGUID()
				app2GUID = generateGUID()
				cfApp1 = initializeAppCR("test-app1", app1GUID, namespace1.Name)
				Expect(k8sClient.Create(context.Background(), cfApp1)).To(Succeed())

				cfApp2 = initializeAppCR("test-app2", app2GUID, namespace2.Name)
				Expect(k8sClient.Create(context.Background(), cfApp2)).To(Succeed())

				processGUID = generateGUID()
				cfProcess1 = initializeProcessCR(processGUID, namespace1.Name, app1GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess1)).To(Succeed())

				cfProcess2 = initializeProcessCR(processGUID, namespace2.Name, app2GUID)
				Expect(k8sClient.Create(context.Background(), cfProcess2)).To(Succeed())

			})

			AfterEach(func() {
				k8sClient.Delete(context.Background(), cfApp1)
				k8sClient.Delete(context.Background(), cfApp2)
				k8sClient.Delete(context.Background(), cfProcess1)
				k8sClient.Delete(context.Background(), cfProcess2)
			})

			It("returns an error", func() {
				_, err := processRepo.FetchProcess(testCtx, client, processGUID)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("duplicate processes exist"))
			})
		})

		When("no Processes exist", func() {
			It("returns an error", func() {
				_, err := processRepo.FetchProcess(testCtx, client, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(NotFoundError{}))
			})
		})
	})
})
