package appworkload_test

import (
	"context"
	"errors"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/knative-runner/controllers"
	"code.cloudfoundry.org/korifi/knative-runner/controllers/appworkload"
	"code.cloudfoundry.org/korifi/knative-runner/controllers/appworkload/fake"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("AppWorkload Reconcile", func() {
	var (
		reconciler          *k8s.PatchingReconciler[korifiv1alpha1.AppWorkload]
		reconcileResult     ctrl.Result
		reconcileErr        error
		req                 ctrl.Request
		appWorkload         *korifiv1alpha1.AppWorkload
		desiredKsvc         *unstructured.Unstructured
		fakeWorkloadToKsvc  *fake.WorkloadToKnativeServiceConverter
		getAppWorkloadError error
		createKsvcError     error
		listPods            *corev1.PodList
		listStatefulSets    *appsv1.StatefulSetList
		listKsvcs           *unstructured.UnstructuredList
	)

	BeforeEach(func() {
		appWorkload = &korifiv1alpha1.AppWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Name:       uuid.NewString(),
				Namespace:  uuid.NewString(),
				Generation: 1,
				Finalizers: []string{finalizer.AppWorkloadFinalizerName},
			},
			Spec: korifiv1alpha1.AppWorkloadSpec{
				RunnerName: controllers.AppWorkloadReconcilerName,
				Instances:  1,
			},
		}

		desiredKsvc = &unstructured.Unstructured{}
		desiredKsvc.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "serving.knative.dev",
			Version: "v1",
			Kind:    "Service",
		})
		desiredKsvc.SetName("kw-test-abc")
		desiredKsvc.SetNamespace(appWorkload.Namespace)
		desiredKsvc.SetLabels(map[string]string{
			appworkload.LabelAppWorkloadGUID: appWorkload.Name,
		})
		Expect(unstructured.SetNestedMap(desiredKsvc.Object, map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]any{
						"autoscaling.knative.dev/min-scale": "0",
					},
				},
				"spec": map[string]any{
					"containers": []any{
						map[string]any{"name": "user-container", "image": "example.com/app"},
					},
				},
			},
		}, "spec")).To(Succeed())

		fakeWorkloadToKsvc = new(fake.WorkloadToKnativeServiceConverter)
		fakeWorkloadToKsvc.ConvertReturns(desiredKsvc, nil)

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      appWorkload.Name,
				Namespace: appWorkload.Namespace,
			},
		}

		getAppWorkloadError = nil
		createKsvcError = nil
		listPods = &corev1.PodList{}
		listStatefulSets = &appsv1.StatefulSetList{}
		listKsvcs = &unstructured.UnstructuredList{}
		listKsvcs.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "serving.knative.dev",
			Version: "v1",
			Kind:    "ServiceList",
		})

		fakeClient.GetStub = func(_ context.Context, nn types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			switch o := obj.(type) {
			case *korifiv1alpha1.AppWorkload:
				if getAppWorkloadError != nil {
					return getAppWorkloadError
				}
				appWorkload.DeepCopyInto(o)
				return nil
			case *unstructured.Unstructured:
				return apierrors.NewNotFound(schema.GroupResource{
					Group:    "serving.knative.dev",
					Resource: "services",
				}, nn.Name)
			default:
				panic("unexpected Get object type")
			}
		}

		fakeClient.CreateStub = func(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
			if _, ok := obj.(*unstructured.Unstructured); ok {
				return createKsvcError
			}
			panic("unexpected Create object type")
		}

		fakeClient.ListStub = func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
			switch l := list.(type) {
			case *corev1.PodList:
				listPods.DeepCopyInto(l)
				return nil
			case *appsv1.StatefulSetList:
				listStatefulSets.DeepCopyInto(l)
				return nil
			case *unstructured.UnstructuredList:
				listKsvcs.DeepCopyInto(l)
				return nil
			default:
				panic("unexpected List object type")
			}
		}

		fakeClient.PatchStub = func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) error {
			if aw, ok := obj.(*korifiv1alpha1.AppWorkload); ok {
				aw.DeepCopyInto(appWorkload)
			}
			return nil
		}

		reconciler = appworkload.NewAppWorkloadReconciler(
			fakeClient,
			scheme.Scheme,
			fakeWorkloadToKsvc,
			ctrl.Log.WithName("controllers").WithName("TestKnativeAppWorkload"),
		)
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = reconciler.Reconcile(context.Background(), req)
	})

	When("the AppWorkload is created", func() {
		It("reconciles without error", func() {
			Expect(reconcileErr).NotTo(HaveOccurred())
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
		})

		It("converts and creates a Knative Service", func() {
			Expect(fakeWorkloadToKsvc.ConvertCallCount()).To(Equal(1))
			Expect(fakeClient.CreateCallCount()).To(Equal(1))
			_, obj, _ := fakeClient.CreateArgsForCall(0)
			created := obj.(*unstructured.Unstructured)
			Expect(created.GetName()).To(Equal("kw-test-abc"))
			Expect(created.GetLabels()).To(HaveKey(appworkload.LabelSpecHash))
		})

		It("sets ActualInstances from ready pods", func() {
			Expect(fakeStatusWriter.PatchCallCount()).To(BeNumerically(">=", 1))
			_, object, _, _ := fakeStatusWriter.PatchArgsForCall(0)
			patched := object.(*korifiv1alpha1.AppWorkload)
			Expect(patched.Status.ObservedGeneration).To(Equal(appWorkload.Generation))
			Expect(patched.Status.ActualInstances).To(BeZero())
			Expect(patched.Status.InstancesStatus["0"].State).To(Equal(korifiv1alpha1.InstanceStateDown))
		})

		When("a ready pod exists", func() {
			BeforeEach(func() {
				listPods = &corev1.PodList{
					Items: []corev1.Pod{{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod-0",
							Namespace: appWorkload.Namespace,
							Labels: map[string]string{
								appworkload.LabelAppWorkloadGUID: appWorkload.Name,
							},
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							}},
						},
					}},
				}
			})

			It("reports a running instance", func() {
				_, object, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				patched := object.(*korifiv1alpha1.AppWorkload)
				Expect(patched.Status.ActualInstances).To(Equal(int32(1)))
				Expect(patched.Status.InstancesStatus["0"].State).To(Equal(korifiv1alpha1.InstanceStateRunning))
			})
		})

		When("conversion fails", func() {
			BeforeEach(func() {
				fakeWorkloadToKsvc.ConvertReturns(nil, errors.New("convert-error"))
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("convert-error"))
			})
		})

		When("creating the Knative Service fails", func() {
			BeforeEach(func() {
				createKsvcError = errors.New("create-failed")
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("create-failed"))
			})
		})

		When("the converted service has no spec", func() {
			BeforeEach(func() {
				desiredKsvc = &unstructured.Unstructured{}
				desiredKsvc.SetGroupVersionKind(schema.GroupVersionKind{
					Group: "serving.knative.dev", Version: "v1", Kind: "Service",
				})
				desiredKsvc.SetName("kw-test-abc")
				desiredKsvc.SetNamespace(appWorkload.Namespace)
				fakeWorkloadToKsvc.ConvertReturns(desiredKsvc, nil)
			})

			It("returns an error", func() {
				Expect(reconcileErr).To(MatchError(ContainSubstring("has no spec")))
			})
		})

		When("the converted service has a non-map spec", func() {
			BeforeEach(func() {
				desiredKsvc.Object["spec"] = "not-a-map"
				fakeWorkloadToKsvc.ConvertReturns(desiredKsvc, nil)
			})

			It("returns an error", func() {
				Expect(reconcileErr).To(HaveOccurred())
			})
		})

		When("spec hashing fails", func() {
			BeforeEach(func() {
				original := *appworkload.MarshalSpecJSON
				DeferCleanup(func() { *appworkload.MarshalSpecJSON = original })
				*appworkload.MarshalSpecJSON = func(any) ([]byte, error) {
					return nil, errors.New("hash-marshal")
				}
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError(ContainSubstring("hash-marshal")))
			})
		})

		When("setting the revision name fails", func() {
			BeforeEach(func() {
				Expect(unstructured.SetNestedField(desiredKsvc.Object, "not-a-map", "spec", "template")).To(Succeed())
				fakeWorkloadToKsvc.ConvertReturns(desiredKsvc, nil)
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(HaveOccurred())
			})
		})

		When("listing pods fails", func() {
			BeforeEach(func() {
				fakeClient.ListStub = func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
					switch list.(type) {
					case *appsv1.StatefulSetList:
						return nil
					case *corev1.PodList:
						return errors.New("list-pods")
					default:
						return nil
					}
				}
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("list-pods"))
			})
		})

		When("a leftover StatefulSet exists", func() {
			BeforeEach(func() {
				listStatefulSets = &appsv1.StatefulSetList{
					Items: []appsv1.StatefulSet{{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "old-sts",
							Namespace: appWorkload.Namespace,
						},
					}},
				}
			})

			It("deletes the StatefulSet", func() {
				Expect(fakeClient.DeleteCallCount()).To(Equal(1))
				_, obj, _ := fakeClient.DeleteArgsForCall(0)
				Expect(obj).To(BeAssignableToTypeOf(&appsv1.StatefulSet{}))
			})

			When("deleting the StatefulSet fails", func() {
				BeforeEach(func() {
					fakeClient.DeleteReturns(errors.New("delete-sts"))
				})

				It("returns the error", func() {
					Expect(reconcileErr).To(MatchError("delete-sts"))
				})
			})

			When("the StatefulSet is already gone", func() {
				BeforeEach(func() {
					fakeClient.DeleteReturns(apierrors.NewNotFound(schema.GroupResource{
						Group: "apps", Resource: "statefulsets",
					}, "old-sts"))
				})

				It("ignores the not-found error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})
		})

		When("listing leftover StatefulSets fails", func() {
			BeforeEach(func() {
				fakeClient.ListStub = func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
					if _, ok := list.(*appsv1.StatefulSetList); ok {
						return errors.New("list-sts")
					}
					return nil
				}
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("list-sts"))
			})
		})

		When("a terminating pod is listed", func() {
			BeforeEach(func() {
				now := metav1.Now()
				listPods = &corev1.PodList{
					Items: []corev1.Pod{{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "dying",
							Namespace:         appWorkload.Namespace,
							DeletionTimestamp: &now,
							Labels:            map[string]string{appworkload.LabelAppWorkloadGUID: appWorkload.Name},
						},
						Status: corev1.PodStatus{
							Conditions: []corev1.PodCondition{{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							}},
						},
					}},
				}
			})

			It("does not count the terminating pod", func() {
				_, object, _, _ := fakeStatusWriter.PatchArgsForCall(0)
				patched := object.(*korifiv1alpha1.AppWorkload)
				Expect(patched.Status.ActualInstances).To(BeZero())
			})
		})

		When("the Knative Service already exists", func() {
			var existing *unstructured.Unstructured

			BeforeEach(func() {
				existing = desiredKsvc.DeepCopy()
				existing.SetUID("existing-uid")
				existing.SetAnnotations(map[string]string{
					"serving.knative.dev/creator": "system",
					"keep-me":                     "yes",
				})
				desiredKsvc.SetAnnotations(map[string]string{
					"serving.knative.dev/creator":            "should-skip",
					"korifi.cloudfoundry.org/application-id": "app",
				})
				fakeWorkloadToKsvc.ConvertReturns(desiredKsvc, nil)

				fakeClient.GetStub = func(_ context.Context, nn types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
					switch o := obj.(type) {
					case *korifiv1alpha1.AppWorkload:
						appWorkload.DeepCopyInto(o)
						return nil
					case *unstructured.Unstructured:
						existing.DeepCopyInto(o)
						return nil
					default:
						panic("unexpected Get object type")
					}
				}
				fakeClient.PatchStub = func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) error {
					if aw, ok := obj.(*korifiv1alpha1.AppWorkload); ok {
						aw.DeepCopyInto(appWorkload)
					}
					if u, ok := obj.(*unstructured.Unstructured); ok {
						u.DeepCopyInto(existing)
					}
					return nil
				}
			})

			It("patches the existing service and skips knative annotations", func() {
				Expect(reconcileErr).NotTo(HaveOccurred())
				Expect(fakeClient.CreateCallCount()).To(Equal(0))
				Expect(fakeClient.PatchCallCount()).To(BeNumerically(">=", 1))
			})

			When("the desired-spec hash is unchanged", func() {
				BeforeEach(func() {
					desiredHash, err := appworkload.SpecHash(map[string]any{
						"template": map[string]any{
							"metadata": map[string]any{
								"annotations": map[string]any{
									"autoscaling.knative.dev/min-scale": "0",
								},
							},
							"spec": map[string]any{
								"containers": []any{
									map[string]any{"name": "user-container", "image": "example.com/app"},
								},
							},
						},
					})
					Expect(err).NotTo(HaveOccurred())
					labels := existing.GetLabels()
					if labels == nil {
						labels = map[string]string{}
					}
					labels[appworkload.LabelSpecHash] = desiredHash
					existing.SetLabels(labels)
				})

				It("reconciles without rewriting the live spec", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("updating the nested spec fails", func() {
				BeforeEach(func() {
					original := *appworkload.SetNestedMap
					DeferCleanup(func() { *appworkload.SetNestedMap = original })
					*appworkload.SetNestedMap = func(map[string]any, map[string]any, ...string) error {
						return errors.New("set-nested-map")
					}
					labels := existing.GetLabels()
					if labels == nil {
						labels = map[string]string{}
					}
					labels[appworkload.LabelSpecHash] = "old-hash"
					existing.SetLabels(labels)
				})

				It("returns the error", func() {
					Expect(reconcileErr).To(MatchError(ContainSubstring("set-nested-map")))
				})
			})
		})
	})

	When("the AppWorkload uses a different runner", func() {
		BeforeEach(func() {
			appWorkload.Spec.RunnerName = "statefulset-runner"
		})

		It("no-ops without converting", func() {
			Expect(reconcileErr).NotTo(HaveOccurred())
			Expect(fakeWorkloadToKsvc.ConvertCallCount()).To(Equal(0))
			Expect(fakeClient.CreateCallCount()).To(Equal(0))
		})
	})

	When("the AppWorkload is missing", func() {
		BeforeEach(func() {
			getAppWorkloadError = apierrors.NewNotFound(schema.GroupResource{
				Group:    "korifi.cloudfoundry.org",
				Resource: "appworkloads",
			}, "missing")
		})

		It("returns success", func() {
			Expect(reconcileErr).NotTo(HaveOccurred())
			Expect(reconcileResult).To(Equal(ctrl.Result{}))
		})
	})

	When("the AppWorkload is being deleted", func() {
		BeforeEach(func() {
			now := metav1.Now()
			appWorkload.DeletionTimestamp = &now
			controllerutil.AddFinalizer(appWorkload, finalizer.AppWorkloadFinalizerName)
		})

		It("removes the finalizer when no Knative Services remain", func() {
			Expect(reconcileErr).NotTo(HaveOccurred())
			Expect(fakeWorkloadToKsvc.ConvertCallCount()).To(Equal(0))
			Expect(appWorkload.Finalizers).NotTo(ContainElement(finalizer.AppWorkloadFinalizerName))
		})

		When("Knative Services are still present", func() {
			BeforeEach(func() {
				remaining := unstructured.Unstructured{}
				remaining.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "serving.knative.dev",
					Version: "v1",
					Kind:    "Service",
				})
				remaining.SetName("still-here")
				remaining.SetNamespace(appWorkload.Namespace)
				listKsvcs.Items = []unstructured.Unstructured{remaining}
			})

			It("deletes them and requeues as not ready", func() {
				Expect(reconcileErr).NotTo(HaveOccurred())
				Expect(reconcileResult.RequeueAfter).To(Equal(time.Second))
				Expect(fakeClient.DeleteCallCount()).To(Equal(1))
			})

			When("deleting a Knative Service fails", func() {
				BeforeEach(func() {
					fakeClient.DeleteReturns(errors.New("delete-ksvc"))
				})

				It("returns the error", func() {
					Expect(reconcileErr).To(MatchError("delete-ksvc"))
				})
			})

			When("the Knative Service is already gone", func() {
				BeforeEach(func() {
					fakeClient.DeleteReturns(apierrors.NewNotFound(schema.GroupResource{
						Group: "serving.knative.dev", Resource: "services",
					}, "still-here"))
				})

				It("requeues while instances remain on the second list", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
					Expect(reconcileResult.RequeueAfter).To(Equal(time.Second))
				})
			})
		})

		When("listing Knative Services fails", func() {
			BeforeEach(func() {
				fakeClient.ListStub = func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
					if _, ok := list.(*unstructured.UnstructuredList); ok {
						return errors.New("list-ksvc")
					}
					return nil
				}
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("list-ksvc"))
			})
		})

		When("the second Knative Service list fails", func() {
			BeforeEach(func() {
				calls := 0
				fakeClient.ListStub = func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
					if _, ok := list.(*unstructured.UnstructuredList); ok {
						calls++
						if calls == 1 {
							listKsvcs.DeepCopyInto(list.(*unstructured.UnstructuredList))
							return nil
						}
						return errors.New("list-ksvc-2")
					}
					return nil
				}
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("list-ksvc-2"))
			})
		})
	})
})
