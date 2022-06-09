package reconciler_test

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/k8sfakes"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/reconciler"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/reconciler/reconcilerfakes"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	"code.cloudfoundry.org/lager"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("reconciler.LRP", func() {
	var (
		logger        lager.Logger
		client        *k8sfakes.FakeClient
		statusWriter  *k8sfakes.FakeStatusWriter
		desirer       *reconcilerfakes.FakeLRPDesirer
		updater       *reconcilerfakes.FakeLRPUpdater
		lrpreconciler *reconciler.LRP
		resultErr     error

		lrp           *eiriniv1.LRP
		getLrpError   error
		statefulSet   *appsv1.StatefulSet
		getStSetError error
	)

	BeforeEach(func() {
		statusWriter = new(k8sfakes.FakeStatusWriter)
		client = new(k8sfakes.FakeClient)
		client.StatusReturns(statusWriter)

		desirer = new(reconcilerfakes.FakeLRPDesirer)
		updater = new(reconcilerfakes.FakeLRPUpdater)
		logger = tests.NewTestLogger("lrp-reconciler")
		lrpreconciler = reconciler.NewLRP(logger, client, desirer, updater)

		lrp = &eiriniv1.LRP{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-lrp",
				Namespace: "some-ns",
			},
			Spec: eiriniv1.LRPSpec{
				GUID:        "the-lrp-guid",
				Version:     "the-lrp-version",
				Command:     []string{"ls", "-la"},
				Instances:   10,
				ProcessType: "web",
				AppName:     "the-app",
				AppGUID:     "the-app-guid",
				OrgName:     "the-org",
				OrgGUID:     "the-org-guid",
				SpaceName:   "the-space",
				SpaceGUID:   "the-space-guid",
				Image:       "eirini/dorini",
				Env: map[string]string{
					"FOO": "BAR",
				},
				Environment: []corev1.EnvVar{{
					Name: "PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "jim"},
							Key:                  "password",
						},
					},
				}},
				Ports:     []int32{8080, 9090},
				MemoryMB:  1024,
				DiskMB:    512,
				CPUWeight: 128,
				Sidecars: []eiriniv1.Sidecar{
					{
						Name:     "hello-sidecar",
						Command:  []string{"sh", "-c", "echo hello"},
						MemoryMB: 8,
						Env: map[string]string{
							"SIDE": "BUS",
						},
					},
					{
						Name:     "bye-sidecar",
						Command:  []string{"sh", "-c", "echo bye"},
						MemoryMB: 16,
						Env: map[string]string{
							"SIDE": "CAR",
						},
					},
				},
				VolumeMounts: []eiriniv1.VolumeMount{
					{
						MountPath: "/path/to/mount",
						ClaimName: "claim-q1",
					},
					{
						MountPath: "/path/in/the/other/direction",
						ClaimName: "claim-c2",
					},
				},
				Health: eiriniv1.Healthcheck{
					Type:      "http",
					Port:      9090,
					Endpoint:  "/heath",
					TimeoutMs: 80,
				},
				UserDefinedAnnotations: map[string]string{
					"user-annotaions.io": "yes",
				},
			},
		}

		getLrpError = nil
		getStSetError = apierrors.NewNotFound(schema.GroupResource{}, "not found")
		client.GetStub = func(_ context.Context, _ types.NamespacedName, object k8sclient.Object) error {
			lrpPtr, ok := object.(*eiriniv1.LRP)
			if ok {
				if getLrpError != nil {
					return getLrpError
				}

				if lrp == nil {
					return apierrors.NewNotFound(schema.GroupResource{}, "")
				}

				lrp.DeepCopyInto(lrpPtr)

				return nil
			}

			stSetPtr, ok := object.(*appsv1.StatefulSet)
			if ok {
				if getStSetError != nil {
					return getStSetError
				}

				if statefulSet == nil {
					return apierrors.NewNotFound(schema.GroupResource{}, "")
				}

				statefulSet.DeepCopyInto(stSetPtr)

				return nil
			}

			Fail(fmt.Sprintf("Unexpected object: %v", object))

			return nil
		}
	})

	JustBeforeEach(func() {
		_, resultErr = lrpreconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "some-ns",
				Name:      "app",
			},
		})
	})

	It("creates a statefulset for each CRD", func() {
		Expect(resultErr).NotTo(HaveOccurred())

		Expect(updater.UpdateCallCount()).To(Equal(0))
		Expect(desirer.DesireCallCount()).To(Equal(1))

		_, actualLRP := desirer.DesireArgsForCall(0)
		Expect(actualLRP.Namespace).To(Equal("some-ns"))
		Expect(actualLRP.Spec.GUID).To(Equal("the-lrp-guid"))
		Expect(actualLRP.Spec.Version).To(Equal("the-lrp-version"))
		Expect(actualLRP.Spec.Command).To(ConsistOf("ls", "-la"))
		Expect(actualLRP.Spec.Instances).To(Equal(10))
		Expect(actualLRP.Spec.PrivateRegistry).To(BeNil())
		Expect(actualLRP.Spec.ProcessType).To(Equal("web"))
		Expect(actualLRP.Spec.AppName).To(Equal("the-app"))
		Expect(actualLRP.Spec.AppGUID).To(Equal("the-app-guid"))
		Expect(actualLRP.Spec.OrgName).To(Equal("the-org"))
		Expect(actualLRP.Spec.OrgGUID).To(Equal("the-org-guid"))
		Expect(actualLRP.Spec.SpaceName).To(Equal("the-space"))
		Expect(actualLRP.Spec.SpaceGUID).To(Equal("the-space-guid"))
		Expect(actualLRP.Spec.Image).To(Equal("eirini/dorini"))
		Expect(actualLRP.Spec.Env).To(Equal(map[string]string{
			"FOO": "BAR",
		}))
		Expect(actualLRP.Spec.Environment).To(HaveLen(1))
		Expect(actualLRP.Spec.Ports).To(Equal([]int32{8080, 9090}))
		Expect(actualLRP.Spec.MemoryMB).To(Equal(int64(1024)))
		Expect(actualLRP.Spec.DiskMB).To(Equal(int64(512)))
		Expect(actualLRP.Spec.CPUWeight).To(Equal(uint8(128)))
		Expect(actualLRP.Spec.Sidecars).To(Equal([]eiriniv1.Sidecar{
			{
				Name:     "hello-sidecar",
				Command:  []string{"sh", "-c", "echo hello"},
				MemoryMB: 8,
				Env: map[string]string{
					"SIDE": "BUS",
				},
			},
			{
				Name:     "bye-sidecar",
				Command:  []string{"sh", "-c", "echo bye"},
				MemoryMB: 16,
				Env: map[string]string{
					"SIDE": "CAR",
				},
			},
		}))
		Expect(actualLRP.Spec.VolumeMounts).To(Equal([]eiriniv1.VolumeMount{
			{
				MountPath: "/path/to/mount",
				ClaimName: "claim-q1",
			},
			{
				MountPath: "/path/in/the/other/direction",
				ClaimName: "claim-c2",
			},
		}))
		Expect(actualLRP.Spec.Health).To(Equal(eiriniv1.Healthcheck{
			Type:      "http",
			Port:      9090,
			Endpoint:  "/heath",
			TimeoutMs: 80,
		}))
		Expect(actualLRP.Spec.UserDefinedAnnotations).To(Equal(map[string]string{
			"user-annotaions.io": "yes",
		}))
	})

	It("does not update the LRP CR", func() {
		Expect(statusWriter.PatchCallCount()).To(BeZero())
		Expect(statusWriter.UpdateCallCount()).To(BeZero())
	})

	When("the statefulset for the LRP already exists", func() {
		BeforeEach(func() {
			getStSetError = nil
			statefulSet = &appsv1.StatefulSet{}
			lrp.Status = eiriniv1.LRPStatus{
				Replicas: 9,
			}
		})

		It("updates the CR status accordingly", func() {
			Expect(resultErr).NotTo(HaveOccurred())

			Expect(statusWriter.PatchCallCount()).To(Equal(1))
			_, actualObject, _, _ := statusWriter.PatchArgsForCall(0)
			actualLrp, ok := actualObject.(*eiriniv1.LRP)
			Expect(ok).To(BeTrue())
			Expect(actualLrp.Name).To(Equal("some-lrp"))
			Expect(updater.UpdateCallCount()).To(Equal(1))
		})

		When("the workload client fails to update the app", func() {
			BeforeEach(func() {
				updater.UpdateReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(resultErr).To(MatchError(ContainSubstring("boom")))
			})
		})
	})

	When("private registry credentials are specified in the LRP CRD", func() {
		BeforeEach(func() {
			lrp = &eiriniv1.LRP{
				Spec: eiriniv1.LRPSpec{
					Image: "private-registry.com:5000/repo/app-image:latest",
					PrivateRegistry: &eiriniv1.PrivateRegistry{
						Username: "docker-user",
						Password: "docker-password",
					},
				},
			}
		})

		It("configures a private registry", func() {
			Expect(desirer.DesireCallCount()).To(Equal(1))
			_, actualLRP := desirer.DesireArgsForCall(0)
			privateRegistry := actualLRP.Spec.PrivateRegistry
			Expect(privateRegistry).NotTo(BeNil())
			Expect(privateRegistry.Username).To(Equal("docker-user"))
			Expect(privateRegistry.Password).To(Equal("docker-password"))
		})
	})

	When("the LRP doesn't exist", func() {
		BeforeEach(func() {
			lrp = nil
		})

		It("does not return an error", func() {
			Expect(resultErr).NotTo(HaveOccurred())
		})
	})

	When("the controller client fails to get the CRD", func() {
		BeforeEach(func() {
			getLrpError = errors.New("boom")
		})

		It("returns an error", func() {
			Expect(resultErr).To(MatchError(ContainSubstring("boom")))
		})
	})

	When("the getting the statefulset fails", func() {
		BeforeEach(func() {
			getStSetError = errors.New("boom")
		})

		It("returns an error", func() {
			Expect(resultErr).To(MatchError("failed to get statefulSet: boom"))
		})
	})

	When("the workload client fails to desire the app", func() {
		BeforeEach(func() {
			desirer.DesireReturns(errors.New("boom"))
		})

		It("returns an error", func() {
			Expect(resultErr).To(MatchError("failed to desire lrp: boom"))
		})
	})
})
