package integration_test

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("RunWorkloadsController", func() {
	var (
		ctx           context.Context
		runWorkload   *korifiv1alpha1.RunWorkload
		namespaceName string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespaceName = prefixedGUID("ns")
		createNamespace(ctx, k8sClient, namespaceName)

		runWorkload = &korifiv1alpha1.RunWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prefixedGUID("rw"),
				Namespace: namespaceName,
			},
			Spec: korifiv1alpha1.RunWorkloadSpec{
				GUID:    prefixedGUID("process"),
				Version: "1",
				AppGUID: "my-app",

				ProcessType:      "web",
				Image:            "my-image",
				Command:          []string{"do-it", "with", "args"},
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "something"}},
				Env:              []corev1.EnvVar{},
				Health:           korifiv1alpha1.Healthcheck{},
				Ports:            []int32{8080},
				Instances:        5,
				MemoryMB:         5,
				DiskMB:           100,
				CPUWeight:        2,
			},
		}
	})

	When("RunWorkload is created", func() {
		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, runWorkload)).To(Succeed())
		})

		It("creates the statefulset", func() {
			statefulset := new(appsv1.StatefulSet)
			statefulsetName := controllers.GetStatefulSetName(*runWorkload)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulsetName, Namespace: namespaceName}, statefulset)).To(Succeed())
			}).Should(Succeed())
		})
		It("the created statefulset contains an owner reference to our runworkload", func() {
			statefulset := new(appsv1.StatefulSet)
			statefulsetName := controllers.GetStatefulSetName(*runWorkload)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulsetName, Namespace: namespaceName}, statefulset)).To(Succeed())
			}).Should(Succeed())

			Expect(statefulset.OwnerReferences).To(HaveLen(1))
			Expect(statefulset.OwnerReferences[0].Kind).To(Equal("RunWorkload"))
			Expect(statefulset.OwnerReferences[0].Name).To(Equal(runWorkload.Name))
		})

		When("some external process is updating the StatefulSet", func() {
			JustBeforeEach(func() {
				statefulset := new(appsv1.StatefulSet)
				statefulsetName := controllers.GetStatefulSetName(*runWorkload)
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulsetName, Namespace: namespaceName}, statefulset)).To(Succeed())
				}).Should(Succeed())

				statefulset.Status.ReadyReplicas = runWorkload.Spec.Instances
				statefulset.Status.Replicas = runWorkload.Spec.Instances
				Expect(k8sClient.Status().Update(ctx, statefulset)).To(Succeed())
			})

			It("eventually updates status.readyReplicas to the same value as the desired number of instances", func() {
				actualRunWorkload := new(korifiv1alpha1.RunWorkload)
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: runWorkload.Name, Namespace: namespaceName}, actualRunWorkload)).To(Succeed())
					g.Expect(actualRunWorkload.Status.ReadyReplicas).To(Equal(runWorkload.Spec.Instances))
				}).Should(Succeed())
			})
		})
	})

	When("RunWorkload update", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, runWorkload)).To(Succeed())
			statefulset := new(appsv1.StatefulSet)
			statefulsetName := controllers.GetStatefulSetName(*runWorkload)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulsetName, Namespace: namespaceName}, statefulset)).To(Succeed())
			}).Should(Succeed())

			runWorkload.Spec.Instances = 2
			runWorkload.Spec.MemoryMB = 10
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Update(ctx, runWorkload)).To(Succeed())
		})

		It("updates the StatefulSet", func() {
			Eventually(func(g Gomega) {
				statefulSet := new(appsv1.StatefulSet)
				statefulSetName := controllers.GetStatefulSetName(*runWorkload)
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulSetName, Namespace: namespaceName}, statefulSet)).To(Succeed())
				g.Expect(*statefulSet.Spec.Replicas).To(Equal(int32(2)))
				g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String()).To(Equal("10Mi"))
			}).Should(Succeed())

		})
	})
})
