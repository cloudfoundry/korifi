package workloads_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	workloadsfakes "code.cloudfoundry.org/korifi/controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		defaultNamespace        = "default"
		stagingConditionType    = "Staging"
		succeededConditionType  = "Succeeded"
		kpackReadyConditionType = "Ready"
	)

	var (
		fakeClient       *workloadsfakes.CFClient
		fakeStatusWriter *fake.StatusWriter

		fakeRegistryAuthFetcher *workloadsfakes.RegistryAuthFetcher
		fakeImageProcessFetcher *workloadsfakes.ImageProcessFetcher
		fakeEnvBuilder          *workloadsfakes.EnvBuilder

		cfAppGUID               string
		cfPackageGUID           string
		cfBuildGUID             string
		kpackImageGUID          string
		kpackRegistrySecretName string
		kpackServiceAccountName string

		cfBuild        *v1alpha1.CFBuild
		cfBuildError   error
		cfApp          *v1alpha1.CFApp
		cfAppError     error
		cfPackage      *v1alpha1.CFPackage
		cfPackageError error

		kpackServiceAccount      *corev1.ServiceAccount
		kpackServiceAccountError error
		kpackRegistrySecret      *corev1.Secret
		kpackRegistrySecretError error

		kpackImage        *buildv1alpha2.Image
		kpackImageError   error
		cfBuildReconciler *CFBuildReconciler
		req               ctrl.Request
		ctx               context.Context

		reconcileResult ctrl.Result
		reconcileErr    error
	)

	BeforeEach(func() {
		fakeClient = new(workloadsfakes.CFClient)

		cfAppGUID = "cf-app-guid"
		cfPackageGUID = "cf-package-guid"
		cfBuildGUID = "cf-build-guid"
		kpackImageGUID = cfBuildGUID

		kpackRegistrySecretName = "source-registry-image-pull-secret"
		kpackServiceAccountName = "kpack-service-account"

		cfBuild = BuildCFBuildObject(cfBuildGUID, defaultNamespace, cfPackageGUID, cfAppGUID)
		cfBuildError = nil
		cfApp = BuildCFAppCRObject(cfAppGUID, defaultNamespace)
		cfAppError = nil
		cfPackage = BuildCFPackageCRObject(cfPackageGUID, defaultNamespace, cfAppGUID)
		cfPackageError = nil
		kpackImage = mockKpackImageObject(kpackImageGUID, defaultNamespace)
		kpackImageError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)

		kpackRegistrySecret = BuildDockerRegistrySecret(kpackRegistrySecretName, defaultNamespace)
		kpackRegistrySecretError = nil
		kpackServiceAccount = BuildServiceAccount(kpackServiceAccountName, defaultNamespace, kpackRegistrySecretName)
		kpackServiceAccountError = nil

		fakeClient.GetStub = func(_ context.Context, namespacedName types.NamespacedName, obj client.Object) error {
			// cast obj to find its kind
			switch obj := obj.(type) {
			case *v1alpha1.CFBuild:
				cfBuild.DeepCopyInto(obj)
				return cfBuildError
			case *v1alpha1.CFApp:
				cfApp.DeepCopyInto(obj)
				return cfAppError
			case *v1alpha1.CFPackage:
				cfPackage.DeepCopyInto(obj)
				return cfPackageError
			case *buildv1alpha2.Image:
				kpackImage.DeepCopyInto(obj)
				return kpackImageError
			case *corev1.ServiceAccount:
				kpackServiceAccount.DeepCopyInto(obj)
				return kpackServiceAccountError
			case *corev1.Secret:
				kpackRegistrySecret.DeepCopyInto(obj)
				return kpackRegistrySecretError
			default:
				panic("test Client Get provided a weird obj")
			}
		}

		fakeStatusWriter = new(fake.StatusWriter)
		fakeClient.StatusReturns(fakeStatusWriter)

		fakeRegistryAuthFetcher = new(workloadsfakes.RegistryAuthFetcher)
		fakeImageProcessFetcher = new(workloadsfakes.ImageProcessFetcher)
		fakeEnvBuilder = new(workloadsfakes.EnvBuilder)
		fakeEnvBuilder.BuildEnvReturns(map[string]string{"foo": "var"}, nil)

		// configure a CFBuildReconciler with the client
		Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		Expect(buildv1alpha2.AddToScheme(scheme.Scheme)).To(Succeed())
		cfBuildReconciler = &CFBuildReconciler{
			Client:              fakeClient,
			Scheme:              scheme.Scheme,
			Log:                 zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
			ControllerConfig:    &config.ControllerConfig{KpackImageTag: "image/registry/tag"},
			RegistryAuthFetcher: fakeRegistryAuthFetcher.Spy,
			ImageProcessFetcher: fakeImageProcessFetcher.Spy,
			EnvBuilder:          fakeEnvBuilder,
		}
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

			It("should create kpack image with the same GUID as the CF Build", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(1), "fakeClient Create was not called 1 time")
				_, actualKpackImage, _ := fakeClient.CreateArgsForCall(0)
				Expect(actualKpackImage.GetName()).To(Equal(cfBuildGUID))
			})

			It("should update the status conditions on CFBuild", func() {
				Expect(fakeClient.StatusCallCount()).To(Equal(1))
			})

			It("should create kpack Image with CFBuild owner reference", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(1), "fakeClient Create was not called 1 time")
				_, actualKpackImage, _ := fakeClient.CreateArgsForCall(0)
				ownerRefs := actualKpackImage.GetOwnerReferences()
				Expect(ownerRefs).To(ConsistOf(metav1.OwnerReference{
					UID:        cfBuild.UID,
					Kind:       cfBuild.Kind,
					APIVersion: cfBuild.APIVersion,
					Name:       cfBuild.Name,
				}))
			})

			It("sets the env vars on the kpack image", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(1))
				_, obj, _ := fakeClient.CreateArgsForCall(0)
				actualKpackImage, ok := obj.(*buildv1alpha2.Image)
				Expect(ok).To(BeTrue(), "create wasn't passed a kpackImage")

				Expect(fakeEnvBuilder.BuildEnvCallCount()).To(Equal(1))
				_, actualApp := fakeEnvBuilder.BuildEnvArgsForCall(0)
				Expect(actualApp).To(Equal(cfApp))

				Expect(actualKpackImage.Spec.Build.Env).To(HaveLen(1))
				Expect(actualKpackImage.Spec.Build.Env[0].Name).To(Equal("foo"))
				Expect(actualKpackImage.Spec.Build.Env[0].Value).To(Equal("var"))
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

			When("the kpack registry secret does not exist", func() {
				BeforeEach(func() {
					kpackRegistrySecretError = apierrors.NewNotFound(schema.GroupResource{}, kpackRegistrySecretName)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})

				It("does not create a kpack image", func() {
					Expect(fakeClient.CreateCallCount()).To(BeZero())
				})
			})

			When("create Kpack Image returns an error", func() {
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

	When("CFBuild status conditions for Staging is True and others are unknown", func() {
		BeforeEach(func() {
			SetStatusCondition(&cfBuild.Status.Conditions, stagingConditionType, metav1.ConditionTrue)
			SetStatusCondition(&cfBuild.Status.Conditions, succeededConditionType, metav1.ConditionUnknown)
		})

		When("on the happy path", func() {
			BeforeEach(func() {
				kpackImageError = nil
				setKpackImageStatus(kpackImage, kpackReadyConditionType, "True")
			})

			It("does not return an error", func() {
				Expect(reconcileErr).NotTo(HaveOccurred())
			})

			It("returns an empty result", func() {
				Expect(reconcileResult).To(Equal(ctrl.Result{}))
			})

			It("does not create a new kpack image", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(0), "fakeClient Create was called When It should not have been")
			})

			It("should update the status conditions on CFBuild", func() {
				Expect(fakeClient.StatusCallCount()).To(Equal(1))
			})
		})

		When("on the unhappy path", func() {
			When("fetch KpackImage returns an error", func() {
				BeforeEach(func() {
					kpackImageError = errors.New("failing on purpose")
				})

				It("should return an error", func() {
					Expect(reconcileErr).To(HaveOccurred())
				})
			})

			When("fetch KpackImage returns a NotFoundError", func() {
				BeforeEach(func() {
					kpackImageError = apierrors.NewNotFound(schema.GroupResource{}, cfBuild.Name)
				})

				It("should NOT return any error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("kpack image status conditions for Type Succeeded is nil", func() {
				It("does not return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})

				It("returns an empty result", func() {
					Expect(reconcileResult).To(Equal(ctrl.Result{}))
				})
			})

			When("kpack image status conditions for Type Succeeded is UNKNOWN", func() {
				BeforeEach(func() {
					kpackImageError = nil
					setKpackImageStatus(kpackImage, kpackReadyConditionType, "Unknown")
				})

				It("does not return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})

				It("returns an empty result", func() {
					Expect(reconcileResult).To(Equal(ctrl.Result{}))
				})
			})

			When("kpack image status conditions for Type Succeeded is FALSE", func() {
				BeforeEach(func() {
					kpackImageError = nil
					setKpackImageStatus(kpackImage, kpackReadyConditionType, "False")
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

			When("kpack image status conditions for Type Succeeded is True and", func() {
				BeforeEach(func() {
					kpackImageError = nil
					setKpackImageStatus(kpackImage, kpackReadyConditionType, "True")
				})

				When("fetch kpack ServiceAccount returns an error", func() {
					BeforeEach(func() {
						kpackServiceAccountError = errors.New("kpackServiceAccountFetchError failing on purpose")
					})

					It("should return an error", func() {
						Expect(reconcileErr).To(HaveOccurred())
					})
				})

				When("compiling the DropletStatus and", func() {
					When("RegistryAuthFetcher fails while retrieving registry credentials", func() {
						var registryAuthFetcherError error

						BeforeEach(func() {
							registryAuthFetcherError = errors.New("registryAuthFetcherError failing on purpose")
							fakeRegistryAuthFetcher.Returns(nil, registryAuthFetcherError)
						})

						It("returns an error", func() {
							Expect(reconcileErr).To(HaveOccurred())
							Expect(reconcileErr).To(Equal(registryAuthFetcherError))
						})
					})

					When("ImageProcessFetcher fails while compiling Droplet Image fields", func() {
						var imageProcessFetcherError error

						BeforeEach(func() {
							imageProcessFetcherError = errors.New("imageProcessFetcherError failing on purpose")
							fakeImageProcessFetcher.Returns(nil, nil, imageProcessFetcherError)
						})

						It("returns an error", func() {
							Expect(reconcileErr).To(HaveOccurred())
							Expect(reconcileErr).To(Equal(imageProcessFetcherError))
						})
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
			})
		})
	})
})

func mockKpackImageObject(guid string, namespace string) *buildv1alpha2.Image {
	return &buildv1alpha2.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: namespace,
		},
		Spec: buildv1alpha2.ImageSpec{
			Tag:                "test-tag",
			Builder:            corev1.ObjectReference{},
			ServiceAccountName: "test-service-account",
			Source: corev1alpha1.SourceConfig{
				Registry: &corev1alpha1.Registry{
					Image:            "image-path",
					ImagePullSecrets: nil,
				},
			},
		},
	}
}

func setKpackImageStatus(kpackImage *buildv1alpha2.Image, conditionType string, conditionStatus string) {
	kpackImage.Status.Conditions = append(kpackImage.Status.Conditions, corev1alpha1.Condition{
		Type:   corev1alpha1.ConditionType(conditionType),
		Status: corev1.ConditionStatus(conditionStatus),
	})
}
