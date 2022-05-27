package workloads_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/controllers/config"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	workloadsfakes "code.cloudfoundry.org/korifi/controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("CFBuildReconciler", func() {
	const (
		defaultNamespace       = "default"
		stagingConditionType   = "Staging"
		succeededConditionType = "Succeeded"
	)

	var (
		fakeClient       *workloadsfakes.CFClient
		fakeStatusWriter *fake.StatusWriter

		fakeEnvBuilder *workloadsfakes.EnvBuilder

		cfAppGUID         string
		cfPackageGUID     string
		cfBuildGUID       string
		buildWorkloadName string

		cfBuild        *korifiv1alpha1.CFBuild
		cfBuildError   error
		cfApp          *korifiv1alpha1.CFApp
		cfAppError     error
		cfPackage      *korifiv1alpha1.CFPackage
		cfPackageError error

		buildWorkload      *korifiv1alpha1.BuildWorkload
		buildWorkloadError error
		cfBuildReconciler  *CFBuildReconciler
		req                ctrl.Request
		ctx                context.Context

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		fakeClient = new(workloadsfakes.CFClient)

		cfAppGUID = "cf-app-guid"
		cfPackageGUID = "cf-package-guid"
		cfBuildGUID = "cf-build-guid"
		buildWorkloadName = cfBuildGUID

		cfBuild = BuildCFBuildObject(cfBuildGUID, defaultNamespace, cfPackageGUID, cfAppGUID)
		cfBuildError = nil
		cfApp = BuildCFAppCRObject(cfAppGUID, defaultNamespace)
		cfAppError = nil
		cfPackage = BuildCFPackageCRObject(cfPackageGUID, defaultNamespace, cfAppGUID)
		cfPackageError = nil
		buildWorkload = mockBuildWorkloadResource(buildWorkloadName, defaultNamespace)
		buildWorkloadError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)

		fakeClient.GetStub = func(_ context.Context, namespacedName types.NamespacedName, obj client.Object) error {
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFBuild:
				cfBuild.DeepCopyInto(obj)
				return cfBuildError
			case *korifiv1alpha1.CFApp:
				cfApp.DeepCopyInto(obj)
				return cfAppError
			case *korifiv1alpha1.CFPackage:
				cfPackage.DeepCopyInto(obj)
				return cfPackageError
			case *korifiv1alpha1.BuildWorkload:
				buildWorkload.DeepCopyInto(obj)
				return buildWorkloadError
			default:
				panic("test Client Get provided a weird obj")
			}
		}

		fakeStatusWriter = new(fake.StatusWriter)
		fakeClient.StatusReturns(fakeStatusWriter)

		fakeEnvBuilder = new(workloadsfakes.EnvBuilder)
		fakeEnvBuilder.BuildEnvReturns(map[string]string{"foo": "var"}, nil)

		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(buildv1alpha2.AddToScheme(scheme.Scheme)).To(Succeed())
		cfBuildReconciler = NewCFBuildReconciler(
			fakeClient,
			scheme.Scheme,
			zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
			&config.ControllerConfig{},
			fakeEnvBuilder,
		)
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: defaultNamespace,
				Name:      cfBuildGUID,
			},
		}
		ctx = context.Background()
	})

	JustBeforeEach(func() {
		reconcileResult, reconcileErr = cfBuildReconciler.Reconcile(ctx, req)
	})

	When("CFBuild status conditions are unknown and", func() {
		When("on the happy path", func() {
			It("does not return an error", func() {
				Expect(reconcileErr).NotTo(HaveOccurred())
			})

			It("returns an empty result", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
			})

			It("should create BuildWorkload with the same GUID as the CF Build", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(1), "fakeClient Create was not called 1 time")
				_, actualWorkload, _ := fakeClient.CreateArgsForCall(0)
				Expect(actualWorkload.GetName()).To(Equal(cfBuildGUID))
			})

			It("should update the status conditions on CFBuild", func() {
				Expect(fakeClient.StatusCallCount()).To(Equal(1))
			})

			It("should create BuildWorkload with CFBuild owner reference", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(1), "fakeClient Create was not called 1 time")
				_, actualWorkload, _ := fakeClient.CreateArgsForCall(0)
				ownerRefs := actualWorkload.GetOwnerReferences()
				Expect(ownerRefs).To(ConsistOf(metav1.OwnerReference{
					UID:        cfBuild.UID,
					Kind:       cfBuild.Kind,
					APIVersion: cfBuild.APIVersion,
					Name:       cfBuild.Name,
				}))
			})

			It("sets the env vars on the BuildWorkload", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(1))
				_, obj, _ := fakeClient.CreateArgsForCall(0)
				actualWorkload, ok := obj.(*korifiv1alpha1.BuildWorkload)
				Expect(ok).To(BeTrue(), "create wasn't passed a buildWorkload")

				Expect(fakeEnvBuilder.BuildEnvCallCount()).To(Equal(1))
				_, actualApp := fakeEnvBuilder.BuildEnvArgsForCall(0)
				Expect(actualApp).To(Equal(cfApp))

				Expect(actualWorkload.Spec.Env).To(HaveLen(1))
				Expect(actualWorkload.Spec.Env[0].Name).To(Equal("foo"))
				Expect(actualWorkload.Spec.Env[0].Value).To(Equal("var"))
			})
		})

		When("on unhappy path", func() {
			When("fetch CFBuild returns an error", func() {
				BeforeEach(func() {
					cfBuildError = errors.New("failing on purpose")
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})

			When("fetch CFBuild returns a NotFoundError", func() {
				BeforeEach(func() {
					cfBuildError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)
				})

				It("should NOT return any error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("fetch CFApp returns an error", func() {
				BeforeEach(func() {
					cfAppError = errors.New("failing on purpose")
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})

			When("fetch CFPackage returns an error", func() {
				BeforeEach(func() {
					cfPackageError = errors.New("failing on purpose")
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})

			When("list CFServiceBindings returns an error", func() {
				BeforeEach(func() {
					fakeClient.ListReturns(errors.New("failing on purpose"))
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})

			When("create BuildWorkload returns an error", func() {
				BeforeEach(func() {
					fakeClient.CreateReturns(errors.New("failing on purpose"))
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})

			When("update status conditions returns an error", func() {
				BeforeEach(func() {
					fakeStatusWriter.UpdateReturns(errors.New("failing on purpose"))
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})

			When("building environment fails", func() {
				BeforeEach(func() {
					fakeEnvBuilder.BuildEnvReturns(nil, errors.New("boom"))
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})
		})
	})

	When("CFBuild status conditions Staging=True and others are unknown", func() {
		BeforeEach(func() {
			SetStatusCondition(&cfBuild.Status.Conditions, stagingConditionType, metav1.ConditionTrue)
			SetStatusCondition(&cfBuild.Status.Conditions, succeededConditionType, metav1.ConditionUnknown)
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				buildWorkloadError = nil
				setBuildWorkloadStatus(buildWorkload, succeededConditionType, metav1.ConditionTrue)
			})

			It("does not return an error", func() {
				Expect(reconcileErr).NotTo(HaveOccurred())
			})

			It("returns an empty result", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
			})

			It("does not create a new BuildWorkload", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(0), "fakeClient Create was called When It should not have been")
			})

			It("should update the status conditions on CFBuild", func() {
				Expect(fakeClient.StatusCallCount()).To(Equal(1))
			})
		})

		When("on the unhappy path", func() {
			When("fetch BuildWorkload returns an error", func() {
				BeforeEach(func() {
					buildWorkloadError = errors.New("failing on purpose")
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})

			When("fetch BuildWorkload returns a NotFoundError", func() {
				BeforeEach(func() {
					buildWorkloadError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)
				})

				It("should NOT return any error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("BuildWorkload is missing status condition Succeeded", func() {
				It("does not return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})

				It("returns an empty result", func() {
					Expect(reconcileResult).To(Equal(ctrl.Result{}))
				})
			})

			When("BuildWorkload status condition Succeeded=Unknown", func() {
				BeforeEach(func() {
					buildWorkloadError = nil
					setBuildWorkloadStatus(buildWorkload, succeededConditionType, metav1.ConditionUnknown)
				})

				It("does not return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})

				It("returns an empty result", func() {
					Expect(reconcileResult).To(Equal(ctrl.Result{}))
				})
			})

			When("BuildWorkload status condition Succeeded=False", func() {
				BeforeEach(func() {
					buildWorkloadError = nil
					setBuildWorkloadStatus(buildWorkload, succeededConditionType, metav1.ConditionFalse)
				})

				It("does not return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})

				It("updates status conditions on CFBuild", func() {
					Expect(fakeClient.StatusCallCount()).To(Equal(1))
				})

				When("update status conditions returns an error", func() {
					BeforeEach(func() {
						fakeStatusWriter.UpdateReturns(errors.New("failing on purpose"))
					})

					It("returns an error", func() {
						Expect(reconcileErr).To(HaveOccurred())
					})
				})
			})

			When("BuildWorkload status condition Succeeded=True", func() {
				BeforeEach(func() {
					buildWorkloadError = nil
					setBuildWorkloadStatus(buildWorkload, succeededConditionType, metav1.ConditionTrue)
				})

				When("update status conditions returns an error", func() {
					BeforeEach(func() {
						fakeStatusWriter.UpdateReturns(errors.New("failing on purpose"))
					})

					It("should return an error", func() {
						Expect(reconcileErr).To(HaveOccurred())
					})
				})
			})
		})
	})
})

func mockBuildWorkloadResource(guid string, namespace string) *korifiv1alpha1.BuildWorkload {
	return &korifiv1alpha1.BuildWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: namespace,
		},
		Spec: korifiv1alpha1.BuildWorkloadSpec{
			Source: korifiv1alpha1.PackageSource{
				Registry: korifiv1alpha1.Registry{
					Image:            "image-path",
					ImagePullSecrets: nil,
				},
			},
		},
	}
}

func setBuildWorkloadStatus(workload *korifiv1alpha1.BuildWorkload, conditionType string, conditionStatus metav1.ConditionStatus) {
	meta.SetStatusCondition(&workload.Status.Conditions, metav1.Condition{
		Type:   conditionType,
		Status: conditionStatus,
		Reason: "shrug",
	})
}
