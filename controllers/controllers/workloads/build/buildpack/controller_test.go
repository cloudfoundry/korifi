package buildpack_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CFBuildpackBuildReconciler Integration Tests", func() {
	var (
		cfApp     *korifiv1alpha1.CFApp
		cfPackage *korifiv1alpha1.CFPackage
		cfBuild   *korifiv1alpha1.CFBuild
	)

	eventuallyBuildWorkloadShould := func(assertion func(*korifiv1alpha1.BuildWorkload, Gomega)) {
		GinkgoHelper()

		Eventually(func(g Gomega) {
			workload := new(korifiv1alpha1.BuildWorkload)
			lookupKey := types.NamespacedName{Name: cfBuild.Name, Namespace: testNamespace}
			g.Expect(adminClient.Get(context.Background(), lookupKey, workload)).To(Succeed())
			assertion(workload, g)
		}).Should(Succeed())
	}

	BeforeEach(func() {
		envSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			StringData: map[string]string{
				"a_key": "a-val",
				"b_key": "b-val",
			},
		}
		Expect(adminClient.Create(ctx, envSecret)).To(Succeed())

		vcapApplicationSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			StringData: map[string]string{
				"VCAP_APPLICATION": "{}",
			},
		}
		Expect(adminClient.Create(ctx, vcapApplicationSecret)).To(Succeed())

		vcapServicesSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			StringData: map[string]string{
				"VCAP_SERVICES": "{}",
			},
		}
		Expect(adminClient.Create(ctx, vcapServicesSecret)).To(Succeed())

		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  "test-app-name",
				DesiredState: "STOPPED",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
				EnvSecretName: envSecret.Name,
			},
		}
		Expect(adminClient.Create(ctx, cfApp)).To(Succeed())
		Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
			cfApp.Status.VCAPServicesSecretName = vcapServicesSecret.Name
			cfApp.Status.VCAPApplicationSecretName = vcapApplicationSecret.Name
		})).To(Succeed())

		cfPackage = &korifiv1alpha1.CFPackage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFPackageSpec{
				Type: "bits",
				AppRef: corev1.LocalObjectReference{
					Name: cfApp.Name,
				},
				Source: korifiv1alpha1.PackageSource{
					Registry: korifiv1alpha1.Registry{
						Image:            "ref",
						ImagePullSecrets: []corev1.LocalObjectReference{{Name: "source-registry-image-pull-secret"}},
					},
				},
			},
		}
		Expect(adminClient.Create(ctx, cfPackage)).To(Succeed())

		cfBuild = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFBuildSpec{
				PackageRef: corev1.LocalObjectReference{
					Name: cfPackage.Name,
				},
				AppRef: corev1.LocalObjectReference{
					Name: cfApp.Name,
				},
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
					Data: korifiv1alpha1.LifecycleData{
						Buildpacks: []string{"first-buildpack", "second-buildpack"},
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(context.Background(), cfBuild)).To(Succeed())
	})

	It("creates a BuildWorkload with the buildRef, source, env, and buildpacks set", func() {
		eventuallyBuildWorkloadShould(func(workload *korifiv1alpha1.BuildWorkload, g Gomega) {
			g.Expect(workload.Spec.BuilderName).To(Equal("buildpack-builder-name"))
			g.Expect(workload.Spec.BuildRef.Name).To(Equal(cfBuild.Name))
			g.Expect(workload.Spec.Source).To(Equal(cfPackage.Spec.Source))
			g.Expect(workload.Spec.Env).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal("a_key"),
					"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
						"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
							"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
								"Name": Equal(cfApp.Spec.EnvSecretName),
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
								"Name": Equal(cfApp.Spec.EnvSecretName),
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
								"Name": Equal(cfApp.Status.VCAPServicesSecretName),
							}),
							"Key": Equal("VCAP_SERVICES"),
						})),
					})),
				}),
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal("VCAP_APPLICATION"),
					"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
						"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
							"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
								"Name": Equal(cfApp.Status.VCAPApplicationSecretName),
							}),
							"Key": Equal("VCAP_APPLICATION"),
						})),
					})),
				}),
			))
			g.Expect(workload.Spec.Buildpacks).To(ConsistOf("first-buildpack", "second-buildpack"))
			g.Expect(workload.GetOwnerReferences()).To(ConsistOf(metav1.OwnerReference{
				UID:                cfBuild.UID,
				Kind:               "CFBuild",
				APIVersion:         "korifi.cloudfoundry.org/v1alpha1",
				Name:               cfBuild.Name,
				Controller:         tools.PtrTo(true),
				BlockOwnerDeletion: tools.PtrTo(true),
			}))
		})
	})

	It("sets the 'build-running' status conditions on CFBuild", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())

			stagingCondition := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)
			g.Expect(stagingCondition).NotTo(BeNil())
			g.Expect(stagingCondition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(stagingCondition.Reason).To(Equal("BuildRunning"))
			g.Expect(stagingCondition.ObservedGeneration).To(Equal(cfBuild.Generation))

			succeededCondition := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)
			g.Expect(succeededCondition).NotTo(BeNil())
			g.Expect(succeededCondition.Status).To(Equal(metav1.ConditionUnknown))
			g.Expect(succeededCondition.ObservedGeneration).To(Equal(cfBuild.Generation))
		}).Should(Succeed())
	})

	When("the referenced app has a ServiceBinding", func() {
		BeforeEach(func() {
			serviceBinding := &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-binding-guid",
					Namespace: testNamespace,
					Labels: map[string]string{
						korifiv1alpha1.CFAppGUIDLabelKey: cfApp.Name,
					},
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					AppRef: corev1.LocalObjectReference{
						Name: cfApp.Name,
					},
				},
			}
			Expect(adminClient.Create(ctx, serviceBinding)).To(Succeed())

			Expect(k8s.Patch(ctx, adminClient, serviceBinding, func() {
				serviceBinding.Status.Binding.Name = "service-secret"
			})).To(Succeed())
		})

		It("adds the binding secret to the workload services", func() {
			eventuallyBuildWorkloadShould(func(workload *korifiv1alpha1.BuildWorkload, g Gomega) {
				g.Expect(workload.Spec.Services).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name":       Equal("service-secret"),
						"Kind":       Equal("Secret"),
						"APIVersion": Equal("v1"),
					}),
				))
			})
		})
	})

	When("a BuildWorkload with CFBuild GUID already exists", func() {
		var existingBuildWorkload *korifiv1alpha1.BuildWorkload

		BeforeEach(func() {
			existingBuildWorkload = &korifiv1alpha1.BuildWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfBuild.Name,
					Namespace: testNamespace,
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
			Expect(adminClient.Create(ctx, existingBuildWorkload)).To(Succeed())
		})

		It("sets the status conditions on the CFBuild to running", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())

				stagingCondition := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)
				g.Expect(stagingCondition).NotTo(BeNil())
				g.Expect(stagingCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(stagingCondition.Reason).To(Equal("BuildRunning"))
				g.Expect(stagingCondition.ObservedGeneration).To(Equal(cfBuild.Generation))

				succeededCondition := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)
				g.Expect(succeededCondition).NotTo(BeNil())
				g.Expect(succeededCondition.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(succeededCondition.ObservedGeneration).To(Equal(cfBuild.Generation))
			}).Should(Succeed())
		})
	})

	When("the BuildWorkload failed", func() {
		JustBeforeEach(func() {
			lookupKey := types.NamespacedName{Name: cfBuild.Name, Namespace: testNamespace}
			Eventually(func(g Gomega) {
				workload := new(korifiv1alpha1.BuildWorkload)
				g.Expect(adminClient.Get(ctx, lookupKey, workload)).To(Succeed())
				g.Expect(k8s.Patch(ctx, adminClient, workload, func() {
					meta.SetStatusCondition(&workload.Status.Conditions, metav1.Condition{
						Type:   korifiv1alpha1.SucceededConditionType,
						Status: metav1.ConditionFalse,
						Reason: "shrug",
					})
				})).To(Succeed())
			}).Should(Succeed())
		})

		It("sets the CFBuild status condition Succeeded = False", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())

				stagingStatusCondition := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)
				g.Expect(stagingStatusCondition).NotTo(BeNil())
				g.Expect(stagingStatusCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(stagingStatusCondition.Reason).To(Equal("BuildNotRunning"))
				g.Expect(stagingStatusCondition.ObservedGeneration).To(Equal(cfBuild.Generation))

				succeededStatusCondition := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)
				g.Expect(succeededStatusCondition).NotTo(BeNil())
				g.Expect(succeededStatusCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(succeededStatusCondition.Reason).To(Equal("BuildFailed"))
				g.Expect(succeededStatusCondition.ObservedGeneration).To(Equal(cfBuild.Generation))
			}).Should(Succeed())
		})
	})

	When("the BuildWorkload finished successfully", func() {
		const (
			buildImageRef       = "some-org/my-image@sha256:some-sha"
			imagePullSecretName = "image-pull-s3cr37"
			buildStack          = "cflinuxfs3"
		)

		JustBeforeEach(func() {
			lookupKey := types.NamespacedName{Name: cfBuild.Name, Namespace: testNamespace}
			Eventually(func(g Gomega) {
				workload := new(korifiv1alpha1.BuildWorkload)
				g.Expect(adminClient.Get(ctx, lookupKey, workload)).To(Succeed())
				g.Expect(k8s.Patch(ctx, adminClient, workload, func() {
					meta.SetStatusCondition(&workload.Status.Conditions, metav1.Condition{
						Type:   korifiv1alpha1.SucceededConditionType,
						Status: metav1.ConditionTrue,
						Reason: "shrug",
					})
					workload.Status.Droplet = &korifiv1alpha1.BuildDropletStatus{
						Registry: korifiv1alpha1.Registry{
							Image:            buildImageRef,
							ImagePullSecrets: []corev1.LocalObjectReference{{Name: imagePullSecretName}},
						},
						Stack: buildStack,
						Ports: []int32{42},
						ProcessTypes: []korifiv1alpha1.ProcessType{{
							Type:    "web",
							Command: "run-stuff",
						}},
					}
				})).To(Succeed())
			}).Should(Succeed())
		})

		It("sets the CFBuild status condition Succeeded = True", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
				stagingStatusCondition := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)
				g.Expect(stagingStatusCondition).NotTo(BeNil())
				g.Expect(stagingStatusCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(stagingStatusCondition.Reason).To(Equal("BuildNotRunning"))
				g.Expect(stagingStatusCondition.ObservedGeneration).To(Equal(cfBuild.Generation))

				succeededStatusCondition := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)
				g.Expect(succeededStatusCondition).NotTo(BeNil())
				g.Expect(succeededStatusCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(succeededStatusCondition.Reason).To(Equal("BuildSucceeded"))
				g.Expect(succeededStatusCondition.ObservedGeneration).To(Equal(cfBuild.Generation))
			}).Should(Succeed())
		})

		It("sets CFBuild.status.droplet", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
				g.Expect(cfBuild.Status.Droplet).NotTo(BeNil())
				g.Expect(cfBuild.Status.Droplet.Registry.Image).To(Equal(buildImageRef))
				g.Expect(cfBuild.Status.Droplet.Registry.ImagePullSecrets).To(ConsistOf(corev1.LocalObjectReference{Name: imagePullSecretName}))
				g.Expect(cfBuild.Status.Droplet.Stack).To(Equal(buildStack))
				g.Expect(cfBuild.Status.Droplet.ProcessTypes).To(ConsistOf(korifiv1alpha1.ProcessType{
					Type:    "web",
					Command: "run-stuff",
				}))
				g.Expect(cfBuild.Status.Droplet.Ports).To(ConsistOf(BeEquivalentTo(42)))
			}).Should(Succeed())
		})
	})
})
