package controllers_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("AppWorkload to StatefulSet Converter", func() {
	var (
		statefulSet *appsv1.StatefulSet
		appWorkload *korifiv1alpha1.AppWorkload
		converter   *controllers.AppWorkloadToStatefulsetConverter
	)

	BeforeEach(func() {
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		appWorkload = createAppWorkload("some-namespace", "guid_1234")
		converter = controllers.NewAppWorkloadToStatefulsetConverter(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)), scheme.Scheme)
	})

	JustBeforeEach(func() {
		var err error
		statefulSet, err = converter.Convert(appWorkload)

		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("Statefulset Annotations",
		func(annotationName, expectedValue string) {
			Expect(statefulSet.Annotations).To(HaveKeyWithValue(annotationName, expectedValue))
		},
		Entry("ProcessGUID", controllers.AnnotationProcessGUID, "guid_1234-version_1234"),
		Entry("AppID", controllers.AnnotationAppID, "premium_app_guid_1234"),
		Entry("Version", controllers.AnnotationVersion, "version_1234"),
	)

	DescribeTable("Statefulset Template Annotations",
		func(annotationName, expectedValue string) {
			Expect(statefulSet.Spec.Template.Annotations).To(HaveKeyWithValue(annotationName, expectedValue))
		},
		Entry("ProcessGUID", controllers.AnnotationProcessGUID, "guid_1234-version_1234"),
		Entry("AppID", controllers.AnnotationAppID, "premium_app_guid_1234"),
		Entry("Version", controllers.AnnotationVersion, "version_1234"),
	)

	It("should be owned by the AppWorkload", func() {
		Expect(statefulSet.OwnerReferences).To(HaveLen(1))
		Expect(statefulSet.OwnerReferences[0].Kind).To(Equal("AppWorkload"))
	})

	It("should base the name and namspace on the appworkload", func() {
		Expect(statefulSet.Namespace).To(Equal(appWorkload.Namespace))
		Expect(statefulSet.Name).To(ContainSubstring("premium-app-guid-1234"))
	})

	It("should set podManagementPolicy to parallel", func() {
		Expect(string(statefulSet.Spec.PodManagementPolicy)).To(Equal("Parallel"))
	})

	It("should deny privilegeEscalation", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation).NotTo(BeNil())
		Expect(*statefulSet.Spec.Template.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation).To(Equal(false))
	})

	It("should drop all capabilities", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].SecurityContext.Capabilities).NotTo(BeNil())
		Expect(*statefulSet.Spec.Template.Spec.Containers[0].SecurityContext.Capabilities).To(Equal(corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		}))
	})

	It("should set the seccomp profile", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].SecurityContext.SeccompProfile).NotTo(BeNil())
		Expect(*statefulSet.Spec.Template.Spec.Containers[0].SecurityContext.SeccompProfile).To(Equal(corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}))
	})

	It("should set the startup probe", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].StartupProbe).To(Equal(appWorkload.Spec.StartupProbe))
	})

	It("should set the liveness probe", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].LivenessProbe).To(Equal(appWorkload.Spec.LivenessProbe))
	})

	It("should not automount service account token", func() {
		Expect(statefulSet.Spec.Template.Spec.AutomountServiceAccountToken).To(Equal(tools.PtrTo(false)))
	})

	It("should set the image", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal(appWorkload.Spec.Image))
	})

	It("should copy the image pull secrets", func() {
		Expect(statefulSet.Spec.Template.Spec.ImagePullSecrets).To(ContainElements(appWorkload.Spec.ImagePullSecrets))
	})

	It("should set the command", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].Command).To(ContainElements(appWorkload.Spec.Command))
	})

	It("should set imagePullPolicy to Always", func() {
		Expect(string(statefulSet.Spec.Template.Spec.Containers[0].ImagePullPolicy)).To(Equal("Always"))
	})

	It("should set app_guid as a label", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(controllers.LabelAppGUID, "premium_app_guid_1234"))
		Expect(statefulSet.Spec.Template.Labels).To(HaveKeyWithValue(controllers.LabelAppGUID, "premium_app_guid_1234"))
	})

	It("should set appworkload guid as a label on the statefulset only", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(controllers.LabelAppWorkloadGUID, "guid_1234"))
	})

	It("should set process_type as a label", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(controllers.LabelProcessType, "worker"))
		Expect(statefulSet.Spec.Template.Labels).To(HaveKeyWithValue(controllers.LabelProcessType, "worker"))
	})

	It("should set guid as a label", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(controllers.LabelGUID, "guid_1234"))
		Expect(statefulSet.Spec.Template.Labels).To(HaveKeyWithValue(controllers.LabelGUID, "guid_1234"))
	})

	It("should set version as a label", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(controllers.LabelVersion, "version_1234"))
		Expect(statefulSet.Spec.Template.Labels).To(HaveKeyWithValue(controllers.LabelVersion, "version_1234"))
	})

	It("should set statefulset-runner-index as a label", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(controllers.LabelStatefulSetRunnerIndex, "true"))
		Expect(statefulSet.Spec.Template.Labels).To(HaveKeyWithValue(controllers.LabelStatefulSetRunnerIndex, "true"))
	})

	It("should set guid as a label selector", func() {
		Expect(statefulSet.Spec.Selector.MatchLabels).To(HaveKeyWithValue(controllers.LabelGUID, "guid_1234"))
	})

	It("should set version as a label selector", func() {
		Expect(statefulSet.Spec.Selector.MatchLabels).To(HaveKeyWithValue(controllers.LabelVersion, "version_1234"))
	})

	It("should set memory limit", func() {
		actualLimit := statefulSet.Spec.Template.Spec.Containers[0].Resources.Limits.Memory()
		Expect(actualLimit.String()).To(Equal("1Gi"))
	})

	It("should set memory request", func() {
		actualRequest := statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests.Memory()
		Expect(actualRequest.String()).To(Equal("1Gi"))
	})

	It("should set cpu request", func() {
		expectedRequest := resource.NewScaledQuantity(5, resource.Milli)
		actualRequest := statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu()
		Expect(actualRequest.String()).To(Equal(expectedRequest.String()))
	})

	It("should not set cpu limit", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().IsZero()).To(BeTrue())
	})

	It("should set disk limit", func() {
		actualLimit := statefulSet.Spec.Template.Spec.Containers[0].Resources.Limits.StorageEphemeral()
		Expect(actualLimit.String()).To(Equal("2Gi"))
	})

	It("should run it with non-root user", func() {
		Expect(statefulSet.Spec.Template.Spec.SecurityContext).NotTo(BeNil())
		Expect(statefulSet.Spec.Template.Spec.SecurityContext.RunAsNonRoot).NotTo(BeNil())
		Expect(*statefulSet.Spec.Template.Spec.SecurityContext.RunAsNonRoot).To(Equal(true))
	})

	It("should set soft inter-pod anti-affinity", func() {
		podAntiAffinity := statefulSet.Spec.Template.Spec.Affinity.PodAntiAffinity
		Expect(podAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution).To(BeEmpty())
		Expect(podAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))

		weightedTerm := podAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0]
		Expect(weightedTerm.Weight).To(Equal(int32(100)))
		Expect(weightedTerm.PodAffinityTerm.TopologyKey).To(Equal("kubernetes.io/hostname"))
		Expect(weightedTerm.PodAffinityTerm.LabelSelector.MatchExpressions).To(ConsistOf(
			metav1.LabelSelectorRequirement{
				Key:      controllers.LabelGUID,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"guid_1234"},
			},
			metav1.LabelSelectorRequirement{
				Key:      controllers.LabelVersion,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"version_1234"},
			},
		))
	})

	It("should set the container environment variables", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1))
		container := statefulSet.Spec.Template.Spec.Containers[0]
		Expect(container.Env).To(ContainElements(
			corev1.EnvVar{Name: controllers.EnvPodName, ValueFrom: expectedValFrom("metadata.name")},
			corev1.EnvVar{Name: controllers.EnvCFInstanceGUID, ValueFrom: expectedValFrom("metadata.uid")},
			corev1.EnvVar{Name: controllers.EnvCFInstanceInternalIP, ValueFrom: expectedValFrom("status.podIP")},
			corev1.EnvVar{Name: controllers.EnvCFInstanceIP, ValueFrom: expectedValFrom("status.hostIP")},
		))
	})

	It("should set the container ports", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1))
		container := statefulSet.Spec.Template.Spec.Containers[0]
		Expect(container.Ports).To(HaveLen(2))
		Expect(container.Ports).To(ContainElements(corev1.ContainerPort{ContainerPort: 8888}, corev1.ContainerPort{ContainerPort: 9999}))
	})

	It("should set the serviceAccountName", func() {
		Expect(statefulSet.Spec.Template.Spec.ServiceAccountName).To(Equal("korifi-app"))
	})

	When("the app has environment set", func() {
		BeforeEach(func() {
			appWorkload.Spec.Env = []corev1.EnvVar{
				{
					Name: "bobs",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "your",
							},
							Key: "uncle",
						},
					},
				},
			}
		})

		It("is included in the stateful set env vars", func() {
			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := statefulSet.Spec.Template.Spec.Containers[0]
			Expect(container.Env).To(ContainElements(
				corev1.EnvVar{Name: controllers.EnvPodName, ValueFrom: expectedValFrom("metadata.name")},
				corev1.EnvVar{Name: controllers.EnvCFInstanceGUID, ValueFrom: expectedValFrom("metadata.uid")},
				corev1.EnvVar{Name: controllers.EnvCFInstanceInternalIP, ValueFrom: expectedValFrom("status.podIP")},
				corev1.EnvVar{Name: controllers.EnvCFInstanceIP, ValueFrom: expectedValFrom("status.hostIP")},
				corev1.EnvVar{Name: "bobs", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "your"},
						Key:                  "uncle",
					},
				}},
			))
		})
	})
})
