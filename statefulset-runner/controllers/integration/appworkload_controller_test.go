package integration_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AppWorkloadsController", func() {
	var (
		ctx           context.Context
		appWorkload   *korifiv1alpha1.AppWorkload
		namespaceName string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespaceName = prefixedGUID("ns")
		createNamespace(ctx, k8sClient, namespaceName)

		appWorkload = &korifiv1alpha1.AppWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prefixedGUID("rw"),
				Namespace: namespaceName,
			},
			Spec: korifiv1alpha1.AppWorkloadSpec{
				GUID:    prefixedGUID("process"),
				Version: "1",
				AppGUID: "my-app",

				ProcessType:      "web",
				Image:            "my-image",
				Command:          []string{"do-it", "with", "args"},
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "something"}},
				Env:              []corev1.EnvVar{},
				LivenessProbe:    &corev1.Probe{},
				ReadinessProbe:   &corev1.Probe{},
				Ports:            []int32{8080},
				Instances:        5,
				RunnerName:       "statefulset-runner",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceEphemeralStorage: resource.MustParse("100Mi"),
						corev1.ResourceMemory:           resource.MustParse("5Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("5m"),
						corev1.ResourceMemory: resource.MustParse("5Mi"),
					},
				},
			},
		}
	})

	When("AppWorkload is created", func() {
		var statefulsetName string
		var err error

		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, appWorkload)).To(Succeed())
			statefulsetName, err = controllers.GetStatefulSetName(*appWorkload)
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates the statefulset", func() {
			statefulset := new(appsv1.StatefulSet)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulsetName, Namespace: namespaceName}, statefulset)).To(Succeed())
			}).Should(Succeed())
		})
		It("the created statefulset contains an owner reference to our appworkload", func() {
			statefulset := new(appsv1.StatefulSet)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulsetName, Namespace: namespaceName}, statefulset)).To(Succeed())
			}).Should(Succeed())

			Expect(statefulset.OwnerReferences).To(HaveLen(1))
			Expect(statefulset.OwnerReferences[0].Kind).To(Equal("AppWorkload"))
			Expect(statefulset.OwnerReferences[0].Name).To(Equal(appWorkload.Name))
		})

		It("creates the pod disruption budget", func() {
			pdb := new(policyv1.PodDisruptionBudget)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulsetName, Namespace: namespaceName}, pdb)).To(Succeed())
			}).Should(Succeed())
			Expect(*pdb.Spec.MinAvailable).To(Equal(intstr.FromString("50%")))
		})

		When("some external process is updating the StatefulSet", func() {
			JustBeforeEach(func() {
				statefulset := new(appsv1.StatefulSet)
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulsetName, Namespace: namespaceName}, statefulset)).To(Succeed())
				}).Should(Succeed())

				ogStatefulset := statefulset.DeepCopy()
				statefulset.Status.ReadyReplicas = appWorkload.Spec.Instances
				statefulset.Status.Replicas = appWorkload.Spec.Instances
				Expect(k8sClient.Status().Patch(ctx, statefulset, client.MergeFrom(ogStatefulset))).To(Succeed())
			})
		})
	})

	When("AppWorkload update", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, appWorkload)).To(Succeed())
			statefulset := new(appsv1.StatefulSet)
			statefulsetName, err := controllers.GetStatefulSetName(*appWorkload)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulsetName, Namespace: namespaceName}, statefulset)).To(Succeed())
			}).Should(Succeed())
		})

		JustBeforeEach(func() {
			actualAppWorkload := new(korifiv1alpha1.AppWorkload)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: appWorkload.Name, Namespace: namespaceName}, actualAppWorkload)).To(Succeed())

				actualAppWorkload.Spec.Instances = 2
				actualAppWorkload.Spec.Resources.Requests[corev1.ResourceCPU] = resource.MustParse("1024m")
				actualAppWorkload.Spec.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("10Mi")
				actualAppWorkload.Spec.Resources.Limits[corev1.ResourceMemory] = resource.MustParse("10Mi")
				g.Expect(k8sClient.Update(ctx, actualAppWorkload)).To(Succeed())
			}).Should(Succeed())
		})

		It("updates the StatefulSet", func() {
			Eventually(func(g Gomega) {
				statefulSet := new(appsv1.StatefulSet)
				statefulSetName, err := controllers.GetStatefulSetName(*appWorkload)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: statefulSetName, Namespace: namespaceName}, statefulSet)).To(Succeed())
				g.Expect(*statefulSet.Spec.Replicas).To(Equal(int32(2)))
				g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String()).To(Equal("10Mi"))
				g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String()).To(Equal("1024m"))
				g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().IsZero()).To(BeTrue())
			}).Should(Succeed())
		})
	})
})
