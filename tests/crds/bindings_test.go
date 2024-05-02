package crds_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = FDescribe("Bindings", func() {
	var (
		namespace        *corev1.Namespace
		deployment       *appsv1.Deployment
		cfServiceBinding *korifiv1alpha1.CFServiceBinding
		sbServiceBinding *servicebindingv1beta1.ServiceBinding
	)

	BeforeEach(func() {
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: uuid.NewString(),
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace.Name,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: tools.PtrTo[int32](1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "nginx",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "nginx",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "nginx",
							Image: "nginx",
							Ports: []corev1.ContainerPort{{
								ContainerPort: 80,
							}},
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

		cfServiceBinding = &korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace.Name,
			},
		}
		Expect(k8sClient.Create(ctx, cfServiceBinding)).To(Succeed())

		sbServiceBinding = &servicebindingv1beta1.ServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace.Name,
			},
			Spec: servicebindingv1beta1.ServiceBindingSpec{
				Type: "user-provided",
				Workload: servicebindingv1beta1.ServiceBindingWorkloadReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       deployment.Name,
				},
				Service: servicebindingv1beta1.ServiceBindingServiceReference{
					APIVersion: "korifi.cloudfoundry.org/v1alpha1",
					Kind:       "CFServiceBinding",
					Name:       cfServiceBinding.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, sbServiceBinding)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	When("the service duck type is set", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace.Name,
				},
				Type: corev1.SecretType("servicebinding.io/user-provided"),
				StringData: map[string]string{
					"foo": "bar",
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
				g.Expect(sbServiceBinding.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal("Ready")),
					HasStatus(Equal(metav1.ConditionUnknown)),
				)))
			}).Should(Succeed())
			Consistently(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
				g.Expect(sbServiceBinding.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal("Ready")),
					HasStatus(Equal(metav1.ConditionUnknown)),
				)))
			}).Should(Succeed())

			Expect(k8s.Patch(ctx, k8sClient, cfServiceBinding, func() {
				cfServiceBinding.Status.Binding.Name = secret.Name
			})).To(Succeed())
		})

		It("becomes ready", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
				g.Expect(sbServiceBinding.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal("Ready")),
					HasStatus(Equal(metav1.ConditionTrue)),
				)))
			}).Should(Succeed())
			Consistently(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
				g.Expect(sbServiceBinding.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal("Ready")),
					HasStatus(Equal(metav1.ConditionTrue)),
				)))
			}).Should(Succeed())
		})
	})
})
