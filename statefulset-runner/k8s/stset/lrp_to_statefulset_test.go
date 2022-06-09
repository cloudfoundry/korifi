package stset_test

import (
	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset/stsetfakes"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("LRP to StatefulSet Converter", func() {
	var (
		allowAutomountServiceAccountToken bool
		allowRunImageAsRoot               bool
		livenessProbeCreator              *stsetfakes.FakeProbeCreator
		readinessProbeCreator             *stsetfakes.FakeProbeCreator
		lrp                               *eiriniv1.LRP
		statefulSet                       *appsv1.StatefulSet
		privateRegistrySecret             *corev1.Secret
		livenessProbe                     *corev1.Probe
		readinessProbe                    *corev1.Probe
	)

	BeforeEach(func() {
		allowAutomountServiceAccountToken = false
		allowRunImageAsRoot = false
		livenessProbeCreator = new(stsetfakes.FakeProbeCreator)
		readinessProbeCreator = new(stsetfakes.FakeProbeCreator)
		lrp = createLRP("the-namespace", "Baldur")
		privateRegistrySecret = nil

		livenessProbe = &corev1.Probe{}
		livenessProbeCreator.Returns(livenessProbe)

		readinessProbe = &corev1.Probe{}
		readinessProbeCreator.Returns(readinessProbe)
	})

	JustBeforeEach(func() {
		converter := stset.NewLRPToStatefulSetConverter("eirini", "secret-name", allowAutomountServiceAccountToken, allowRunImageAsRoot, livenessProbeCreator.Spy, readinessProbeCreator.Spy)

		var err error
		statefulSet, err = converter.Convert("Baldur", lrp, privateRegistrySecret)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should create a healthcheck probe", func() {
		Expect(livenessProbeCreator.CallCount()).To(Equal(1))
	})

	It("should create a readiness probe", func() {
		Expect(readinessProbeCreator.CallCount()).To(Equal(1))
	})

	DescribeTable("Statefulset Annotations",
		func(annotationName, expectedValue string) {
			Expect(statefulSet.Annotations).To(HaveKeyWithValue(annotationName, expectedValue))
		},
		Entry("ProcessGUID", stset.AnnotationProcessGUID, "guid_1234-version_1234"),
		Entry("AppName", stset.AnnotationAppName, "Baldur"),
		Entry("AppID", stset.AnnotationAppID, "premium_app_guid_1234"),
		Entry("Version", stset.AnnotationVersion, "version_1234"),
		Entry("SpaceName", stset.AnnotationSpaceName, "space-foo"),
		Entry("SpaceGUID", stset.AnnotationSpaceGUID, "space-guid"),
		Entry("OrgName", stset.AnnotationOrgName, "org-foo"),
		Entry("OrgGUID", stset.AnnotationOrgGUID, "org-guid"),
	)

	DescribeTable("Statefulset Template Annotations",
		func(annotationName, expectedValue string) {
			Expect(statefulSet.Spec.Template.Annotations).To(HaveKeyWithValue(annotationName, expectedValue))
		},
		Entry("ProcessGUID", stset.AnnotationProcessGUID, "guid_1234-version_1234"),
		Entry("AppName", stset.AnnotationAppName, "Baldur"),
		Entry("AppID", stset.AnnotationAppID, "premium_app_guid_1234"),
		Entry("Version", stset.AnnotationVersion, "version_1234"),
		Entry("SpaceName", stset.AnnotationSpaceName, "space-foo"),
		Entry("SpaceGUID", stset.AnnotationSpaceGUID, "space-guid"),
		Entry("OrgName", stset.AnnotationOrgName, "org-foo"),
		Entry("OrgGUID", stset.AnnotationOrgGUID, "org-guid"),
	)

	It("should set seccomp pod annotation", func() {
		Expect(statefulSet.Spec.Template.Annotations[corev1.SeccompPodAnnotationKey]).To(Equal(corev1.SeccompProfileRuntimeDefault))
	})

	It("should set podManagementPolicy to parallel", func() {
		Expect(string(statefulSet.Spec.PodManagementPolicy)).To(Equal("Parallel"))
	})

	It("should set podImagePullSecret", func() {
		Expect(statefulSet.Spec.Template.Spec.ImagePullSecrets).To(HaveLen(1))
		secret := statefulSet.Spec.Template.Spec.ImagePullSecrets[0]
		Expect(secret.Name).To(Equal("secret-name"))
	})

	It("should deny privilegeEscalation", func() {
		Expect(*statefulSet.Spec.Template.Spec.Containers[0].SecurityContext.AllowPrivilegeEscalation).To(Equal(false))
	})

	It("should set the liveness probe", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].LivenessProbe).To(Equal(livenessProbe))
	})

	It("should set the readiness probe", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].ReadinessProbe).To(Equal(readinessProbe))
	})

	It("should not automount service account token", func() {
		f := false
		Expect(statefulSet.Spec.Template.Spec.AutomountServiceAccountToken).To(Equal(&f))
	})

	It("should set imagePullPolicy to Always", func() {
		Expect(string(statefulSet.Spec.Template.Spec.Containers[0].ImagePullPolicy)).To(Equal("Always"))
	})

	It("should set app_guid as a label", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(stset.LabelAppGUID, "premium_app_guid_1234"))
		Expect(statefulSet.Spec.Template.Labels).To(HaveKeyWithValue(stset.LabelAppGUID, "premium_app_guid_1234"))
	})

	It("should set process_type as a label", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(stset.LabelProcessType, "worker"))
		Expect(statefulSet.Spec.Template.Labels).To(HaveKeyWithValue(stset.LabelProcessType, "worker"))
	})

	It("should set guid as a label", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(stset.LabelGUID, "guid_1234"))
		Expect(statefulSet.Spec.Template.Labels).To(HaveKeyWithValue(stset.LabelGUID, "guid_1234"))
	})

	It("should set version as a label", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(stset.LabelVersion, "version_1234"))
		Expect(statefulSet.Spec.Template.Labels).To(HaveKeyWithValue(stset.LabelVersion, "version_1234"))
	})

	It("should set source_type as a label", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(stset.LabelSourceType, "APP"))
		Expect(statefulSet.Spec.Template.Labels).To(HaveKeyWithValue(stset.LabelSourceType, "APP"))
	})

	It("should set cf space and org information as labels", func() {
		haveSpaceAndOrgInformation := SatisfyAll(
			HaveKeyWithValue(stset.LabelOrgGUID, "org-guid"),
			HaveKeyWithValue(stset.LabelOrgName, "org-foo"),
			HaveKeyWithValue(stset.LabelSpaceGUID, "space-guid"),
			HaveKeyWithValue(stset.LabelSpaceName, "space-foo"),
		)
		Expect(statefulSet.Labels).To(haveSpaceAndOrgInformation)
		Expect(statefulSet.Spec.Template.Labels).To(haveSpaceAndOrgInformation)
	})

	It("should set guid as a label selector", func() {
		Expect(statefulSet.Spec.Selector.MatchLabels).To(HaveKeyWithValue(stset.LabelGUID, "guid_1234"))
	})

	It("should set version as a label selector", func() {
		Expect(statefulSet.Spec.Selector.MatchLabels).To(HaveKeyWithValue(stset.LabelVersion, "version_1234"))
	})

	It("should set source_type as a label selector", func() {
		Expect(statefulSet.Spec.Selector.MatchLabels).To(HaveKeyWithValue(stset.LabelSourceType, "APP"))
	})

	It("should set memory limit", func() {
		actualLimit := statefulSet.Spec.Template.Spec.Containers[0].Resources.Limits.Memory()
		Expect(actualLimit.String()).To(Equal("1Gi"))
	})

	It("should set memory request", func() {
		actualLimit := statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests.Memory()
		Expect(actualLimit.String()).To(Equal("1Gi"))
	})

	It("should set cpu request", func() {
		expectedLimit := resource.NewScaledQuantity(2, resource.Milli)
		actualLimit := statefulSet.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu()
		Expect(actualLimit).To(Equal(expectedLimit))
	})

	It("should set disk limit", func() {
		actualLimit := statefulSet.Spec.Template.Spec.Containers[0].Resources.Limits.StorageEphemeral()
		Expect(actualLimit.String()).To(Equal("2Gi"))
	})

	It("should set user defined annotations", func() {
		Expect(statefulSet.Spec.Template.Annotations["prometheus.io/scrape"]).To(Equal("secret-value"))
	})

	It("should run it with non-root user", func() {
		Expect(statefulSet.Spec.Template.Spec.SecurityContext.RunAsNonRoot).To(PointTo(Equal(true)))
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
				Key:      stset.LabelGUID,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"guid_1234"},
			},
			metav1.LabelSelectorRequirement{
				Key:      stset.LabelVersion,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"version_1234"},
			},
			metav1.LabelSelectorRequirement{
				Key:      stset.LabelSourceType,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"APP"},
			},
		))
	})

	It("should set application service account", func() {
		Expect(statefulSet.Spec.Template.Spec.ServiceAccountName).To(Equal("eirini"))
	})

	It("should set the container environment variables", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(1))
		container := statefulSet.Spec.Template.Spec.Containers[0]
		Expect(container.Env).To(ContainElements(
			corev1.EnvVar{Name: eirinictrl.EnvPodName, ValueFrom: expectedValFrom("metadata.name")},
			corev1.EnvVar{Name: eirinictrl.EnvCFInstanceGUID, ValueFrom: expectedValFrom("metadata.uid")},
			corev1.EnvVar{Name: eirinictrl.EnvCFInstanceInternalIP, ValueFrom: expectedValFrom("status.podIP")},
			corev1.EnvVar{Name: eirinictrl.EnvCFInstanceIP, ValueFrom: expectedValFrom("status.hostIP")},
		))
	})

	When("the app has environment set", func() {
		BeforeEach(func() {
			lrp.Spec.Environment = []corev1.EnvVar{
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
				corev1.EnvVar{Name: eirinictrl.EnvPodName, ValueFrom: expectedValFrom("metadata.name")},
				corev1.EnvVar{Name: eirinictrl.EnvCFInstanceGUID, ValueFrom: expectedValFrom("metadata.uid")},
				corev1.EnvVar{Name: eirinictrl.EnvCFInstanceInternalIP, ValueFrom: expectedValFrom("status.podIP")},
				corev1.EnvVar{Name: eirinictrl.EnvCFInstanceIP, ValueFrom: expectedValFrom("status.hostIP")},
				corev1.EnvVar{Name: "bobs", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "your"},
						Key:                  "uncle",
					},
				}},
			))
		})
	})

	When("the app has sidecars", func() {
		BeforeEach(func() {
			lrp.Spec.Sidecars = []eiriniv1.Sidecar{
				{
					Name:    "first-sidecar",
					Command: []string{"echo", "the first sidecar"},
					Env: map[string]string{
						"FOO": "BAR",
					},
					MemoryMB: 101,
				},
				{
					Name:    "second-sidecar",
					Command: []string{"echo", "the second sidecar"},
					Env: map[string]string{
						"FOO": "BAZ",
					},
					MemoryMB: 102,
				},
			}
		})

		It("should set the sidecar containers", func() {
			containers := statefulSet.Spec.Template.Spec.Containers
			Expect(containers).To(HaveLen(3))

			Expect(containers).To(ContainElements(
				corev1.Container{
					Name:    "first-sidecar",
					Image:   "gcr.io/foo/bar",
					Command: []string{"echo", "the first sidecar"},
					Env:     []corev1.EnvVar{{Name: "FOO", Value: "BAR"}},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory:           stset.MebibyteQuantity(101),
							corev1.ResourceEphemeralStorage: stset.MebibyteQuantity(lrp.Spec.DiskMB),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: stset.MebibyteQuantity(101),
							corev1.ResourceCPU:    *resource.NewScaledQuantity(int64(lrp.Spec.CPUWeight), resource.Milli),
						},
					},
				},
				corev1.Container{
					Name:    "second-sidecar",
					Image:   "gcr.io/foo/bar",
					Command: []string{"echo", "the second sidecar"},
					Env:     []corev1.EnvVar{{Name: "FOO", Value: "BAZ"}},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory:           stset.MebibyteQuantity(102),
							corev1.ResourceEphemeralStorage: stset.MebibyteQuantity(lrp.Spec.DiskMB),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: stset.MebibyteQuantity(102),
							corev1.ResourceCPU:    *resource.NewScaledQuantity(int64(lrp.Spec.CPUWeight), resource.Milli),
						},
					},
				},
			))
		})
	})

	When("automounting service account token is allowed", func() {
		BeforeEach(func() {
			allowAutomountServiceAccountToken = true
		})

		It("does not set automountServiceAccountToken", func() {
			Expect(statefulSet.Spec.Template.Spec.AutomountServiceAccountToken).To(BeNil())
		})
	})

	When("application should run as root", func() {
		BeforeEach(func() {
			allowRunImageAsRoot = true
		})

		It("does not set privileged context", func() {
			Expect(statefulSet.Spec.Template.Spec.SecurityContext).To(BeNil())
		})
	})

	When("the app references a private docker image", func() {
		BeforeEach(func() {
			lrp.Spec.PrivateRegistry = &eiriniv1.PrivateRegistry{
				Username: "user",
				Password: "password",
			}

			privateRegistrySecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "private-registry-secret",
				},
			}
		})

		It("should add the private repo secret to podImagePullSecret", func() {
			Expect(statefulSet.Spec.Template.Spec.ImagePullSecrets).To(HaveLen(2))
			secret := statefulSet.Spec.Template.Spec.ImagePullSecrets[1]
			Expect(secret.Name).To(Equal("private-registry-secret"))
		})
	})
})
