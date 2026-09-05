package appworkload_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/knative-runner/controllers"
	"code.cloudfoundry.org/korifi/knative-runner/controllers/appworkload"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("AppWorkload to Knative Service Converter", func() {
	var (
		ksvc        *unstructured.Unstructured
		appWorkload *korifiv1alpha1.AppWorkload
		converter   *appworkload.AppWorkloadToKnativeServiceConverter
	)

	BeforeEach(func() {
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		appWorkload = &korifiv1alpha1.AppWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "guid_1234",
				Namespace:  "some-namespace",
				Generation: 1,
				Annotations: map[string]string{
					korifiv1alpha1.CFAppLastStopRevisionKey: "lastStopAppRev",
				},
			},
			Spec: korifiv1alpha1.AppWorkloadSpec{
				AppGUID:          "premium_app_guid_1234",
				GUID:             "guid_1234",
				Version:          "version_1234",
				Image:            "gcr.io/foo/bar",
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "some-secret-name"}},
				Command: []string{
					"/bin/sh",
					"-c",
					"while true; do echo hello; sleep 10;done",
				},
				ProcessType: "web",
				Env:         []corev1.EnvVar{},
				StartupProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz",
							Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8080)},
						},
					},
					FailureThreshold: 30,
					PeriodSeconds:    2,
				},
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz",
							Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(8080)},
						},
					},
					PeriodSeconds:    30,
					FailureThreshold: 1,
				},
				Ports:      []int32{8080},
				Instances:  2,
				RunnerName: controllers.AppWorkloadReconcilerName,
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceEphemeralStorage: resource.MustParse("2048Mi"),
						corev1.ResourceMemory:           resource.MustParse("1024Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("5m"),
						corev1.ResourceMemory: resource.MustParse("1024Mi"),
					},
				},
			},
		}

		converter = appworkload.NewAppWorkloadToKnativeServiceConverter(scheme.Scheme)
	})

	JustBeforeEach(func() {
		var err error
		ksvc, err = converter.Convert(appWorkload)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Service identity", func() {
		It("creates a Knative Service GVK", func() {
			Expect(ksvc.GetAPIVersion()).To(Equal("serving.knative.dev/v1"))
			Expect(ksvc.GetKind()).To(Equal("Service"))
		})

		It("bases namespace on the AppWorkload and uses a DNS-1035 name", func() {
			Expect(ksvc.GetNamespace()).To(Equal(appWorkload.Namespace))
			Expect(ksvc.GetName()).To(MatchRegexp(`^[a-z]([-a-z0-9]*[a-z0-9])?$`))
			Expect(ksvc.GetName()).To(HavePrefix("kw-"))
		})

		It("keeps a stable name when version changes but lastStopAppRev is unchanged", func() {
			originalName := ksvc.GetName()
			appWorkload.Spec.Version = "another_version"
			renamed, err := converter.Convert(appWorkload)
			Expect(err).NotTo(HaveOccurred())
			Expect(renamed.GetName()).To(Equal(originalName))
		})

		It("renames when lastStopAppRev changes", func() {
			originalName := ksvc.GetName()
			appWorkload.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey] = "another_version"
			renamed, err := converter.Convert(appWorkload)
			Expect(err).NotTo(HaveOccurred())
			Expect(renamed.GetName()).NotTo(Equal(originalName))
		})

		It("defaults lastStopAppRev to Spec.Version when the annotation is absent", func() {
			originalName := ksvc.GetName()
			appWorkload.Spec.Version = appWorkload.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]
			delete(appWorkload.Annotations, korifiv1alpha1.CFAppLastStopRevisionKey)
			renamed, err := converter.Convert(appWorkload)
			Expect(err).NotTo(HaveOccurred())
			Expect(renamed.GetName()).To(Equal(originalName))
		})
	})

	Describe("Labels", func() {
		It("sets Korifi labels on the Service and revision template", func() {
			Expect(ksvc.GetLabels()).To(SatisfyAll(
				HaveKeyWithValue(controllers.LabelGUID, "guid_1234"),
				HaveKeyWithValue(appworkload.LabelAppGUID, "premium_app_guid_1234"),
				HaveKeyWithValue(appworkload.LabelAppWorkloadGUID, "guid_1234"),
				HaveKeyWithValue(appworkload.LabelProcessType, "web"),
				HaveKeyWithValue(appworkload.LabelVersion, "version_1234"),
				HaveKeyWithValue("apps.kubernetes.io/pod-index", "0"),
			))

			templateLabels, _, err := unstructured.NestedStringMap(ksvc.Object, "spec", "template", "metadata", "labels")
			Expect(err).NotTo(HaveOccurred())
			Expect(templateLabels).To(HaveKeyWithValue(appworkload.LabelAppWorkloadGUID, "guid_1234"))
		})
	})

	Describe("Scale annotations", func() {
		It("pins min-scale and max-scale to CF instances", func() {
			anns, _, err := unstructured.NestedStringMap(ksvc.Object, "spec", "template", "metadata", "annotations")
			Expect(err).NotTo(HaveOccurred())
			Expect(anns).To(HaveKeyWithValue(appworkload.AnnotationMinScale, "2"))
			Expect(anns).To(HaveKeyWithValue(appworkload.AnnotationMaxScale, "2"))
		})

		It("keeps min-scale 0 and max-scale 1 when instances is 0", func() {
			appWorkload.Spec.Instances = 0
			zeroed, err := converter.Convert(appWorkload)
			Expect(err).NotTo(HaveOccurred())
			anns, _, err := unstructured.NestedStringMap(zeroed.Object, "spec", "template", "metadata", "annotations")
			Expect(err).NotTo(HaveOccurred())
			Expect(anns).To(HaveKeyWithValue(appworkload.AnnotationMinScale, "0"))
			Expect(anns).To(HaveKeyWithValue(appworkload.AnnotationMaxScale, "1"))
		})
	})

	Describe("User container", func() {
		It("configures image, command, ports, and security context", func() {
			containers, found, err := unstructured.NestedSlice(ksvc.Object, "spec", "template", "spec", "containers")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(containers).To(HaveLen(1))

			container := containers[0].(map[string]any)
			Expect(container["name"]).To(Equal(appworkload.ApplicationContainerName))
			Expect(container["image"]).To(Equal(appWorkload.Spec.Image))
			Expect(container["imagePullPolicy"]).To(Equal("Always"))
			Expect(container["command"]).To(Equal([]any{"/bin/sh", "-c", "while true; do echo hello; sleep 10;done"}))

			ports := container["ports"].([]any)
			Expect(ports).To(ConsistOf(MatchKeys(IgnoreExtras, Keys{
				"name":          Equal("http1"),
				"containerPort": BeNumerically("==", 8080),
			})))

			sec := container["securityContext"].(map[string]any)
			Expect(sec["allowPrivilegeEscalation"]).To(BeFalse())
			Expect(sec["runAsNonRoot"]).To(BeTrue())
			Expect(sec["runAsUser"]).To(BeNumerically("==", 1000))
		})
	})

	Describe("Env", func() {
		It("sets CF_INSTANCE_INDEX to 0 and drops Knative-reserved env", func() {
			appWorkload.Spec.Env = []corev1.EnvVar{
				{Name: "PORT", Value: "8080"},
				{Name: "K_SERVICE", Value: "nope"},
				{Name: "CUSTOM", Value: "keep"},
				{
					Name: "FIELDREF",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
					},
				},
			}
			converted, err := converter.Convert(appWorkload)
			Expect(err).NotTo(HaveOccurred())

			containers, _, err := unstructured.NestedSlice(converted.Object, "spec", "template", "spec", "containers")
			Expect(err).NotTo(HaveOccurred())
			env := containers[0].(map[string]any)["env"].([]any)
			names := []string{}
			for _, e := range env {
				names = append(names, e.(map[string]any)["name"].(string))
			}
			Expect(names).To(ContainElements("CUSTOM", appworkload.EnvCFInstanceIndex))
			Expect(names).NotTo(ContainElement("PORT"))
			Expect(names).NotTo(ContainElement("K_SERVICE"))
			Expect(names).NotTo(ContainElement("FIELDREF"))

			Expect(env).To(ContainElement(MatchKeys(IgnoreExtras, Keys{
				"name":  Equal(appworkload.EnvCFInstanceIndex),
				"value": Equal("0"),
			})))
		})

		It("sorts env vars for stable hashes", func() {
			appWorkload.Spec.Env = []corev1.EnvVar{
				{Name: "b-second", Value: "second"},
				{Name: "c-third", Value: "third"},
				{Name: "a-first", Value: "first"},
			}
			converted, err := converter.Convert(appWorkload)
			Expect(err).NotTo(HaveOccurred())
			containers, _, err := unstructured.NestedSlice(converted.Object, "spec", "template", "spec", "containers")
			Expect(err).NotTo(HaveOccurred())
			env := containers[0].(map[string]any)["env"].([]any)
			Expect(env[0].(map[string]any)["name"]).To(Equal("CF_INSTANCE_INDEX"))
			Expect(env[1].(map[string]any)["name"]).To(Equal("a-first"))
			Expect(env[2].(map[string]any)["name"]).To(Equal("b-second"))
			Expect(env[3].(map[string]any)["name"]).To(Equal("c-third"))
		})
	})

	Describe("Pod spec", func() {
		It("does not set a pod-level securityContext", func() {
			_, found, err := unstructured.NestedFieldNoCopy(ksvc.Object, "spec", "template", "spec", "securityContext")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse())
		})

		It("sets the service account and disables automount", func() {
			sa, _, err := unstructured.NestedString(ksvc.Object, "spec", "template", "spec", "serviceAccountName")
			Expect(err).NotTo(HaveOccurred())
			Expect(sa).To(Equal(appworkload.ServiceAccountName))

			automount, found, err := unstructured.NestedBool(ksvc.Object, "spec", "template", "spec", "automountServiceAccountToken")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(automount).To(BeFalse())
		})

		It("copies image pull secrets", func() {
			secrets, _, err := unstructured.NestedSlice(ksvc.Object, "spec", "template", "spec", "imagePullSecrets")
			Expect(err).NotTo(HaveOccurred())
			Expect(secrets).To(ConsistOf(MatchKeys(IgnoreExtras, Keys{
				"name": Equal("some-secret-name"),
			})))
		})
	})

	Describe("Service bindings", func() {
		When("the app workload has services", func() {
			BeforeEach(func() {
				appWorkload.Spec.Services = []korifiv1alpha1.ServiceBinding{{
					Secret: "service-secret",
					Name:   "binding-name",
				}}
			})

			It("sets SERVICE_BINDING_ROOT and mounts the secret", func() {
				containers, _, err := unstructured.NestedSlice(ksvc.Object, "spec", "template", "spec", "containers")
				Expect(err).NotTo(HaveOccurred())
				env := containers[0].(map[string]any)["env"].([]any)
				Expect(env).To(ContainElement(MatchKeys(IgnoreExtras, Keys{
					"name":  Equal(appworkload.EnvServiceBindingRoot),
					"value": Equal("/bindings"),
				})))

				mounts := containers[0].(map[string]any)["volumeMounts"].([]any)
				Expect(mounts).To(ConsistOf(MatchKeys(IgnoreExtras, Keys{
					"name":      Equal("binding-name"),
					"readOnly":  BeTrue(),
					"mountPath": Equal("/bindings/binding-name"),
				})))

				volumes, _, err := unstructured.NestedSlice(ksvc.Object, "spec", "template", "spec", "volumes")
				Expect(err).NotTo(HaveOccurred())
				Expect(volumes).To(HaveLen(1))
				Expect(volumes[0].(map[string]any)["name"]).To(Equal("binding-name"))
				Expect(volumes[0].(map[string]any)["secret"]).To(MatchKeys(IgnoreExtras, Keys{
					"secretName": Equal("service-secret"),
				}))
			})
		})
	})

	Describe("Stability", func() {
		It("produces a stable knative service regardless of map iteration order", func() {
			for i := 0; i < 50; i++ {
				again, err := converter.Convert(appWorkload)
				Expect(err).NotTo(HaveOccurred())
				Expect(again.Object).To(Equal(ksvc.Object), "iteration %d", i)
			}
		})
	})
})
