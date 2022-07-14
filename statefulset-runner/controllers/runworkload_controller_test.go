package controllers_test

import (
	"context"
	"errors"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/statefulset-runner/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	testRunworkloadGUID = "test-runworkload-guid"
	testNamespace       = "test-ns"
)

var _ = Describe("RunWorkload to StatefulSet Converter", func() {
	var (
		statefulSet *appsv1.StatefulSet
		runWorkload *korifiv1alpha1.RunWorkload
		reconciler  *controllers.RunWorkloadReconciler
	)

	BeforeEach(func() {
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		runWorkload = createRunWorkload("some-namespace", "guid_1234")
		reconciler = controllers.NewRunWorkloadReconciler(nil, scheme.Scheme, zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	})

	JustBeforeEach(func() {
		var err error
		statefulSet, err = reconciler.Convert(*runWorkload)

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

	It("should be owned by the RunWorkload", func() {
		Expect(statefulSet.OwnerReferences).To(HaveLen(1))
		Expect(statefulSet.OwnerReferences[0].Kind).To(Equal("RunWorkload"))
	})

	It("should base the name and namspace on the runworkload", func() {
		Expect(statefulSet.Namespace).To(Equal(runWorkload.Namespace))
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

	It("should set the liveness probe", func() {
		probe := statefulSet.Spec.Template.Spec.Containers[0].LivenessProbe
		Expect(probe).NotTo(BeNil())
		Expect(probe.ProbeHandler.HTTPGet).NotTo(BeNil())
		Expect(probe.ProbeHandler.HTTPGet.Path).To(Equal("/healthz"))
		Expect(probe.ProbeHandler.HTTPGet.Port.IntValue()).To(Equal(8080))
	})

	It("should set the readiness probe", func() {
		probe := statefulSet.Spec.Template.Spec.Containers[0].ReadinessProbe
		Expect(probe).NotTo(BeNil())
		Expect(probe.ProbeHandler.HTTPGet).NotTo(BeNil())
		Expect(probe.ProbeHandler.HTTPGet.Path).To(Equal("/healthz"))
		Expect(probe.ProbeHandler.HTTPGet.Port.IntValue()).To(Equal(8080))
	})

	It("should not automount service account token", func() {
		f := false
		Expect(statefulSet.Spec.Template.Spec.AutomountServiceAccountToken).To(Equal(&f))
	})

	It("should set the image", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal(runWorkload.Spec.Image))
	})

	It("copy the image pull secrets", func() {
		Expect(statefulSet.Spec.Template.Spec.ImagePullSecrets).To(ContainElements(runWorkload.Spec.ImagePullSecrets))
	})

	It("should set the command", func() {
		Expect(statefulSet.Spec.Template.Spec.Containers[0].Command).To(ContainElements(runWorkload.Spec.Command))
	})

	It("should set imagePullPolicy to Always", func() {
		Expect(string(statefulSet.Spec.Template.Spec.Containers[0].ImagePullPolicy)).To(Equal("Always"))
	})

	It("should set app_guid as a label", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(controllers.LabelAppGUID, "premium_app_guid_1234"))
		Expect(statefulSet.Spec.Template.Labels).To(HaveKeyWithValue(controllers.LabelAppGUID, "premium_app_guid_1234"))
	})

	It("should set runworkload guid as a label on the statefulset only", func() {
		Expect(statefulSet.Labels).To(HaveKeyWithValue(controllers.LabelRunWorkloadGUID, "guid_1234"))
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
		Expect(actualRequest).To(Equal(expectedRequest))
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

	When("the app has environment set", func() {
		BeforeEach(func() {
			runWorkload.Spec.Env = []corev1.EnvVar{
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

var _ = Describe("RunWorkload Reconcile", func() {
	var (
		fakeClient                   *fake.Client
		fakeStatusWriter             *fake.StatusWriter
		reconciler                   *controllers.RunWorkloadReconciler
		reconcileResult              ctrl.Result
		reconcileErr                 error
		ctx                          context.Context
		req                          ctrl.Request
		runworkload                  *korifiv1alpha1.RunWorkload
		statefulSet                  *v1.StatefulSet
		getRunworkloadError          error
		getStatefulSetError          error
		createStatefulSetError       error
		updateRunworkloadStatusError error
	)

	BeforeEach(func() {
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		runworkload = createRunWorkload("some-namespace", "guid_1234")
		statefulSet = &v1.StatefulSet{}

		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      testRunworkloadGUID,
				Namespace: testNamespace,
			},
		}

		getRunworkloadError = nil
		getStatefulSetError = apierrors.NewNotFound(schema.GroupResource{
			Group:    "v1",
			Resource: "StatefulSet",
		}, "some-resource")
		createStatefulSetError = nil
		updateRunworkloadStatusError = nil

		fakeClient = new(fake.Client)
		fakeClient.GetStub = func(_ context.Context, _ types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.RunWorkload:
				runworkload.DeepCopyInto(obj)
				return getRunworkloadError
			case *v1.StatefulSet:
				if getStatefulSetError == nil {
					statefulSet.DeepCopyInto(obj)
				}
				return getStatefulSetError
			default:
				panic("TestClient Get provided an unexpected object type")
			}
		}

		fakeClient.CreateStub = func(ctx context.Context, obj client.Object, option ...client.CreateOption) error {
			switch obj.(type) {
			case *v1.StatefulSet:
				return createStatefulSetError
			default:
				panic("TestClient Create provided an unexpected object type")
			}
		}

		fakeStatusWriter = &fake.StatusWriter{}
		fakeClient.StatusReturns(fakeStatusWriter)

		fakeStatusWriter.UpdateStub = func(ctx context.Context, obj client.Object, option ...client.UpdateOption) error {
			return updateRunworkloadStatusError
		}

		reconciler = controllers.NewRunWorkloadReconciler(fakeClient, scheme.Scheme, zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = reconciler.Reconcile(ctx, req)
	})

	When("the Runworkload is being created", func() {
		It("returns an empty result and does not return error", func() {
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
			Expect(reconcileErr).NotTo(HaveOccurred())
		})

		It("creates a StatefulSet", func() {
			Expect(fakeClient.CreateCallCount()).To(Equal(1), "Client.Create call count mismatch")
			_, obj, _ := fakeClient.CreateArgsForCall(0)
			Expect(obj).To(BeAssignableToTypeOf(new(v1.StatefulSet)))
		})

		When("creating the StatefulSet fails", func() {
			BeforeEach(func() {
				createStatefulSetError = errors.New("big sad")
			})

			It("returns an error", func() {
				Expect(reconcileErr).To(MatchError("big sad"))
			})
		})
	})

	When("the Runworkload is being deleted", func() {
		BeforeEach(func() {
			getRunworkloadError = apierrors.NewNotFound(schema.GroupResource{
				Group:    "v1alpha1",
				Resource: "RunWorkload",
			}, "some-resource")
		})

		It("returns an empty result and does not return error", func() {
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
			Expect(reconcileErr).NotTo(HaveOccurred())
		})
	})

	When("the Runworkload is being updated", func() {
		BeforeEach(func() {
			//nolint
			var replicas int32
			replicas = 1

			runworkload = &korifiv1alpha1.RunWorkload{
				TypeMeta: metav1.TypeMeta{
					Kind:       "",
					APIVersion: "",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sts",
					Namespace: testNamespace,
				},
				Spec: korifiv1alpha1.RunWorkloadSpec{
					GUID:                 "test-sts",
					Version:              "1",
					Instances:            2,
					MemoryMiB:            10,
					DiskMiB:              10,
					CPURequestMillicores: 10,
					CPULimitMillicores:   20,
				},
			}

			getStatefulSetError = nil
			statefulSet = &v1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sts-66a71561d2",
					Namespace: testNamespace,
				},
				Spec: v1.StatefulSetSpec{
					Replicas: &replicas,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-app",
									Image: "test-image",
									Resources: corev1.ResourceRequirements{
										Limits: map[corev1.ResourceName]resource.Quantity{
											corev1.ResourceMemory:           controllers.MebibyteQuantity(512),
											corev1.ResourceEphemeralStorage: controllers.MebibyteQuantity(512),
											corev1.ResourceCPU:              *resource.NewScaledQuantity(20, resource.Milli),
										},
										Requests: map[corev1.ResourceName]resource.Quantity{
											corev1.ResourceMemory: controllers.MebibyteQuantity(512),
											corev1.ResourceCPU:    *resource.NewScaledQuantity(10, resource.Milli),
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("scales instances", func() {
			Expect(fakeClient.PatchCallCount()).To(Equal(1))
		})
	})
})

func expectedValFrom(fieldPath string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: "",
			FieldPath:  fieldPath,
		},
	}
}
