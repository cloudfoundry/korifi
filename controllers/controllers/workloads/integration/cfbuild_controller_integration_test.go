package integration_test

import (
	"context"
	"time"

	. "github.com/onsi/gomega/gstruct"

	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CFBuildReconciler", func() {
	const (
		succeededConditionType              = "Succeeded"
		kpackReadyConditionType             = "Ready"
		wellFormedRegistryCredentialsSecret = "image-registry-credentials"
	)

	When("CFBuild status conditions are missing or unknown", func() {
		var (
			namespaceGUID    string
			cfAppGUID        string
			cfPackageGUID    string
			cfBuildGUID      string
			newNamespace     *corev1.Namespace
			desiredCFApp     *workloadsv1alpha1.CFApp
			desiredCFPackage *workloadsv1alpha1.CFPackage
			desiredCFBuild   *workloadsv1alpha1.CFBuild
		)

		BeforeEach(func() {
			beforeCtx := context.Background()

			namespaceGUID = GenerateGUID()
			newNamespace = BuildNamespaceObject(namespaceGUID)
			Expect(
				k8sClient.Create(beforeCtx, newNamespace),
			).To(Succeed())
			DeferCleanup(func() {
				k8sClient.Delete(context.Background(), newNamespace) //nolint
			})

			cfAppGUID = GenerateGUID()
			desiredCFApp = BuildCFAppCRObject(cfAppGUID, namespaceGUID)
			Expect(
				k8sClient.Create(beforeCtx, desiredCFApp),
			).To(Succeed())

			cfPackageGUID = GenerateGUID()
			desiredCFPackage = BuildCFPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			Expect(
				k8sClient.Create(beforeCtx, desiredCFPackage),
			).To(Succeed())
		})

		JustBeforeEach(func() {
			beforeCtx := context.Background()

			cfBuildGUID = GenerateGUID()
			desiredCFBuild = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfBuildGUID,
					Namespace: namespaceGUID,
				},
				Spec: workloadsv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: cfPackageGUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: cfAppGUID,
					},
					StagingMemoryMB: 1024,
					StagingDiskMB:   1024,
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: nil,
							Stack:      "",
						},
					},
				},
			}
			Expect(
				k8sClient.Create(beforeCtx, desiredCFBuild),
			).To(Succeed())
		})

		It("eventually reconciles to set the owner reference on the CFBuild", func() {
			Eventually(func() []metav1.OwnerReference {
				var createdCFBuild workloadsv1alpha1.CFBuild
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}, &createdCFBuild)
				if err != nil {
					return nil
				}
				return createdCFBuild.GetOwnerReferences()
			}, 5*time.Second).Should(ConsistOf(metav1.OwnerReference{
				APIVersion: workloadsv1alpha1.GroupVersion.Identifier(),
				Kind:       "CFApp",
				Name:       desiredCFApp.Name,
				UID:        desiredCFApp.UID,
			}))
		})

		When("kpack image with CFBuild GUID doesn't exist", func() {
			It("eventually creates a Kpack Image", func() {
				testCtx := context.Background()
				kpackImageLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdKpackImage := new(buildv1alpha2.Image)
				Eventually(func() bool {
					err := k8sClient.Get(testCtx, kpackImageLookupKey, createdKpackImage)
					return err == nil
				}, 10*time.Second, 250*time.Millisecond).Should(BeTrue(), "could not retrieve the kpack image")
				kpackImageTag := "image/registry/tag" + "/" + cfBuildGUID
				Expect(createdKpackImage.Spec.Tag).To(Equal(kpackImageTag))
				Expect(createdKpackImage.GetOwnerReferences()).To(ConsistOf(metav1.OwnerReference{
					UID:        desiredCFBuild.UID,
					Kind:       "CFBuild",
					APIVersion: "workloads.cloudfoundry.org/v1alpha1",
					Name:       desiredCFBuild.Name,
				}))
				Expect(createdKpackImage.Spec.Builder.Name).To(Equal("cf-kpack-builder"))
				Expect(k8sClient.Delete(testCtx, createdKpackImage)).To(Succeed())
			})

			It("eventually sets the status conditions on CFBuild", func() {
				testCtx := context.Background()
				cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(workloadsv1alpha1.CFBuild)
				Eventually(func() []metav1.Condition {
					err := k8sClient.Get(testCtx, cfBuildLookupKey, createdCFBuild)
					if err != nil {
						return nil
					}
					return createdCFBuild.Status.Conditions
				}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "CFBuild status conditions were empty")
			})
		})

		FWhen("the referenced app has a ServiceBinding and Secret", func() {
			var (
				secret1          *corev1.Secret
				secret2          *corev1.Secret
				serviceInstance1 *servicesv1alpha1.CFServiceInstance
				serviceInstance2 *servicesv1alpha1.CFServiceInstance
				serviceBinding1  *servicesv1alpha1.CFServiceBinding
				serviceBinding2  *servicesv1alpha1.CFServiceBinding
			)

			BeforeEach(func() {
				ctx := context.Background()

				secret1Data := map[string]string{
					"foo": "bar",
				}
				secret1 = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret1",
						Namespace: newNamespace.Name,
					},
					StringData: secret1Data,
				}
				Expect(
					k8sClient.Create(ctx, secret1),
				).To(Succeed())

				serviceInstance1 = &servicesv1alpha1.CFServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-instance-1-guid",
						Namespace: newNamespace.Name,
					},
					Spec: servicesv1alpha1.CFServiceInstanceSpec{
						Name:       "service-instance-1-name",
						SecretName: secret1.Name,
						Type:       "user-provided",
						Tags: []string{
							"tag1",
							"tag2",
						},
					},
				}
				Expect(
					k8sClient.Create(ctx, serviceInstance1),
				).To(Succeed())

				serviceBinding1 = &servicesv1alpha1.CFServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-binding-1-guid",
						Namespace: newNamespace.Name,
						Labels: map[string]string{
							workloadsv1alpha1.CFAppGUIDLabelKey: desiredCFApp.Name,
						},
					},
					Spec: servicesv1alpha1.CFServiceBindingSpec{
						Name: "service-binding-1-name",
						Service: corev1.ObjectReference{
							Kind:       "ServiceInstance",
							Name:       serviceInstance1.Name,
							APIVersion: "services.cloudfoundry.org/v1alpha1",
						},
						SecretName: secret1.Name,
						AppRef: corev1.LocalObjectReference{
							Name: desiredCFApp.Name,
						},
					},
				}
				Expect(
					k8sClient.Create(ctx, serviceBinding1),
				).To(Succeed())

				secret2Data := map[string]string{
					"key1": "value1",
					"key2": "value2",
				}
				secret2 = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret2",
						Namespace: newNamespace.Name,
					},
					StringData: secret2Data,
				}
				Expect(
					k8sClient.Create(ctx, secret2),
				).To(Succeed())

				serviceInstance2 = &servicesv1alpha1.CFServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-instance-2-guid",
						Namespace: newNamespace.Name,
					},
					Spec: servicesv1alpha1.CFServiceInstanceSpec{
						Name:       "service-instance-2-name",
						SecretName: secret2.Name,
						Type:       "user-provided",
						Tags:       []string{},
					},
				}
				Expect(
					k8sClient.Create(ctx, serviceInstance2),
				).To(Succeed())

				serviceBinding2 = &servicesv1alpha1.CFServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-binding-2-guid",
						Namespace: newNamespace.Name,
						Labels: map[string]string{
							workloadsv1alpha1.CFAppGUIDLabelKey: desiredCFApp.Name,
						},
					},
					Spec: servicesv1alpha1.CFServiceBindingSpec{
						Name: "",
						Service: corev1.ObjectReference{
							Kind:       "ServiceInstance",
							Name:       serviceInstance2.Name,
							APIVersion: "services.cloudfoundry.org/v1alpha1",
						},
						SecretName: secret2.Name,
						AppRef: corev1.LocalObjectReference{
							Name: desiredCFApp.Name,
						},
					},
				}
				Expect(
					k8sClient.Create(ctx, serviceBinding2),
				).To(Succeed())

				// sleep to encourage the CFBuildController cache to have the secrets around before the CFBuild is created
				time.Sleep(time.Millisecond * 50)
			})

			It("eventually creates a kpack image with the underlying secret mapped onto it", func() {
				testCtx := context.Background()
				createdKpackImage := new(buildv1alpha2.Image)
				Eventually(func() int {
					err := k8sClient.Get(testCtx, types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}, createdKpackImage)
					if err != nil || createdKpackImage.Spec.Build == nil {
						return 0
					}
					return len(createdKpackImage.Spec.Build.Services)
				}, 10*time.Second, 250*time.Millisecond).Should(Equal(2), "ServiceBinding Secrets did not show up on kpack image")

				Expect(createdKpackImage.Spec.Build.Services).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name":       Equal(secret1.Name),
						"Kind":       Equal("Secret"),
						"APIVersion": Equal("v1"),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":       Equal(secret2.Name),
						"Kind":       Equal("Secret"),
						"APIVersion": Equal("v1"),
					}),
				))
			})

		})

		When("kpack image with CFBuild GUID already exists", func() {
			var (
				newCFBuildGUID     string
				existingKpackImage *buildv1alpha2.Image
				newCFBuild         *workloadsv1alpha1.CFBuild
			)

			BeforeEach(func() {
				beforeCtx := context.Background()
				newCFBuildGUID = GenerateGUID()
				existingKpackImage = &buildv1alpha2.Image{
					ObjectMeta: metav1.ObjectMeta{
						Name:      newCFBuildGUID,
						Namespace: namespaceGUID,
					},
					Spec: buildv1alpha2.ImageSpec{
						Tag: "my-tag-string",
						Builder: corev1.ObjectReference{
							Name: "my-builder",
						},
						ServiceAccountName: "my-service-account",
						Source: corev1alpha1.SourceConfig{
							Registry: &corev1alpha1.Registry{
								Image:            "not-an-image",
								ImagePullSecrets: nil,
							},
						},
					},
				}
				Expect(k8sClient.Create(beforeCtx, existingKpackImage)).To(Succeed())
				newCFBuild = BuildCFBuildObject(newCFBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
				Expect(k8sClient.Create(beforeCtx, newCFBuild)).To(Succeed())
			})

			AfterEach(func() {
				afterCtx := context.Background()
				Expect(k8sClient.Delete(afterCtx, existingKpackImage)).To(Succeed())
				Expect(k8sClient.Delete(afterCtx, newCFBuild)).To(Succeed())
			})

			It("eventually sets the status conditions on CFBuild", func() {
				testCtx := context.Background()
				cfBuildLookupKey := types.NamespacedName{Name: newCFBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(workloadsv1alpha1.CFBuild)
				Eventually(func() []metav1.Condition {
					err := k8sClient.Get(testCtx, cfBuildLookupKey, createdCFBuild)
					if err != nil {
						return nil
					}
					return createdCFBuild.Status.Conditions
				}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "CFBuild status conditions were empty")
			})
		})
	})

	When("CFBuild status conditions for Staging is True and others are unknown", func() {
		var (
			namespaceGUID    string
			cfAppGUID        string
			cfPackageGUID    string
			cfBuildGUID      string
			newNamespace     *corev1.Namespace
			desiredCFApp     *workloadsv1alpha1.CFApp
			desiredCFPackage *workloadsv1alpha1.CFPackage
			desiredCFBuild   *workloadsv1alpha1.CFBuild
		)

		BeforeEach(func() {
			namespaceGUID = GenerateGUID()
			cfAppGUID = GenerateGUID()
			cfPackageGUID = GenerateGUID()
			cfBuildGUID = GenerateGUID()

			beforeCtx := context.Background()

			newNamespace = BuildNamespaceObject(namespaceGUID)
			Expect(k8sClient.Create(beforeCtx, newNamespace)).To(Succeed())

			desiredCFApp = BuildCFAppCRObject(cfAppGUID, namespaceGUID)
			Expect(k8sClient.Create(beforeCtx, desiredCFApp)).To(Succeed())

			dockerRegistrySecret := BuildDockerRegistrySecret(wellFormedRegistryCredentialsSecret, namespaceGUID)
			Expect(k8sClient.Create(beforeCtx, dockerRegistrySecret)).To(Succeed())

			registryServiceAccountName := "kpack-service-account"
			registryServiceAccount := BuildServiceAccount(registryServiceAccountName, namespaceGUID, wellFormedRegistryCredentialsSecret)
			Expect(k8sClient.Create(beforeCtx, registryServiceAccount)).To(Succeed())

			desiredCFPackage = BuildCFPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			desiredCFPackage.Spec.Source.Registry.ImagePullSecrets = []corev1.LocalObjectReference{{Name: wellFormedRegistryCredentialsSecret}}
			Expect(k8sClient.Create(beforeCtx, desiredCFPackage)).To(Succeed())

			desiredCFBuild = BuildCFBuildObject(cfBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
			Expect(k8sClient.Create(beforeCtx, desiredCFBuild)).To(Succeed())
		})

		AfterEach(func() {
			afterCtx := context.Background()
			Expect(k8sClient.Delete(afterCtx, desiredCFApp)).To(Succeed())
			Expect(k8sClient.Delete(afterCtx, desiredCFPackage)).To(Succeed())
			Expect(k8sClient.Delete(afterCtx, desiredCFBuild)).To(Succeed())
			Expect(k8sClient.Delete(afterCtx, newNamespace)).To(Succeed())
		})

		When("kpack image status condition for Type Succeeded is False", func() {
			BeforeEach(func() {
				testCtx := context.Background()
				kpackImageLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdKpackImage := new(buildv1alpha2.Image)
				Eventually(func() bool {
					err := k8sClient.Get(testCtx, kpackImageLookupKey, createdKpackImage)
					return err == nil
				}, 10*time.Second, 250*time.Millisecond).Should(BeTrue(), "could not retrieve the kpack image")
				setKpackImageStatus(createdKpackImage, kpackReadyConditionType, "False")
				Expect(k8sClient.Status().Update(testCtx, createdKpackImage)).To(Succeed())
			})

			It("eventually sets the status condition for Type Succeeded on CFBuild to False", func() {
				testCtx := context.Background()
				cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(workloadsv1alpha1.CFBuild)
				Eventually(func() bool {
					err := k8sClient.Get(testCtx, cfBuildLookupKey, createdCFBuild)
					if err != nil {
						return false
					}
					return meta.IsStatusConditionFalse(createdCFBuild.Status.Conditions, succeededConditionType)
				}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())
			})
		})

		When("kpack image has built successfully", func() {
			const (
				kpackBuildImageRef    = "some-org/my-image@sha256:some-sha"
				kpackImageLatestStack = "cflinuxfs3"
			)

			var (
				returnedProcessTypes []workloadsv1alpha1.ProcessType
				returnedPorts        []int32
			)

			BeforeEach(func() {
				testCtx := context.Background()

				// Fill out fake ImageProcessFetcher
				returnedProcessTypes = []workloadsv1alpha1.ProcessType{{Type: "web", Command: "my-command"}, {Type: "db", Command: "my-command2"}}
				returnedPorts = []int32{8080, 8443}
				fakeImageProcessFetcher.Returns(
					returnedProcessTypes,
					returnedPorts,
					nil,
				)

				kpackImageLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdKpackImage := new(buildv1alpha2.Image)
				Eventually(func() bool {
					err := k8sClient.Get(testCtx, kpackImageLookupKey, createdKpackImage)
					return err == nil
				}, 10*time.Second, 250*time.Millisecond).Should(BeTrue(), "could not retrieve the kpack image")
				setKpackImageStatus(createdKpackImage, kpackReadyConditionType, "True")
				createdKpackImage.Status.LatestImage = kpackBuildImageRef
				createdKpackImage.Status.LatestStack = kpackImageLatestStack
				Expect(k8sClient.Status().Update(testCtx, createdKpackImage)).To(Succeed())
			})

			It("eventually sets the status condition for Type Succeeded on CFBuild to True", func() {
				testCtx := context.Background()
				cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(workloadsv1alpha1.CFBuild)

				Eventually(func() bool {
					err := k8sClient.Get(testCtx, cfBuildLookupKey, createdCFBuild)
					if err != nil {
						return false
					}
					return meta.IsStatusConditionTrue(createdCFBuild.Status.Conditions, succeededConditionType)
				}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())
			})

			It("eventually sets BuildStatusDroplet object", func() {
				testCtx := context.Background()
				cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(workloadsv1alpha1.CFBuild)
				Eventually(func() *workloadsv1alpha1.BuildDropletStatus {
					err := k8sClient.Get(testCtx, cfBuildLookupKey, createdCFBuild)
					if err != nil {
						return nil
					}
					return createdCFBuild.Status.BuildDropletStatus
				}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeNil(), "BuildStatusDroplet was nil on CFBuild")
				Expect(fakeImageProcessFetcher.CallCount()).NotTo(Equal(0), "Build Controller imageProcessFetcher was not called")
				Expect(createdCFBuild.Status.BuildDropletStatus.Registry.Image).To(Equal(kpackBuildImageRef), "droplet registry image does not match kpack image latestImage")
				Expect(createdCFBuild.Status.BuildDropletStatus.Stack).To(Equal(kpackImageLatestStack), "droplet stack does not match kpack image latestStack")
				Expect(createdCFBuild.Status.BuildDropletStatus.Registry.ImagePullSecrets).To(Equal(desiredCFPackage.Spec.Source.Registry.ImagePullSecrets))
				Expect(createdCFBuild.Status.BuildDropletStatus.ProcessTypes).To(Equal(returnedProcessTypes))
				Expect(createdCFBuild.Status.BuildDropletStatus.Ports).To(Equal(returnedPorts))
			})
		})
	})
})

func setKpackImageStatus(kpackImage *buildv1alpha2.Image, conditionType string, conditionStatus string) {
	kpackImage.Status.Conditions = append(kpackImage.Status.Conditions, corev1alpha1.Condition{
		Type:   corev1alpha1.ConditionType(conditionType),
		Status: corev1.ConditionStatus(conditionStatus),
	})
}
