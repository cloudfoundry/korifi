package integration_test

import (
	"context"

	. "github.com/onsi/gomega/gstruct"

	servicesv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/services/v1alpha1"

	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFBuildReconciler", func() {
	const (
		succeededConditionType              = "Succeeded"
		kpackReadyConditionType             = "Ready"
		wellFormedRegistryCredentialsSecret = "image-registry-credentials"
	)

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

	eventuallyKpackImageShould := func(assertion func(*buildv1alpha2.Image, Gomega)) {
		Eventually(func(g Gomega) {
			kpackImage := new(buildv1alpha2.Image)
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}, kpackImage)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kpackImage.Spec.Build).ToNot(BeNil())
			assertion(kpackImage, g)
		}).Should(Succeed())
	}

	BeforeEach(func() {
		namespaceGUID = GenerateGUID()
		cfAppGUID = GenerateGUID()
		cfPackageGUID = GenerateGUID()

		beforeCtx := context.Background()

		newNamespace = BuildNamespaceObject(namespaceGUID)
		Expect(k8sClient.Create(beforeCtx, newNamespace)).To(Succeed())

		desiredCFApp = BuildCFAppCRObject(cfAppGUID, namespaceGUID)
		Expect(k8sClient.Create(beforeCtx, desiredCFApp)).To(Succeed())

		envVarSecret := BuildCFAppEnvVarsSecret(desiredCFApp.Name, namespaceGUID, map[string]string{
			"a_key": "a-val",
			"b_key": "b-val",
		})
		Expect(k8sClient.Create(context.Background(), envVarSecret)).To(Succeed())

		dockerRegistrySecret := BuildDockerRegistrySecret(wellFormedRegistryCredentialsSecret, namespaceGUID)
		Expect(k8sClient.Create(beforeCtx, dockerRegistrySecret)).To(Succeed())

		registryServiceAccountName := "kpack-service-account"
		registryServiceAccount := BuildServiceAccount(registryServiceAccountName, namespaceGUID, wellFormedRegistryCredentialsSecret)
		Expect(k8sClient.Create(beforeCtx, registryServiceAccount)).To(Succeed())
	})

	When("CFBuild status conditions are missing or unknown", func() {
		BeforeEach(func() {
			beforeCtx := context.Background()
			desiredCFPackage = BuildCFPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			Expect(k8sClient.Create(beforeCtx, desiredCFPackage)).To(Succeed())

			kpackSecret := BuildDockerRegistrySecret("source-registry-image-pull-secret", namespaceGUID)
			Expect(k8sClient.Create(beforeCtx, kpackSecret)).To(Succeed())
		})

		JustBeforeEach(func() {
			cfBuildGUID = GenerateGUID()
			desiredCFBuild = BuildCFBuildObject(cfBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
			Expect(k8sClient.Create(context.Background(), desiredCFBuild)).To(Succeed())
		})

		It("eventually reconciles to set the owner reference on the CFBuild", func() {
			Eventually(func() []metav1.OwnerReference {
				var createdCFBuild workloadsv1alpha1.CFBuild
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}, &createdCFBuild)
				if err != nil {
					return nil
				}
				return createdCFBuild.GetOwnerReferences()
			}).Should(ConsistOf(metav1.OwnerReference{
				APIVersion: workloadsv1alpha1.GroupVersion.Identifier(),
				Kind:       "CFApp",
				Name:       desiredCFApp.Name,
				UID:        desiredCFApp.UID,
			}))
		})

		It("creates a kpack image with the envvars set on it", func() {
			eventuallyKpackImageShould(func(kpackImage *buildv1alpha2.Image, g Gomega) {
				g.Expect(kpackImage.Spec.Build.Env).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"Name": Equal("a_key")}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal("b_key")}),
					MatchFields(IgnoreExtras, Fields{"Name": Equal("VCAP_SERVICES")}),
				))
			})
		})

		When("kpack image with CFBuild GUID doesn't exist", func() {
			It("eventually creates a Kpack Image", func() {
				testCtx := context.Background()
				kpackImageLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdKpackImage := new(buildv1alpha2.Image)
				Eventually(func() bool {
					err := k8sClient.Get(testCtx, kpackImageLookupKey, createdKpackImage)
					return err == nil
				}).Should(BeTrue(), "could not retrieve the kpack image")
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
				}).ShouldNot(BeEmpty(), "CFBuild status conditions were empty")
			})
		})

		When("the referenced app has a ServiceBinding and Secret", func() {
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

				serviceBinding1Name := "service-binding-1-name"
				serviceBinding1 = &servicesv1alpha1.CFServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-binding-1-guid",
						Namespace: newNamespace.Name,
						Labels: map[string]string{
							workloadsv1alpha1.CFAppGUIDLabelKey: desiredCFApp.Name,
						},
					},
					Spec: servicesv1alpha1.CFServiceBindingSpec{
						Name: &serviceBinding1Name,
						Service: corev1.ObjectReference{
							Kind:       "ServiceInstance",
							Name:       serviceInstance1.Name,
							APIVersion: "services.cloudfoundry.org/v1alpha1",
						},
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

				serviceBinding2Name := "service-binding-2-name"
				serviceBinding2 = &servicesv1alpha1.CFServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-binding-2-guid",
						Namespace: newNamespace.Name,
						Labels: map[string]string{
							workloadsv1alpha1.CFAppGUIDLabelKey: desiredCFApp.Name,
						},
					},
					Spec: servicesv1alpha1.CFServiceBindingSpec{
						Name: &serviceBinding2Name,
						Service: corev1.ObjectReference{
							Kind:       "ServiceInstance",
							Name:       serviceInstance2.Name,
							APIVersion: "services.cloudfoundry.org/v1alpha1",
						},
						AppRef: corev1.LocalObjectReference{
							Name: desiredCFApp.Name,
						},
					},
				}
				Expect(
					k8sClient.Create(ctx, serviceBinding2),
				).To(Succeed())

				createdServiceBinding1 := serviceBinding1.DeepCopy()
				createdServiceBinding1.Status.Binding.Name = secret1.Name
				meta.SetStatusCondition(&createdServiceBinding1.Status.Conditions, metav1.Condition{
					Type:    "BindingSecretAvailable",
					Status:  metav1.ConditionTrue,
					Reason:  "SecretFound",
					Message: "",
				})
				Expect(k8sClient.Status().Patch(ctx, createdServiceBinding1, client.MergeFrom(serviceBinding1))).To(Succeed())

				createdServiceBinding2 := serviceBinding2.DeepCopy()
				createdServiceBinding2.Status.Binding.Name = secret2.Name
				meta.SetStatusCondition(&createdServiceBinding2.Status.Conditions, metav1.Condition{
					Type:    "BindingSecretAvailable",
					Status:  metav1.ConditionTrue,
					Reason:  "SecretFound",
					Message: "",
				})
				Expect(k8sClient.Status().Patch(ctx, createdServiceBinding2, client.MergeFrom(serviceBinding2))).To(Succeed())
			})

			It("eventually creates a kpack image with the underlying secret mapped onto it", func() {
				eventuallyKpackImageShould(func(kpackImage *buildv1alpha2.Image, g Gomega) {
					g.Expect(kpackImage.Spec.Build.Services).To(ConsistOf(
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

			It("sets the VCAP_SERVICES env var in the image", func() {
				eventuallyKpackImageShould(func(kpackImage *buildv1alpha2.Image, g Gomega) {
					g.Expect(kpackImage.Spec.Build.Env).To(ContainElements(
						MatchFields(IgnoreExtras, Fields{"Name": Equal("VCAP_SERVICES")}),
					))
				})
			})

			It("sets the VCAP_SERVICES key in the env var secret", func() {
				textCtx := context.Background()
				envVarSecret := new(corev1.Secret)

				Eventually(func(g Gomega) {
					err := k8sClient.Get(textCtx, types.NamespacedName{Name: desiredCFApp.Spec.EnvSecretName, Namespace: namespaceGUID}, envVarSecret)
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(envVarSecret.Data).To(HaveKey("VCAP_SERVICES"))
				}).Should(Succeed())
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
				}).ShouldNot(BeEmpty(), "CFBuild status conditions were empty")
			})
		})
	})

	When("CFBuild status conditions for Staging is True and others are unknown", func() {
		BeforeEach(func() {
			desiredCFPackage = BuildCFPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			desiredCFPackage.Spec.Source.Registry.ImagePullSecrets = []corev1.LocalObjectReference{{Name: wellFormedRegistryCredentialsSecret}}
			Expect(k8sClient.Create(context.Background(), desiredCFPackage)).To(Succeed())

			cfBuildGUID = GenerateGUID()
			desiredCFBuild = BuildCFBuildObject(cfBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
			Expect(k8sClient.Create(context.Background(), desiredCFBuild)).To(Succeed())
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
				}).Should(BeTrue(), "could not retrieve the kpack image")
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
				}).Should(BeTrue())
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
				}).Should(BeTrue(), "could not retrieve the kpack image")
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
				}).Should(BeTrue())
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
				}).ShouldNot(BeNil(), "BuildStatusDroplet was nil on CFBuild")
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
