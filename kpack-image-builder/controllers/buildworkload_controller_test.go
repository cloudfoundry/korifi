package controllers_test

import (
	"encoding/base64"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("BuildWorkloadReconciler", func() {
	const (
		wellFormedRegistryCredentialsSecret = "image-registry-credentials"
	)

	var (
		namespaceGUID  string
		cfBuildGUID    string
		clusterBuilder *buildv1alpha2.ClusterBuilder
		buildWorkload  *korifiv1alpha1.BuildWorkload
		source         korifiv1alpha1.PackageSource
		env            []corev1.EnvVar
		services       []corev1.ObjectReference
		reconcilerName string
		buildpacks     []string
	)

	BeforeEach(func() {
		reconcilerName = "kpack-image-builder"
		namespaceGUID = PrefixedGUID("namespace")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceGUID}})).To(Succeed())

		dockerRegistrySecret := buildDockerRegistrySecret(wellFormedRegistryCredentialsSecret, namespaceGUID)
		Expect(k8sClient.Create(ctx, dockerRegistrySecret)).To(Succeed())

		registryServiceAccount := buildServiceAccount("builder-service-account", namespaceGUID, wellFormedRegistryCredentialsSecret)
		Expect(k8sClient.Create(ctx, registryServiceAccount)).To(Succeed())

		clusterBuilder = &buildv1alpha2.ClusterBuilder{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cf-kpack-builder",
			},
		}
		Expect(k8sClient.Create(ctx, clusterBuilder)).To(Succeed())

		Expect(k8s.Patch(ctx, k8sClient, clusterBuilder, func() {
			clusterBuilder.Status.Conditions = corev1alpha1.Conditions{
				{
					Type:               corev1alpha1.ConditionType("Ready"),
					Status:             corev1.ConditionStatus(metav1.ConditionTrue),
					LastTransitionTime: corev1alpha1.VolatileTime{Inner: metav1.Now()},
				},
			}
		})).To(Succeed())

		cfBuildGUID = PrefixedGUID("cf-build")
		env = []corev1.EnvVar{{
			Name: "VCAP_SERVICES",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "some-vcap-services-secret",
					},
					Key: "VCAP_SERVICES",
				},
			},
		}}
		services = []corev1.ObjectReference{
			{
				Kind: "Secret",
				Name: "some-services-secret",
			},
		}

		source = korifiv1alpha1.PackageSource{
			Registry: korifiv1alpha1.Registry{
				Image:            "PACKAGE_IMAGE",
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: wellFormedRegistryCredentialsSecret}},
			},
		}

		buildpacks = nil

		fakeImageConfigGetter.ConfigReturns(image.Config{
			Labels: map[string]string{
				"io.buildpacks.build.metadata": `{
					"processes": [
						{"type": "web", "command": "my-command", "args": ["foo", "bar"]},
						{"type": "db", "command": "my-command2"}
					]
				}`,
			},
			ExposedPorts: []int32{8080, 8443},
		}, nil)
	})

	AfterEach(func() {
		if clusterBuilder != nil {
			Expect(k8sClient.Delete(ctx, clusterBuilder)).To(Succeed())
		}
	})

	Describe("BuildWorkload initialization phase", func() {
		JustBeforeEach(func() {
			buildWorkload = buildWorkloadObject(cfBuildGUID, namespaceGUID, source, env, services, reconcilerName, buildpacks)
			Expect(k8sClient.Create(ctx, buildWorkload)).To(Succeed())
		})

		CheckInitialization := func() {
			It("reconciles the kpack.Image", func() {
				Eventually(func(g Gomega) {
					kpackImage := new(buildv1alpha2.Image)
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-guid", Namespace: namespaceGUID}, kpackImage)).To(Succeed())
					g.Expect(kpackImage.Spec.Build).ToNot(BeNil())
					g.Expect(kpackImage.Spec.Source.Registry.Image).To(BeEquivalentTo(source.Registry.Image))
					g.Expect(kpackImage.Spec.Source.Registry.ImagePullSecrets).To(BeEquivalentTo(source.Registry.ImagePullSecrets))
					g.Expect(kpackImage.Spec.Build.Env).To(Equal(env))
					g.Expect(kpackImage.Spec.Build.Services).To(BeEquivalentTo(services))
				}).Should(Succeed())
			})

			It("sets the build workload succeeded condition to unknown", func() {
				cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				updatedBuildWorkload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, cfBuildLookupKey, updatedBuildWorkload)).To(Succeed())
					g.Expect(mustHaveCondition(g, updatedBuildWorkload.Status.Conditions, "Succeeded").Status).To(Equal(metav1.ConditionUnknown))
				}).Should(Succeed())
			})

			It("creates the image repository", func() {
				Eventually(func(g Gomega) {
					g.Expect(imageRepoCreator.CreateRepositoryCallCount()).ToNot(BeZero())
					_, repoName := imageRepoCreator.CreateRepositoryArgsForCall(0)
					g.Expect(repoName).To(Equal("my.repository/my-prefix/app-guid-droplets"))
				}).Should(Succeed())
			})
		}

		CheckInitialization()

		When("kpack image already exists", func() {
			BeforeEach(func() {
				Expect(k8sClient.Create(ctx, &buildv1alpha2.Image{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app-guid",
						Namespace: namespaceGUID,
						Labels: map[string]string{
							controllers.BuildWorkloadLabelKey: cfBuildGUID,
						},
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
				})).To(Succeed())
			})

			CheckInitialization()
		})

		When("the source image pull secret doesn't exist", func() {
			var nonExistentSecret string

			BeforeEach(func() {
				nonExistentSecret = PrefixedGUID("no-such-secret")
				source.Registry.ImagePullSecrets = []corev1.LocalObjectReference{
					{Name: nonExistentSecret},
				}
			})

			It("doesn't create the kpack Image as long as the secret is missing", func() {
				Consistently(func() bool {
					lookupKey := types.NamespacedName{Name: "app-guid", Namespace: namespaceGUID}
					return k8serrors.IsNotFound(k8sClient.Get(ctx, lookupKey, new(buildv1alpha2.Image)))
				}).Should(BeTrue())
			})
		})

		When("buildpacks are specified", func() {
			BeforeEach(func() {
				buildpacks = []string{"paketo-buildpacks/java"}
			})

			It("sets the Succeeded conditions to False", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				updatedWorkload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, lookupKey, updatedWorkload)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(mustHaveCondition(g, updatedWorkload.Status.Conditions, "Succeeded").Status).To(Equal(metav1.ConditionFalse))
				}).Should(Succeed())

				foundCondition := meta.FindStatusCondition(updatedWorkload.Status.Conditions, "Succeeded")
				Expect(foundCondition.Message).To(ContainSubstring("buildpack"))
			})

			It("doesn't create a kpack Image", func() {
				Consistently(func() bool {
					lookupKey := types.NamespacedName{Name: "app-guid", Namespace: namespaceGUID}
					return k8serrors.IsNotFound(k8sClient.Get(ctx, lookupKey, new(buildv1alpha2.Image)))
				}).Should(BeTrue())
			})
		})

		When("reconciler name on BuildWorkload is not kpack-image-builder", func() {
			BeforeEach(func() {
				reconcilerName = "notkpackreconciler"
			})

			It("does not create a kpack image resource", func() {
				Consistently(func(g Gomega) {
					kpackImage := new(buildv1alpha2.Image)
					err := k8sClient.Get(ctx, types.NamespacedName{Name: "app-guid", Namespace: namespaceGUID}, kpackImage)
					g.Expect(err).To(MatchError(fmt.Sprintf("Image.kpack.io %q not found", "app-guid")))
				}).Should(Succeed())
			})

			When("the other reconciler has partially reconciled the object and created an Image", func() {
				BeforeEach(func() {
					image := buildKpackImageObject("app-guid", namespaceGUID, source, env, services)
					Expect(k8sClient.Create(ctx, image)).To(Succeed())

					kpackImageLookupKey := types.NamespacedName{Name: "app-guid", Namespace: namespaceGUID}
					createdKpackImage := new(buildv1alpha2.Image)
					Eventually(func() error {
						return k8sClient.Get(ctx, kpackImageLookupKey, createdKpackImage)
					}).Should(Succeed())

					Expect(k8s.Patch(ctx, k8sClient, createdKpackImage, func() {
						setKpackImageStatus(createdKpackImage, "Ready", metav1.ConditionTrue)
						createdKpackImage.Status.LatestImage = "some-org/my-image@sha256:some-sha"
						createdKpackImage.Status.LatestStack = "cflinuxfs3"
					})).To(Succeed())
				})

				JustBeforeEach(func() {
					updatedBuildWorkload := new(korifiv1alpha1.BuildWorkload)
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}, updatedBuildWorkload)
					}).Should(Succeed())

					Expect(k8s.Patch(ctx, k8sClient, updatedBuildWorkload, func() {
						meta.SetStatusCondition(&updatedBuildWorkload.Status.Conditions, metav1.Condition{
							Type:    korifiv1alpha1.SucceededConditionType,
							Status:  metav1.ConditionUnknown,
							Reason:  "thinking",
							Message: "thunking",
						})
					})).To(Succeed())
				})

				It("doesn't continue to reconcile the object", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
						g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, "Succeeded").Status).To(Equal(metav1.ConditionUnknown))
					}).Should(Succeed())

					Consistently(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
						g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, "Succeeded").Status).To(Equal(metav1.ConditionUnknown))
					}).Should(Succeed())
				})
			})
		})
	})

	Describe("updating build workload status from kpack image", func() {
		var (
			createdKpackImage    *buildv1alpha2.Image
			build, build1        *buildv1alpha2.Build
			buildSucceededStatus metav1.ConditionStatus
			buildSucceededReason string
			kpackBuildImageRef   string
			kpackBuildStack      string
		)

		BeforeEach(func() {
			buildWorkload = buildWorkloadObject(cfBuildGUID, namespaceGUID, source, env, services, reconcilerName, buildpacks)
			Expect(k8sClient.Create(ctx, buildWorkload)).To(Succeed())

			kpackImageLookupKey := types.NamespacedName{Name: "app-guid", Namespace: namespaceGUID}
			createdKpackImage = new(buildv1alpha2.Image)
			Eventually(func() error {
				return k8sClient.Get(ctx, kpackImageLookupKey, createdKpackImage)
			}).Should(Succeed())

			build = &buildv1alpha2.Build{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "build",
					Namespace: namespaceGUID,
					Labels: map[string]string{
						"image.kpack.io/image":           "app-guid",
						"image.kpack.io/imageGeneration": "1",
					},
				},
			}

			build1 = build.DeepCopy()
			Expect(k8sClient.Create(ctx, build1)).To(Succeed())

			buildSucceededStatus = ""
			buildSucceededReason = ""
		})

		JustBeforeEach(func() {
			Expect(k8s.Patch(ctx, k8sClient, build1, func() {
				build1.Status.Conditions = append(build1.Status.Conditions, corev1alpha1.Condition{
					Type:   corev1alpha1.ConditionType("Succeeded"),
					Status: corev1.ConditionStatus(buildSucceededStatus),
					Reason: buildSucceededReason,
				})

				build1.Status.Stack.ID = kpackBuildStack
				build1.Status.LatestImage = kpackBuildImageRef
			}))
		})

		When("there are two buildworkloads for the image", func() {
			var (
				cfBuildGUID2 string
				build2       *buildv1alpha2.Build
			)

			BeforeEach(func() {
				cfBuildGUID2 = cfBuildGUID + "-2"
				source2 := source
				source2.Registry.Image += "2"
				buildWorkload2 := buildWorkloadObject(cfBuildGUID2, namespaceGUID, source2, env, services, reconcilerName, buildpacks)
				Expect(k8sClient.Create(ctx, buildWorkload2)).To(Succeed())

				Eventually(func(g Gomega) {
					kpackImageLookupKey := types.NamespacedName{Name: "app-guid", Namespace: namespaceGUID}
					g.Expect(k8sClient.Get(ctx, kpackImageLookupKey, createdKpackImage)).To(Succeed())
					g.Expect(createdKpackImage.Spec.Source.Registry.Image).To(Equal(source2.Registry.Image))
				}).Should(Succeed())

				buildSucceededStatus = metav1.ConditionFalse

				build2 = build.DeepCopy()
				build2.Name = "build-2"
				build2.Labels["image.kpack.io/imageGeneration"] = "2"
				Expect(k8sClient.Create(ctx, build2)).To(Succeed())
			})

			JustBeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, build2, func() {
					build2.Status.Conditions = append(build2.Status.Conditions, corev1alpha1.Condition{
						Type:   corev1alpha1.ConditionType("Succeeded"),
						Status: corev1.ConditionStatus("True"),
					})
				}))
			})

			It("updates both the build workload statuses", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				updatedWorkload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, lookupKey, updatedWorkload)).To(Succeed())
					g.Expect(mustHaveCondition(g, updatedWorkload.Status.Conditions, "Succeeded").Status).To(Equal(metav1.ConditionFalse))
				}).Should(Succeed())

				lookupKey.Name = cfBuildGUID2
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, lookupKey, updatedWorkload)).To(Succeed())
					g.Expect(mustHaveCondition(g, updatedWorkload.Status.Conditions, "Succeeded").Status).To(Equal(metav1.ConditionTrue))
				}).Should(Succeed())
			})
		})

		When("the build is not ready", func() {
			BeforeEach(func() {
				buildSucceededStatus = metav1.ConditionFalse
				buildSucceededReason = "BuildFailed"
			})

			It("sets the Succeeded condition to False", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				updatedWorkload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, lookupKey, updatedWorkload)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(mustHaveCondition(g, updatedWorkload.Status.Conditions, "Succeeded").Status).To(Equal(metav1.ConditionFalse))
					g.Expect(mustHaveCondition(g, updatedWorkload.Status.Conditions, "Succeeded").Reason).To(Equal("BuildFailed"))
				}).Should(Succeed())
			})
		})

		When("the build succeeded", func() {
			var configCallCount int

			BeforeEach(func() {
				buildSucceededStatus = metav1.ConditionTrue
				kpackBuildImageRef = "foo.bar/baz@sha256:hello"
				kpackBuildStack = "cflinuxfs3"

				configCallCount = fakeImageConfigGetter.ConfigCallCount()
			})

			It("sets the Succeeded condition to True", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				updatedWorkload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, lookupKey, updatedWorkload)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(mustHaveCondition(g, updatedWorkload.Status.Conditions, "Succeeded").Status).To(Equal(metav1.ConditionTrue))
				}).Should(Succeed())
			})

			It("sets status.droplet", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				updatedBuildWorkload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func(g Gomega) *korifiv1alpha1.BuildDropletStatus {
					err := k8sClient.Get(ctx, lookupKey, updatedBuildWorkload)
					g.Expect(err).NotTo(HaveOccurred())
					return updatedBuildWorkload.Status.Droplet
				}).ShouldNot(BeNil())

				Expect(fakeImageConfigGetter.ConfigCallCount()).To(Equal(configCallCount + 1))
				_, creds, ref := fakeImageConfigGetter.ConfigArgsForCall(configCallCount)
				Expect(creds.Namespace).To(Equal(namespaceGUID))
				Expect(creds.ServiceAccountName).To(Equal("builder-service-account"))
				Expect(ref).To(Equal(kpackBuildImageRef))

				Expect(updatedBuildWorkload.Status.Droplet.Registry.Image).To(Equal(kpackBuildImageRef))
				Expect(updatedBuildWorkload.Status.Droplet.Stack).To(Equal(kpackBuildStack))
				Expect(updatedBuildWorkload.Status.Droplet.Registry.ImagePullSecrets).To(Equal(source.Registry.ImagePullSecrets))
				Expect(updatedBuildWorkload.Status.Droplet.ProcessTypes).To(Equal([]korifiv1alpha1.ProcessType{
					{Type: "web", Command: `my-command "foo" "bar"`},
					{Type: "db", Command: "my-command2"},
				}))
				Expect(updatedBuildWorkload.Status.Droplet.Ports).To(Equal([]int32{8080, 8443}))
			})
		})
	})

	Describe("awaiting kpack builder readiness", func() {
		BeforeEach(func() {
			buildWorkload = buildWorkloadObject(cfBuildGUID, namespaceGUID, source, env, services, reconcilerName, buildpacks)
			Expect(k8sClient.Create(ctx, buildWorkload)).To(Succeed())

			Expect(k8s.Patch(ctx, k8sClient, clusterBuilder, func() {
				clusterBuilder.Status.Conditions = corev1alpha1.Conditions{
					{
						Type:               corev1alpha1.ConditionType("Ready"),
						Status:             corev1.ConditionStatus(metav1.ConditionFalse),
						LastTransitionTime: corev1alpha1.VolatileTime{Inner: metav1.Now()},
					},
				}
			})).To(Succeed())
		})

		It("sets the Succeeded condition to False", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
				g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, "Succeeded").Status).To(Equal(metav1.ConditionFalse))
				g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, "Succeeded").Reason).To(Equal("BuilderNotReady"))
			}).Should(Succeed())
		})

		When("the kpack builder becomes ready", func() {
			BeforeEach(func() {
				Consistently(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
					g.Expect(meta.FindStatusCondition(buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType)).To(
						SatisfyAny(
							BeNil(),
							PointTo(MatchFields(
								IgnoreExtras,
								Fields{"Status": Equal(metav1.ConditionUnknown)},
							)),
						),
					)
				}, "2s").Should(Succeed())

				Expect(k8s.Patch(ctx, k8sClient, clusterBuilder, func() {
					clusterBuilder.Status.Conditions = corev1alpha1.Conditions{
						{
							Type:               corev1alpha1.ConditionType("Ready"),
							Status:             corev1.ConditionStatus(metav1.ConditionTrue),
							LastTransitionTime: corev1alpha1.VolatileTime{Inner: metav1.Now()},
						},
					}
				})).To(Succeed())
			})

			It("never sets Succeeded condition to False (as it is tolerant towards builder being unavailable for a while)", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
					g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, "Succeeded").Status).ToNot(Equal(metav1.ConditionFalse))
				}).Should(Succeed())
				Consistently(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
					g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, "Succeeded").Status).ToNot(Equal(metav1.ConditionFalse))
				}).Should(Succeed())
			})
		})

		When("the kpack builder is not found", func() {
			BeforeEach(func() {
				Expect(k8sClient.Delete(ctx, clusterBuilder)).To(Succeed())
				clusterBuilder = nil
			})

			It("sets the Succeeded condition to False", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
					g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, "Succeeded").Status).To(Equal(metav1.ConditionFalse))
					g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, "Succeeded").Reason).To(Equal("BuilderNotReady"))
					g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, "Succeeded").Message).To(Equal("ClusterBuilder not found"))
				}).Should(Succeed())
			})
		})
	})

	Describe("detecting skipped previous builds", func() {
		var buildWorkload2 *korifiv1alpha1.BuildWorkload

		BeforeEach(func() {
			buildWorkload = buildWorkloadObject(cfBuildGUID, namespaceGUID, source, env, services, reconcilerName, buildpacks)
			Expect(k8sClient.Create(ctx, buildWorkload)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
				g.Expect(buildWorkload.Labels).To(HaveKey(controllers.ImageGenerationKey))
			}).Should(Succeed())

			source2 := source
			source2.Registry.Image += "2"
			buildWorkload2 = buildWorkloadObject(testutils.GenerateGUID(), namespaceGUID, source2, env, services, reconcilerName, buildpacks)
			Expect(k8sClient.Create(ctx, buildWorkload2)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload2), buildWorkload2)).To(Succeed())
				g.Expect(buildWorkload2.Labels).To(HaveKey(controllers.ImageGenerationKey))
			}).Should(Succeed())
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, &buildv1alpha2.Build{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "build",
					Namespace: namespaceGUID,
					Labels: map[string]string{
						"image.kpack.io/image":           "app-guid",
						"image.kpack.io/imageGeneration": buildWorkload2.Labels[controllers.ImageGenerationKey],
					},
				},
			})).To(Succeed())
		})

		It("fails the first build workload", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
				g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType).Status).To(Equal(metav1.ConditionFalse))
				g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType).Reason).To(Equal("KpackMissedBuild"))
			}).Should(Succeed())
		})

		When("previous build workload builds are still running", func() {
			BeforeEach(func() {
				Expect(k8sClient.Create(ctx, &buildv1alpha2.Build{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testutils.GenerateGUID(),
						Namespace: namespaceGUID,
						Labels: map[string]string{
							"image.kpack.io/image":           "app-guid",
							"image.kpack.io/imageGeneration": buildWorkload.Labels[controllers.ImageGenerationKey],
						},
					},
				})).To(Succeed())
			})

			It("does not fail it", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
					g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType).Status).To(Equal(metav1.ConditionUnknown))
				}).Should(Succeed())
			})
		})

		When("the first build workload has already completed", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, buildWorkload, func() {
					meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
						Type:   korifiv1alpha1.SucceededConditionType,
						Status: metav1.ConditionTrue,
						Reason: "DoneAlready",
					})
				})).To(Succeed())

				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
					g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType).Status).To(Equal(metav1.ConditionTrue))
				}).Should(Succeed())
			})

			It("does not fail the first build workload", func() {
				Consistently(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), buildWorkload)).To(Succeed())
					g.Expect(mustHaveCondition(g, buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType).Status).To(Equal(metav1.ConditionTrue))
				}).Should(Succeed())
			})
		})

		When("there are build workloads with greater image generation", func() {
			var buildWorkload3 *korifiv1alpha1.BuildWorkload

			BeforeEach(func() {
				source3 := source
				source3.Registry.Image += "3"
				buildWorkload3 = buildWorkloadObject(testutils.GenerateGUID(), namespaceGUID, source3, env, services, reconcilerName, buildpacks)
				Expect(k8sClient.Create(ctx, buildWorkload3)).To(Succeed())
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload3), buildWorkload3)).To(Succeed())
					g.Expect(buildWorkload3.Labels).To(HaveKey(controllers.ImageGenerationKey))
				}).Should(Succeed())
			})

			It("does not fail them", func() {
				Eventually(func(g Gomega) {
					g.Expect(mustHaveCondition(g, buildWorkload3.Status.Conditions, korifiv1alpha1.SucceededConditionType).Status).To(Equal(metav1.ConditionUnknown))
				}).Should(Succeed())
				Consistently(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload3), buildWorkload3)).To(Succeed())
					g.Expect(mustHaveCondition(g, buildWorkload3.Status.Conditions, korifiv1alpha1.SucceededConditionType).Status).To(Equal(metav1.ConditionUnknown))
				}).Should(Succeed())
			})
		})
	})

	Describe("deleting the build workload", func() {
		var kpackBuild *buildv1alpha2.Build

		BeforeEach(func() {
			buildWorkload = buildWorkloadObject(cfBuildGUID, namespaceGUID, source, env, services, reconcilerName, buildpacks)
			Expect(k8sClient.Create(ctx, buildWorkload)).To(Succeed())

			kpackBuild = &buildv1alpha2.Build{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "build",
					Namespace: namespaceGUID,
					Labels: map[string]string{
						"image.kpack.io/image":           "app-guid",
						"image.kpack.io/imageGeneration": "1",
					},
				},
			}

			Expect(k8sClient.Create(ctx, kpackBuild)).To(Succeed())
		})

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				foundBuildWorkload := new(korifiv1alpha1.BuildWorkload)
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), foundBuildWorkload)).To(Succeed())
				g.Expect(foundBuildWorkload.Labels).To(HaveKey(controllers.ImageGenerationKey))
			}).Should(Succeed())
			Expect(k8sClient.Delete(ctx, buildWorkload)).To(Succeed())
		})

		It("deletes the corresponding kpack.Build", func() {
			Eventually(func(g Gomega) {
				foundBuildWorkload := new(korifiv1alpha1.BuildWorkload)
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), foundBuildWorkload)).To(MatchError(ContainSubstring("not found")))
				foundKpackBuild := new(buildv1alpha2.Build)
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(kpackBuild), foundKpackBuild)).To(MatchError(ContainSubstring("not found")))
			}).Should(Succeed())
		})

		When("there is another buildworkload referring to the kpack.Build", func() {
			BeforeEach(func() {
				otherBuildWorkload := buildWorkloadObject(testutils.GenerateGUID(), namespaceGUID, source, env, services, reconcilerName, buildpacks)
				otherBuildWorkload.Labels[controllers.ImageGenerationKey] = "1"
				Expect(k8sClient.Create(ctx, otherBuildWorkload)).To(Succeed())
			})

			It("doesn't delete the kpack.Build", func() {
				Eventually(func(g Gomega) {
					foundBuildWorkload := new(korifiv1alpha1.BuildWorkload)
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(buildWorkload), foundBuildWorkload)).To(MatchError(ContainSubstring("not found")))
				}).Should(Succeed())

				Consistently(func(g Gomega) {
					foundKpackBuild := new(buildv1alpha2.Build)
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(kpackBuild), foundKpackBuild)).To(Succeed())
				}).Should(Succeed())
			})
		})
	})
})

