package appworkload_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/knative-runner/controllers"
	"code.cloudfoundry.org/korifi/knative-runner/controllers/appworkload"
	"code.cloudfoundry.org/korifi/knative-runner/controllers/appworkload/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var _ = Describe("Helpers", func() {
	Describe("revisionName", func() {
		It("joins service name and hash", func() {
			Expect(appworkload.RevisionName("kw-app", "abcd1234efgh5678")).To(Equal("kw-app-abcd1234efgh5678"))
		})

		It("truncates hash to fit the 63-char DNS limit", func() {
			name := strings.Repeat("a", 50)
			got := appworkload.RevisionName(name, "0123456789abcdef")
			Expect(len(got)).To(BeNumerically("<=", 63))
			Expect(got).To(HavePrefix(name + "-"))
		})

		It("returns empty when the service name leaves no room for a suffix", func() {
			Expect(appworkload.RevisionName(strings.Repeat("n", 62), "hash")).To(BeEmpty())
		})
	})

	Describe("specHash", func() {
		It("returns a stable 16-char hex digest", func() {
			h1, err := appworkload.SpecHash(map[string]any{"a": "b"})
			Expect(err).NotTo(HaveOccurred())
			h2, err := appworkload.SpecHash(map[string]any{"a": "b"})
			Expect(err).NotTo(HaveOccurred())
			Expect(h1).To(Equal(h2))
			Expect(h1).To(HaveLen(16))
		})

		It("returns an error when marshaling fails", func() {
			original := *appworkload.MarshalSpecJSON
			DeferCleanup(func() { *appworkload.MarshalSpecJSON = original })
			*appworkload.MarshalSpecJSON = func(any) ([]byte, error) {
				return nil, errors.New("marshal-boom")
			}
			_, err := appworkload.SpecHash(map[string]any{"a": "b"})
			Expect(err).To(MatchError(ContainSubstring("marshal-boom")))
		})
	})

	Describe("filterAppWorkloads", func() {
		It("keeps non-AppWorkload objects", func() {
			Expect(appworkload.FilterAppWorkloads(&corev1.Pod{})).To(BeTrue())
		})

		It("keeps AppWorkloads for the knative runner", func() {
			Expect(appworkload.FilterAppWorkloads(&korifiv1alpha1.AppWorkload{
				Spec: korifiv1alpha1.AppWorkloadSpec{RunnerName: controllers.AppWorkloadReconcilerName},
			})).To(BeTrue())
		})

		It("drops AppWorkloads for other runners", func() {
			Expect(appworkload.FilterAppWorkloads(&korifiv1alpha1.AppWorkload{
				Spec: korifiv1alpha1.AppWorkloadSpec{RunnerName: "statefulset-runner"},
			})).To(BeFalse())
		})
	})

	Describe("enqueueAppWorkloadRequests", func() {
		var reconciler *appworkload.AppWorkloadReconciler

		BeforeEach(func() {
			reconciler = appworkload.NewTestAppWorkloadReconciler(nil, scheme.Scheme, nil, ctrl.Log)
		})

		It("enqueues from the appworkload-guid label", func() {
			reqs := reconciler.EnqueueAppWorkloadRequests(context.Background(), &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod",
					Namespace: "ns",
					Labels:    map[string]string{appworkload.LabelAppWorkloadGUID: "aw-1"},
				},
			})
			Expect(reqs).To(HaveLen(1))
			Expect(reqs[0].Name).To(Equal("aw-1"))
			Expect(reqs[0].Namespace).To(Equal("ns"))
		})

		It("returns nothing without the label", func() {
			Expect(reconciler.EnqueueAppWorkloadRequests(context.Background(), &corev1.Pod{})).To(BeEmpty())
		})
	})

	Describe("sanitizeName and truncateString", func() {
		It("keeps a valid DNS-1035 name and truncates long ones", func() {
			Expect(appworkload.SanitizeName("kw-my-app", "kw-fallback")).To(Equal("kw-my-app"))
			long := "kw-" + strings.Repeat("a", 80)
			Expect(appworkload.SanitizeName(long, "kw-fallback")).To(HaveLen(40))
		})

		It("falls back when the name is not DNS-1035", func() {
			Expect(appworkload.SanitizeName("123-nope", "kw-fallback")).To(Equal("kw-fallback"))
			Expect(appworkload.SanitizeName("123-nope", "kw-"+strings.Repeat("f", 80))).To(HaveLen(40))
		})

		It("truncates only when needed", func() {
			Expect(appworkload.TruncateString("abc", 10)).To(Equal("abc"))
			Expect(appworkload.TruncateString("abcdefghij", 4)).To(Equal("abcd"))
		})
	})

	Describe("knativeServiceName", func() {
		It("uses the fallback sanitize path for awkward app GUIDs", func() {
			name := appworkload.KnativeServiceName(&korifiv1alpha1.AppWorkload{
				Spec: korifiv1alpha1.AppWorkloadSpec{
					AppGUID: "!!!",
					GUID:    "guid",
					Version: "v1",
				},
			})
			Expect(name).To(HavePrefix("kw-guid-"))
		})
	})

	Describe("converter error paths", func() {
		var appWorkload *korifiv1alpha1.AppWorkload

		BeforeEach(func() {
			appWorkload = &korifiv1alpha1.AppWorkload{
				ObjectMeta: metav1.ObjectMeta{Name: "aw", Namespace: "ns"},
				Spec: korifiv1alpha1.AppWorkloadSpec{
					AppGUID: "app",
					GUID:    "guid",
					Version: "v1",
					Image:   "example.com/app",
				},
			}
		})

		It("surfaces marshaledPodSpec marshal errors", func() {
			original := *appworkload.MarshalJSON
			DeferCleanup(func() { *appworkload.MarshalJSON = original })
			*appworkload.MarshalJSON = func(any) ([]byte, error) {
				return nil, errors.New("pod-marshal")
			}
			_, err := appworkload.MarshaledPodSpec(appWorkload)
			Expect(err).To(MatchError(ContainSubstring("pod-marshal")))
		})

		It("surfaces marshaledPodSpec unmarshal errors", func() {
			originalM := *appworkload.MarshalJSON
			originalU := *appworkload.UnmarshalJSON
			DeferCleanup(func() {
				*appworkload.MarshalJSON = originalM
				*appworkload.UnmarshalJSON = originalU
			})
			*appworkload.MarshalJSON = func(any) ([]byte, error) { return []byte(`{}`), nil }
			*appworkload.UnmarshalJSON = func([]byte, any) error { return errors.New("pod-unmarshal") }
			_, err := appworkload.MarshaledPodSpec(appWorkload)
			Expect(err).To(MatchError(ContainSubstring("pod-unmarshal")))
		})

		It("surfaces buildKnativeService marshal errors", func() {
			original := *appworkload.MarshalJSON
			DeferCleanup(func() { *appworkload.MarshalJSON = original })
			*appworkload.MarshalJSON = func(any) ([]byte, error) {
				return nil, errors.New("ksvc-marshal")
			}
			_, err := appworkload.BuildKnativeService("n", "ns", nil, nil, nil, nil)
			Expect(err).To(MatchError(ContainSubstring("ksvc-marshal")))
		})

		It("surfaces buildKnativeService unmarshal errors", func() {
			original := *appworkload.UnstructuredFromJSON
			DeferCleanup(func() { *appworkload.UnstructuredFromJSON = original })
			*appworkload.UnstructuredFromJSON = func(*unstructured.Unstructured, []byte) error {
				return errors.New("ksvc-unmarshal")
			}
			_, err := appworkload.BuildKnativeService("n", "ns", map[string]string{}, map[string]string{}, map[string]string{}, map[string]any{})
			Expect(err).To(MatchError(ContainSubstring("ksvc-unmarshal")))
		})

		It("surfaces Convert errors from marshaledPodSpec", func() {
			original := *appworkload.MarshalJSON
			DeferCleanup(func() { *appworkload.MarshalJSON = original })
			*appworkload.MarshalJSON = func(any) ([]byte, error) {
				return nil, errors.New("convert-pod")
			}
			_, err := appworkload.NewAppWorkloadToKnativeServiceConverter(scheme.Scheme).Convert(appWorkload)
			Expect(err).To(MatchError(ContainSubstring("convert-pod")))
		})

		It("surfaces Convert errors from buildKnativeService", func() {
			calls := 0
			original := *appworkload.MarshalJSON
			DeferCleanup(func() { *appworkload.MarshalJSON = original })
			*appworkload.MarshalJSON = func(v any) ([]byte, error) {
				calls++
				if calls == 1 {
					return original(v)
				}
				return nil, errors.New("convert-ksvc")
			}
			_, err := appworkload.NewAppWorkloadToKnativeServiceConverter(scheme.Scheme).Convert(appWorkload)
			Expect(err).To(MatchError(ContainSubstring("convert-ksvc")))
		})
	})
})

