package label_indexer_test

import (
	"maps"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("LabelIndexerWebhook", func() {
	Describe("CFRoute", func() {
		var route *korifiv1alpha1.CFRoute

		BeforeEach(func() {
			route = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host: "example",
					Path: "/example",
					DomainRef: corev1.ObjectReference{
						Name: "example.com",
					},
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, route)).To(Succeed())
		})

		It("labels the CFRoute with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
				g.Expect(route.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.CFDomainGUIDLabelKey:      Equal("example.com"),
					korifiv1alpha1.SpaceGUIDKey:              Equal(namespace),
					korifiv1alpha1.CFRouteIsUnmappedLabelKey: Equal("true"),
					korifiv1alpha1.CFRouteHostLabelKey:       Equal("example"),
					korifiv1alpha1.CFRoutePathLabelKey:       Equal("4757e8253d1d2e04aa277d3b9178cf69d8383d43fd9f894f9460ebda"), // SHA224 hash of "/example"
				}))
			}).Should(Succeed())
		})

		When("the route has destinations", func() {
			var app1GUID, app2GUID string

			BeforeEach(func() {
				app1GUID = uuid.NewString()
				app2GUID = uuid.NewString()

				route.Spec.Destinations = []korifiv1alpha1.Destination{
					{AppRef: corev1.LocalObjectReference{Name: app1GUID}},
					{AppRef: corev1.LocalObjectReference{Name: app2GUID}},
				}
			})

			It("adds labels per every destination app", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
					g.Expect(route.Labels).To(MatchKeys(IgnoreExtras, Keys{
						korifiv1alpha1.DestinationAppGUIDLabelPrefix + app1GUID: BeEmpty(),
						korifiv1alpha1.DestinationAppGUIDLabelPrefix + app2GUID: BeEmpty(),
						korifiv1alpha1.CFRouteIsUnmappedLabelKey:                Equal("false"),
					}))
				}).Should(Succeed())
			})

			When("the destination does not have app ref", func() {
				BeforeEach(func() {
					route.Spec.Destinations = []korifiv1alpha1.Destination{
						{GUID: "dest-guid"},
					}
				})

				It("does not add destination app labels", func() {
					Consistently(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
						g.Expect(maps.Keys(route.Labels)).NotTo(ContainElement(HavePrefix(korifiv1alpha1.DestinationAppGUIDLabelPrefix)))
					}).Should(Succeed())
				})
			})
		})
	})

	Describe("CFApp", func() {
		var app *korifiv1alpha1.CFApp

		BeforeEach(func() {
			app = &korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFAppSpec{
					DisplayName: "my-awesome-app",
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "docker",
					},
					DesiredState: "STOPPED",
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, app)).To(Succeed())
		})

		It("labels the CFApp with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(app), app)).To(Succeed())
				g.Expect(app.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.GUIDLabelKey:        Equal(app.Name),
					korifiv1alpha1.SpaceGUIDKey:        Equal(app.Namespace),
					korifiv1alpha1.CFAppDisplayNameKey: Equal("55c637753261f0925cbd0f3e595238aaf9c290fcf2efdbdf6e9b8bd4"), // SHA224 hash of "my-awesome-app"
				}))
			}).Should(Succeed())
		})
	})

	Describe("CFBuild", func() {
		var build *korifiv1alpha1.CFBuild

		BeforeEach(func() {
			build = &korifiv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{Name: uuid.NewString()},
					AppRef:     corev1.LocalObjectReference{Name: uuid.NewString()},
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "docker",
					},
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, build)).To(Succeed())
		})

		It("labels the CFBuild with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(build), build)).To(Succeed())
				g.Expect(build.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.SpaceGUIDKey:          Equal(build.Namespace),
					korifiv1alpha1.CFAppGUIDLabelKey:     Equal(build.Spec.AppRef.Name),
					korifiv1alpha1.CFPackageGUIDLabelKey: Equal(build.Spec.PackageRef.Name),
				}))
			}).Should(Succeed())
		})
	})

	Describe("CFDomain", func() {
		var domain *korifiv1alpha1.CFDomain

		BeforeEach(func() {
			domain = &korifiv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFDomainSpec{
					Name: "my.domain.com",
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, domain)).To(Succeed())
		})

		It("labels the CFDomain with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(domain), domain)).To(Succeed())
				g.Expect(domain.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.CFEncodedDomainNameLabelKey: Equal("0e4dd01876db0488e15402b99a19f9d8fea335c127fc626639e3375d"), // SHA224 hash of "my.domain.com"
				}))
			}).Should(Succeed())
		})
	})

	Describe("CFPackage", func() {
		var pkg *korifiv1alpha1.CFPackage

		BeforeEach(func() {
			pkg = &korifiv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFPackageSpec{
					AppRef: corev1.LocalObjectReference{Name: uuid.NewString()},
					Type:   "bits",
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, pkg)).To(Succeed())
		})

		It("labels the CFPackage with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(pkg), pkg)).To(Succeed())
				g.Expect(pkg.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.SpaceGUIDKey:      Equal(pkg.Namespace),
					korifiv1alpha1.CFAppGUIDLabelKey: Equal(pkg.Spec.AppRef.Name),
				}))
			}).Should(Succeed())
		})
	})

	Describe("CFProcess", func() {
		var process *korifiv1alpha1.CFProcess

		BeforeEach(func() {
			process = &korifiv1alpha1.CFProcess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, process)).To(Succeed())
		})

		It("labels the CFProcess with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(process), process)).To(Succeed())
				g.Expect(process.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.SpaceGUIDKey: Equal(process.Namespace),
				}))
			}).Should(Succeed())
		})
	})

	Describe("CFServiceInstance", func() {
		var instance *korifiv1alpha1.CFServiceInstance

		BeforeEach(func() {
			instance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					Type: "user-provided",
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, instance)).To(Succeed())
		})

		It("labels the CFServiceInstance with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(instance.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.SpaceGUIDKey: Equal(instance.Namespace),
				}))
			}).Should(Succeed())
		})
	})

	Describe("CFServiceBinding", func() {
		var binding *korifiv1alpha1.CFServiceBinding

		BeforeEach(func() {
			binding = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Type: "app",
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, binding)).To(Succeed())
		})

		It("labels the CFServiceBinding with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.SpaceGUIDKey: Equal(binding.Namespace),
				}))
			}).Should(Succeed())
		})
	})

	Describe("CFTask", func() {
		var task *korifiv1alpha1.CFTask

		BeforeEach(func() {
			task = &korifiv1alpha1.CFTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, task)).To(Succeed())
		})

		It("labels the CFTask with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(task), task)).To(Succeed())
				g.Expect(task.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.SpaceGUIDKey: Equal(task.Namespace),
				}))
			}).Should(Succeed())
		})
	})
})