func setKpackImageStatus(kpackImage *buildv1alpha2.Image, conditionType string, conditionStatus metav1.ConditionStatus) {
	kpackImage.Status.Conditions = append(kpackImage.Status.Conditions, corev1alpha1.Condition{
		Type:   corev1alpha1.ConditionType(conditionType),
		Status: corev1.ConditionStatus(conditionStatus),
	})
}

func buildDockerRegistrySecret(name, namespace string) *corev1.Secret {
	dockerRegistryUsername := "user"
	dockerRegistryPassword := "password"
	dockerAuth := base64.StdEncoding.EncodeToString([]byte(dockerRegistryUsername + ":" + dockerRegistryPassword))
	dockerConfigJSON := `{"auths":{"https://index.docker.io/v1/":{"username":"` + dockerRegistryUsername + `","password":"` + dockerRegistryPassword + `","auth":"` + dockerAuth + `"}}}`
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Immutable: nil,
		Data:      nil,
		StringData: map[string]string{
			".dockerconfigjson": dockerConfigJSON,
		},
		Type: "kubernetes.io/dockerconfigjson",
	}
}

func buildServiceAccount(name, namespace, imagePullSecretName string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Secrets:          []corev1.ObjectReference{{Name: imagePullSecretName}},
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: imagePullSecretName}},
	}
}

