package apps_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFAppReconciler Integration Tests", func() {
	var (
		cfBuild *korifiv1alpha1.CFBuild
		cfApp   *korifiv1alpha1.CFApp
	)

	BeforeEach(func() {
		appName := uuid.NewString()

		cfPackage := &korifiv1alpha1.CFPackage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFPackageSpec{
				Type: "bits",
				AppRef: corev1.LocalObjectReference{
					Name: appName,
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
					Name: appName,
				},
				StagingMemoryMB: 1024,
				StagingDiskMB:   1024,
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
					// TODO: no need for Data as it empty
					Data: korifiv1alpha1.LifecycleData{
						Buildpacks: nil,
						Stack:      "",
					},
				},
			},
		}
		Expect(adminClient.Create(ctx, cfBuild)).To(Succeed())
		Expect(k8s.Patch(ctx, adminClient, cfBuild, func() {
			cfBuild.Status = korifiv1alpha1.CFBuildStatus{
				Droplet: &korifiv1alpha1.BuildDropletStatus{
					Registry: korifiv1alpha1.Registry{
						Image:            "image/registry/url",
						ImagePullSecrets: []corev1.LocalObjectReference{{Name: "some-image-pull-secret"}},
					},
					Stack: "cflinuxfs3",
				},
			}
		})).To(Succeed())

		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: testNamespace,
				Annotations: map[string]string{
					korifiv1alpha1.CFAppRevisionKey: "42",
				},
				Finalizers: []string{
					korifiv1alpha1.CFAppFinalizerName,
				},
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  "test-app",
				DesiredState: korifiv1alpha1.StoppedState,
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
				CurrentDropletRef: corev1.LocalObjectReference{
					Name: cfBuild.Name,
				},
			},
		}
		Expect(adminClient.Create(ctx, cfApp)).To(Succeed())
		Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
			cfApp.Status.ActualState = korifiv1alpha1.StoppedState
		})).To(Succeed())
	})

	It("sets the observed generation in the cfapp status", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
			g.Expect(cfApp.Status.ObservedGeneration).To(BeEquivalentTo(cfApp.Generation))
		}).Should(Succeed())
	})

	It("sets the app actual state to the desired state", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
			g.Expect(cfApp.Status.ActualState).To(Equal(korifiv1alpha1.StoppedState))
		}).Should(Succeed())
	})

	It("sets the last-stop-app-rev annotation to the value of the app-rev annotation", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
			g.Expect(cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]).To(Equal("42"))
		}).Should(Succeed())
	})

	It("sets status.VCAPApplicationSecretName", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())

			g.Expect(cfApp.Status.VCAPApplicationSecretName).NotTo(BeEmpty())
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cfApp.Namespace,
					Name:      cfApp.Status.VCAPApplicationSecretName,
				},
			}), &corev1.Secret{})).To(Succeed())
		}).Should(Succeed())
	})

	It("set status.VCAPServicesSecretName", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
			g.Expect(cfApp.Status.VCAPServicesSecretName).NotTo(BeEmpty())

			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cfApp.Namespace,
					Name:      cfApp.Status.VCAPServicesSecretName,
				},
			}), &corev1.Secret{})).To(Succeed())
		}).Should(Succeed())
	})

	When("lastStopAppRev annotation is set", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				g.Expect(cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]).To(Equal("42"))
			}).Should(Succeed())

			Expect(k8s.PatchResource(ctx, adminClient, cfApp, func() {
				cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey] = "2"
			})).To(Succeed())
		})

		It("does not override it", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				g.Expect(cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]).To(Equal("2"))
			}).Should(Succeed())
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				g.Expect(cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]).To(Equal("2"))
			}, "1s").Should(Succeed())
		})
	})

	It("sets the ready condition to true", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
			g.Expect(cfApp.Status.Conditions).To(ContainElement(SatisfyAll(
				HasType(Equal(korifiv1alpha1.StatusConditionReady)),
				HasStatus(Equal(metav1.ConditionTrue)),
			)))
		}).Should(Succeed())
	})

	It("creates a default web CFProcess", func() {
		Eventually(func(g Gomega) {
			cfProcessList := &korifiv1alpha1.CFProcessList{}
			g.Expect(
				adminClient.List(ctx, cfProcessList, &client.ListOptions{
					Namespace: cfApp.Namespace,
				}),
			).To(Succeed())
			g.Expect(cfProcessList.Items).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"Spec": MatchFields(IgnoreExtras, Fields{
						"ProcessType":     Equal("web"),
						"DetectedCommand": BeEmpty(),
						"Command":         BeEmpty(),
						"AppRef":          Equal(corev1.LocalObjectReference{Name: cfApp.Name}),
					}),
					"ObjectMeta": MatchFields(IgnoreExtras, Fields{
						"OwnerReferences": ConsistOf(MatchFields(IgnoreExtras, Fields{
							"Name": Equal(cfApp.Name),
						})),
					}),
				}),
			))
		}).Should(Succeed())
	})

	When("the droplet specifies processes", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, cfBuild, func() {
				cfBuild.Status.Droplet.ProcessTypes = []korifiv1alpha1.ProcessType{{
					Type:    "web",
					Command: "web-process command",
				}, {
					Type:    "worker",
					Command: "process-worker command",
				}}
			})).To(Succeed())
		})

		It("creates a CFProcess per droplet process", func() {
			Eventually(func(g Gomega) {
				cfProcessList := &korifiv1alpha1.CFProcessList{}
				g.Expect(
					adminClient.List(ctx, cfProcessList, &client.ListOptions{
						Namespace: cfApp.Namespace,
					}),
				).To(Succeed())
				g.Expect(cfProcessList.Items).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"ProcessType":     Equal("web"),
							"DetectedCommand": Equal("web-process command"),
							"Command":         BeEmpty(),
						}),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"ProcessType":     Equal("worker"),
							"DetectedCommand": Equal("process-worker command"),
							"Command":         BeEmpty(),
						}),
					}),
				))
			}).Should(Succeed())
		})
	})

	When("the app desired state does not match the actual state", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				cfProcessList := &korifiv1alpha1.CFProcessList{}
				g.Expect(adminClient.List(ctx, cfProcessList, &client.ListOptions{
					Namespace: cfApp.Namespace,
				})).To(Succeed())
				g.Expect(cfProcessList.Items).To(HaveLen(1))

				process := &cfProcessList.Items[0]
				g.Expect(k8s.Patch(ctx, adminClient, process, func() {
					process.Status.ActualInstances = 1
				})).To(Succeed())

				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				g.Expect(cfApp.Status.ActualState).To(Equal(korifiv1alpha1.StartedState))
			}).Should(Succeed())
		})

		It("sets the ready condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				g.Expect(cfApp.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
					HasReason(Equal("DesiredStateNotReached")),
				)))
			}).Should(Succeed())
		})
	})

	When("the cfapp droplet ref is not set", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, cfApp, func() {
				cfApp.Spec.CurrentDropletRef.Name = ""
			})).To(Succeed())
		})

		It("sets the ready condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				g.Expect(cfApp.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
					HasReason(Equal("DropletNotAssigned")),
				)))
			}).Should(Succeed())
		})
	})

	When("the current droplet does not exist", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, cfApp, func() {
				cfApp.Spec.CurrentDropletRef.Name = "i-do-not-exist"
			})).To(Succeed())
		})

		It("sets the ready condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				g.Expect(cfApp.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
					HasReason(Equal("CannotResolveCurrentDropletRef")),
				)))
			}).Should(Succeed())
		})
	})

	When("the app has a service binding", func() {
		var binding *korifiv1alpha1.CFServiceBinding

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: cfApp.Namespace,
				},
				Data: map[string][]byte{
					korifiv1alpha1.CredentialsSecretKey: []byte("{}"),
				},
			}
			Expect(adminClient.Create(ctx, secret)).To(Succeed())

			instance := &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: cfApp.Namespace,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					SecretName: secret.Name,
					Type:       "user-provided",
				},
			}
			Expect(adminClient.Create(ctx, instance)).To(Succeed())

			binding = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: cfApp.Namespace,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					DisplayName: new(string),
					Service: corev1.ObjectReference{
						Namespace: cfApp.Namespace,
						Name:      instance.Name,
					},
					AppRef: corev1.LocalObjectReference{Name: cfApp.Name},
				},
			}
			Expect(adminClient.Create(ctx, binding)).To(Succeed())

			Expect(k8s.Patch(ctx, adminClient, binding, func() {
				binding.Status.Credentials.Name = secret.Name
			})).To(Succeed())
		})

		It("sets the ready condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				g.Expect(cfApp.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
					HasReason(Equal("BindingNotReady")),
				)))
			}).Should(Succeed())
		})

		When("the binding becomes ready", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, adminClient, binding, func() {
					meta.SetStatusCondition(&binding.Status.Conditions, metav1.Condition{
						Type:   korifiv1alpha1.StatusConditionReady,
						Status: metav1.ConditionTrue,
						Reason: "BindingReady",
					})
				})).To(Succeed())
			})

			It("sets the ready condition to true", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
					g.Expect(cfApp.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionTrue)),
					)))
				}).Should(Succeed())
			})
		})
	})

	Describe("finalization", func() {
		var (
			cfDomainGUID string
			cfRoute      *korifiv1alpha1.CFRoute
		)

		BeforeEach(func() {
			cfDomainGUID = uuid.NewString()
			cfDomain := &korifiv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      cfDomainGUID,
				},
				Spec: korifiv1alpha1.CFDomainSpec{
					Name: "a" + uuid.NewString() + ".com",
				},
			}
			Expect(adminClient.Create(ctx, cfDomain)).To(Succeed())

			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: testNamespace,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "test-route-host",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      cfDomainGUID,
						Namespace: testNamespace,
					},
					Destinations: []korifiv1alpha1.Destination{
						{
							GUID: "destination-1-guid",
							AppRef: corev1.LocalObjectReference{
								Name: cfApp.Name,
							},
							ProcessType: "web",
							Protocol:    tools.PtrTo("http1"),
						},
					},
				},
			}
			Expect(adminClient.Create(ctx, cfRoute)).To(Succeed())

			cfServiceBinding := korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: testNamespace,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					AppRef: corev1.LocalObjectReference{
						Name: cfApp.Name,
					},
				},
			}
			Expect(adminClient.Create(ctx, &cfServiceBinding)).To(Succeed())

			Expect(adminClient.Delete(ctx, cfApp)).To(Succeed())
			Eventually(func(g Gomega) {
				err := adminClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})

		It("deletes the destination on the CFRoute", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)).To(Succeed())
				g.Expect(cfRoute.Spec.Destinations).To(BeEmpty())
			}).Should(Succeed())
		})

		It("deletes the referencing service bindings", func() {
			Eventually(func(g Gomega) {
				sbList := korifiv1alpha1.CFServiceBindingList{}
				g.Expect(adminClient.List(ctx, &sbList, client.InNamespace(cfApp.Namespace))).To(Succeed())
				g.Expect(sbList.Items).To(BeEmpty())
			}).Should(Succeed())
		})
	})
})