var _ = Describe("SetupWithManager", Ordered, func() {
	var testEnv *envtest.Environment

	BeforeAll(func() {
		assets := os.Getenv("KUBEBUILDER_ASSETS")
		if assets == "" {
			matches, err := filepath.Glob(filepath.Join("..", "..", "..", "testbin", "k8s", "*"))
			Expect(err).NotTo(HaveOccurred())
			Expect(matches).NotTo(BeEmpty(), "set KUBEBUILDER_ASSETS (scripts/run-tests.sh) or install envtest binaries under testbin/k8s")
			assets, err = filepath.Abs(matches[0])
			Expect(err).NotTo(HaveOccurred())
		}

		testEnv = &envtest.Environment{
			BinaryAssetsDirectory: assets,
			CRDDirectoryPaths: []string{
				filepath.Join("..", "..", "..", "helm", "korifi", "controllers", "crds"),
			},
			ErrorIfCRDPathMissing: true,
		}
		_, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		Expect(testEnv.Stop()).To(Succeed())
	})

	It("registers the AppWorkload controller builder", func() {
		mgr, err := ctrl.NewManager(testEnv.Config, ctrl.Options{
			Scheme: scheme.Scheme,
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		reconciler := appworkload.NewTestAppWorkloadReconciler(
			mgr.GetClient(),
			mgr.GetScheme(),
			new(fake.WorkloadToKnativeServiceConverter),
			ctrl.Log.WithName("test"),
		)
		builder := reconciler.SetupWithManager(mgr)
		Expect(builder).NotTo(BeNil())
	})
})