func PrefixedGUID(prefix string) string {
	return prefix + "-" + uuid.NewString()[:8]
}

func buildWorkloadObject(cfBuildGUID string, namespace string, source korifiv1alpha1.PackageSource, env []corev1.EnvVar, services []corev1.ObjectReference, reconcilerName string, buildpacks []string) *korifiv1alpha1.BuildWorkload {
	return &korifiv1alpha1.BuildWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfBuildGUID,
			Namespace: namespace,
			Labels: map[string]string{
				korifiv1alpha1.CFAppGUIDLabelKey: "app-guid",
			},
		},
		Spec: korifiv1alpha1.BuildWorkloadSpec{
			BuildRef: korifiv1alpha1.RequiredLocalObjectReference{
				Name: cfBuildGUID,
			},
			Source:      source,
			Buildpacks:  buildpacks,
			Env:         env,
			Services:    services,
			BuilderName: reconcilerName,
		},
	}
}

func mustHaveCondition(g Gomega, conditions []metav1.Condition, conditionType string) *metav1.Condition {
	foundCondition := meta.FindStatusCondition(conditions, conditionType)
	g.ExpectWithOffset(1, foundCondition).NotTo(BeNil())
	return foundCondition
}

func buildKpackImageObject(name string, namespace string, source korifiv1alpha1.PackageSource, env []corev1.EnvVar, services []corev1.ObjectReference) *buildv1alpha2.Image {
	return &buildv1alpha2.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				controllers.BuildWorkloadLabelKey: name,
			},
		},
		Spec: buildv1alpha2.ImageSpec{
			Tag: "kpack-image-tag",
			Builder: corev1.ObjectReference{
				Kind:       "ClusterBuilder",
				Name:       "default",
				APIVersion: "kpack.io/v1alpha2",
			},
			ServiceAccountName: "kpack-service-account",
			Source: corev1alpha1.SourceConfig{
				Registry: &corev1alpha1.Registry{
					Image:            source.Registry.Image,
					ImagePullSecrets: source.Registry.ImagePullSecrets,
				},
			},
			Build: &buildv1alpha2.ImageBuild{
				Services: services,
				Env:      env,
			},
		},
	}
}
