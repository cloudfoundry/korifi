package integration_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFBuildReconciler Integration Tests", func() {
	const (
		stagingConditionType                = "Staging"
		succeededConditionType              = "Succeeded"
		wellFormedRegistryCredentialsSecret = "image-registry-credentials"
	)

	var (
		namespaceGUID    string
		cfAppGUID        string
		cfPackageGUID    string
		cfBuildGUID      string
		namespace        *corev1.Namespace
		desiredCFApp     *korifiv1alpha1.CFApp
		desiredCFPackage *korifiv1alpha1.CFPackage
		desiredCFBuild   *korifiv1alpha1.CFBuild
	)

	eventuallyBuildWorkloadShould := func(assertion func(*korifiv1alpha1.BuildWorkload, Gomega)) {
		EventuallyWithOffset(1, func(g Gomega) {
			workload := new(korifiv1alpha1.BuildWorkload)
			lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
			g.Expect(k8sClient.Get(context.Background(), lookupKey, workload)).To(Succeed())
			assertion(workload, g)
		}).Should(Succeed())
	}

	BeforeEach(func() {
		namespaceGUID = PrefixedGUID("namespace")
		cfAppGUID = PrefixedGUID("cf-app")
		cfPackageGUID = PrefixedGUID("cf-package")

		beforeCtx := context.Background()

		namespace = BuildNamespaceObject(namespaceGUID)
		Expect(k8sClient.Create(beforeCtx, namespace)).To(Succeed())

		desiredCFApp = BuildCFAppCRObject(cfAppGUID, namespaceGUID)
		Expect(k8sClient.Create(beforeCtx, desiredCFApp)).To(Succeed())

		Eventually(func() string {
			actualCFApp := &korifiv1alpha1.CFApp{}
			err := k8sClient.Get(beforeCtx, types.NamespacedName{Name: cfAppGUID, Namespace: namespaceGUID}, actualCFApp)
			if err != nil {
				return ""
			}
			return actualCFApp.Status.VCAPServicesSecretName
		}).ShouldNot(BeEmpty())

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

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	When("CFBuild status conditions are missing or unknown", func() {
		BeforeEach(func() {
			ctx := context.Background()
			desiredCFPackage = BuildCFPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			Expect(k8sClient.Create(ctx, desiredCFPackage)).To(Succeed())

			kpackSecret := BuildDockerRegistrySecret("source-registry-image-pull-secret", namespaceGUID)
			Expect(k8sClient.Create(ctx, kpackSecret)).To(Succeed())
		})

		JustBeforeEach(func() {
			cfBuildGUID = PrefixedGUID("cf-build")
			desiredCFBuild = BuildCFBuildObject(cfBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
			Expect(k8sClient.Create(context.Background(), desiredCFBuild)).To(Succeed())
		})

		It("reconciles to set the owner reference on the CFBuild", func() {
			Eventually(func(g Gomega) []metav1.OwnerReference {
				var createdCFBuild korifiv1alpha1.CFBuild
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				g.Expect(k8sClient.Get(context.Background(), lookupKey, &createdCFBuild)).To(Succeed())
				return createdCFBuild.GetOwnerReferences()
			}).Should(ConsistOf(metav1.OwnerReference{
				APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				Kind:       "CFApp",
				Name:       desiredCFApp.Name,
				UID:        desiredCFApp.UID,
			}))
		})

		It("creates a BuildWorkload with the buildRef, source, and env set", func() {
			createdCFApp := &korifiv1alpha1.CFApp{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: cfAppGUID, Namespace: namespaceGUID}, createdCFApp)).To(Succeed())

			eventuallyBuildWorkloadShould(func(workload *korifiv1alpha1.BuildWorkload, g Gomega) {
				g.Expect(workload.Spec.BuildRef.Name).To(Equal(cfBuildGUID))
				g.Expect(workload.Spec.Source).To(Equal(desiredCFPackage.Spec.Source))
				g.Expect(workload.Spec.Env).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("a_key"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
								"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
									"Name": Equal(createdCFApp.Spec.EnvSecretName),
								}),
								"Key": Equal("a_key"),
							})),
						})),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("b_key"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
								"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
									"Name": Equal(createdCFApp.Spec.EnvSecretName),
								}),
								"Key": Equal("b_key"),
							})),
						})),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("VCAP_SERVICES"),
						"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
							"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
								"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
									"Name": Equal(createdCFApp.Status.VCAPServicesSecretName),
								}),
								"Key": Equal("VCAP_SERVICES"),
							})),
						})),
					}),
				))
			})
		})

		When("BuildWorkload with CFBuild GUID doesn't exist", func() {
			It("creates a BuildWorkload owned by the CFBuild", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdWorkload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func() error {
					return k8sClient.Get(context.Background(), lookupKey, createdWorkload)
				}).Should(Succeed())
				Expect(createdWorkload.GetOwnerReferences()).To(ConsistOf(metav1.OwnerReference{
					UID:        desiredCFBuild.UID,
					Kind:       "CFBuild",
					APIVersion: "korifi.cloudfoundry.org/v1alpha1",
					Name:       desiredCFBuild.Name,
				}))
			})

			It("sets the status conditions on CFBuild", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(korifiv1alpha1.CFBuild)
				Eventually(func(g Gomega) []metav1.Condition {
					g.Expect(k8sClient.Get(context.Background(), lookupKey, createdCFBuild)).To(Succeed())
					return createdCFBuild.Status.Conditions
				}).ShouldNot(BeEmpty())

				stagingCondition := meta.FindStatusCondition(createdCFBuild.Status.Conditions, stagingConditionType)
				succeededCondition := meta.FindStatusCondition(createdCFBuild.Status.Conditions, succeededConditionType)
				Expect(stagingCondition.Status).To(Equal(metav1.ConditionTrue))
				Expect(succeededCondition.Status).To(Equal(metav1.ConditionUnknown))
			})
		})

		When("the referenced app has a ServiceBinding and Secret", func() {
			var (
				secret1          *corev1.Secret
				secret2          *corev1.Secret
				serviceInstance1 *korifiv1alpha1.CFServiceInstance
				serviceInstance2 *korifiv1alpha1.CFServiceInstance
				serviceBinding1  *korifiv1alpha1.CFServiceBinding
				serviceBinding2  *korifiv1alpha1.CFServiceBinding
			)

			BeforeEach(func() {
				ctx := context.Background()

				secret1Data := map[string]string{
					"foo": "bar",
				}
				secret1 = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret1",
						Namespace: namespace.Name,
					},
					StringData: secret1Data,
				}
				Expect(
					k8sClient.Create(ctx, secret1),
				).To(Succeed())

				serviceInstance1 = &korifiv1alpha1.CFServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-instance-1-guid",
						Namespace: namespace.Name,
					},
					Spec: korifiv1alpha1.CFServiceInstanceSpec{
						DisplayName: "service-instance-1-name",
						SecretName:  secret1.Name,
						Type:        "user-provided",
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
				serviceBinding1 = &korifiv1alpha1.CFServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-binding-1-guid",
						Namespace: namespace.Name,
						Labels: map[string]string{
							korifiv1alpha1.CFAppGUIDLabelKey: desiredCFApp.Name,
						},
					},
					Spec: korifiv1alpha1.CFServiceBindingSpec{
						DisplayName: &serviceBinding1Name,
						Service: corev1.ObjectReference{
							Kind:       "ServiceInstance",
							Name:       serviceInstance1.Name,
							APIVersion: "korifi.cloudfoundry.org/v1alpha1",
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
						Namespace: namespace.Name,
					},
					StringData: secret2Data,
				}
				Expect(
					k8sClient.Create(ctx, secret2),
				).To(Succeed())

				serviceInstance2 = &korifiv1alpha1.CFServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-instance-2-guid",
						Namespace: namespace.Name,
					},
					Spec: korifiv1alpha1.CFServiceInstanceSpec{
						DisplayName: "service-instance-2-name",
						SecretName:  secret2.Name,
						Type:        "user-provided",
						Tags:        []string{},
					},
				}
				Expect(
					k8sClient.Create(ctx, serviceInstance2),
				).To(Succeed())

				serviceBinding2Name := "service-binding-2-name"
				serviceBinding2 = &korifiv1alpha1.CFServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-binding-2-guid",
						Namespace: namespace.Name,
						Labels: map[string]string{
							korifiv1alpha1.CFAppGUIDLabelKey: desiredCFApp.Name,
						},
					},
					Spec: korifiv1alpha1.CFServiceBindingSpec{
						DisplayName: &serviceBinding2Name,
						Service: corev1.ObjectReference{
							Kind:       "ServiceInstance",
							Name:       serviceInstance2.Name,
							APIVersion: "korifi.cloudfoundry.org/v1alpha1",
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

			It("creates a BuildWorkload with the underlying secret mapped onto it", func() {
				eventuallyBuildWorkloadShould(func(workload *korifiv1alpha1.BuildWorkload, g Gomega) {
					g.Expect(workload.Spec.Services).To(ConsistOf(
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
				createdCFApp := &korifiv1alpha1.CFApp{}
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: cfAppGUID, Namespace: namespaceGUID}, createdCFApp)).To(Succeed())

				eventuallyBuildWorkloadShould(func(workload *korifiv1alpha1.BuildWorkload, g Gomega) {
					g.Expect(workload.Spec.Env).To(ContainElements(
						MatchFields(IgnoreExtras, Fields{
							"Name": Equal("VCAP_SERVICES"),
							"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
								"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
									"Key": Equal("VCAP_SERVICES"),
									"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
										"Name": Equal(createdCFApp.Status.VCAPServicesSecretName),
									}),
								})),
							})),
						}),
					))
				})
			})
		})

		When("a BuildWorkload with CFBuild GUID already exists", func() {
			var (
				newCFBuildGUID        string
				existingBuildWorkload *korifiv1alpha1.BuildWorkload
				newCFBuild            *korifiv1alpha1.CFBuild
			)

			BeforeEach(func() {
				ctx := context.Background()
				newCFBuildGUID = PrefixedGUID("new-cf-build")
				existingBuildWorkload = &korifiv1alpha1.BuildWorkload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      newCFBuildGUID,
						Namespace: namespaceGUID,
					},
					Spec: korifiv1alpha1.BuildWorkloadSpec{
						Source: korifiv1alpha1.PackageSource{
							Registry: korifiv1alpha1.Registry{
								Image:            "not-an-image",
								ImagePullSecrets: nil,
							},
						},
					},
				}
				newCFBuild = BuildCFBuildObject(newCFBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)

				Expect(k8sClient.Create(ctx, existingBuildWorkload)).To(Succeed())
				Expect(k8sClient.Create(ctx, newCFBuild)).To(Succeed())
			})

			It("sets the status conditions on CFBuild", func() {
				lookupKey := types.NamespacedName{Name: newCFBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(korifiv1alpha1.CFBuild)
				Eventually(func(g Gomega) []metav1.Condition {
					g.Expect(
						k8sClient.Get(context.Background(), lookupKey, createdCFBuild),
					).To(Succeed())
					return createdCFBuild.Status.Conditions
				}).ShouldNot(BeEmpty())

				stagingCondition := meta.FindStatusCondition(createdCFBuild.Status.Conditions, stagingConditionType)
				succeededCondition := meta.FindStatusCondition(createdCFBuild.Status.Conditions, succeededConditionType)
				Expect(stagingCondition.Status).To(Equal(metav1.ConditionTrue))
				Expect(succeededCondition.Status).To(Equal(metav1.ConditionUnknown))
			})
		})
	})

	When("CFBuild status conditions Staging=True and others are unknown", func() {
		BeforeEach(func() {
			desiredCFPackage = BuildCFPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			desiredCFPackage.Spec.Source.Registry.ImagePullSecrets = []corev1.LocalObjectReference{{Name: wellFormedRegistryCredentialsSecret}}
			Expect(k8sClient.Create(context.Background(), desiredCFPackage)).To(Succeed())

			cfBuildGUID = PrefixedGUID("cf-build")
			desiredCFBuild = BuildCFBuildObject(cfBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
			Expect(k8sClient.Create(context.Background(), desiredCFBuild)).To(Succeed())
		})

		When("the BuildWorkload failed", func() {
			BeforeEach(func() {
				testCtx := context.Background()
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				workload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func() error {
					return k8sClient.Get(testCtx, lookupKey, workload)
				}).Should(Succeed())
				setBuildWorkloadStatus(workload, succeededConditionType, metav1.ConditionFalse)
				Expect(k8sClient.Status().Update(testCtx, workload)).To(Succeed())
			})

			It("sets the CFBuild status condition Succeeded = False", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(korifiv1alpha1.CFBuild)
				Eventually(func(g Gomega) metav1.ConditionStatus {
					g.Expect(k8sClient.Get(context.Background(), lookupKey, createdCFBuild)).To(Succeed())
					return meta.FindStatusCondition(createdCFBuild.Status.Conditions, succeededConditionType).Status
				}).Should(Equal(metav1.ConditionFalse))
			})
		})

		When("the BuildWorkload finished successfully", func() {
			const (
				buildImageRef       = "some-org/my-image@sha256:some-sha"
				imagePullSecretName = "image-pull-s3cr37"
				buildStack          = "cflinuxfs3"
			)

			var (
				returnedProcessTypes []korifiv1alpha1.ProcessType
				returnedPorts        []int32
			)

			BeforeEach(func() {
				ctx := context.Background()

				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				workload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func() error {
					return k8sClient.Get(ctx, lookupKey, workload)
				}).Should(Succeed())

				returnedPorts = []int32{42}
				returnedProcessTypes = []korifiv1alpha1.ProcessType{
					{
						Type:    "web",
						Command: "run-stuff",
					},
				}

				setBuildWorkloadStatus(workload, succeededConditionType, "True")
				workload.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{
					Registry: korifiv1alpha1.Registry{
						Image:            buildImageRef,
						ImagePullSecrets: []corev1.LocalObjectReference{{Name: imagePullSecretName}},
					},
					Stack:        buildStack,
					Ports:        returnedPorts,
					ProcessTypes: returnedProcessTypes,
				}
				Expect(k8sClient.Status().Update(ctx, workload)).To(Succeed())
			})

			It("sets the CFBuild status condition Succeeded = True", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(korifiv1alpha1.CFBuild)

				Eventually(func(g Gomega) metav1.ConditionStatus {
					g.Expect(k8sClient.Get(context.Background(), lookupKey, createdCFBuild)).To(Succeed())
					return meta.FindStatusCondition(createdCFBuild.Status.Conditions, succeededConditionType).Status
				}).Should(Equal(metav1.ConditionTrue))
			})

			It("sets CFBuild.status.droplet", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				createdCFBuild := new(korifiv1alpha1.CFBuild)
				Eventually(func(g Gomega) *korifiv1alpha1.BuildDropletStatus {
					g.Expect(k8sClient.Get(context.Background(), lookupKey, createdCFBuild)).To(Succeed())
					return createdCFBuild.Status.Droplet
				}).ShouldNot(BeNil())

				Expect(createdCFBuild.Status.Droplet.Registry.Image).To(Equal(buildImageRef))
				Expect(createdCFBuild.Status.Droplet.Registry.ImagePullSecrets).To(ConsistOf(corev1.LocalObjectReference{Name: imagePullSecretName}))
				Expect(createdCFBuild.Status.Droplet.Stack).To(Equal(buildStack))
				Expect(createdCFBuild.Status.Droplet.ProcessTypes).To(Equal(returnedProcessTypes))
				Expect(createdCFBuild.Status.Droplet.Ports).To(Equal(returnedPorts))
			})
		})
	})
})

func setBuildWorkloadStatus(workload *korifiv1alpha1.BuildWorkload, conditionType string, conditionStatus metav1.ConditionStatus) {
	meta.SetStatusCondition(&workload.Status.Conditions, metav1.Condition{
		Type:   conditionType,
		Status: conditionStatus,
		Reason: "shrug",
	})
}
