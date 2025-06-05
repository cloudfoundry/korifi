package label_indexer_test

import (
	"maps"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
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
			Expect(k8s.Patch(ctx, adminClient, build, func() {
				build.Status.State = korifiv1alpha1.BuildStateStaged
			})).To(Succeed())
		})

		It("labels the CFBuild with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(build), build)).To(Succeed())
				g.Expect(build.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.SpaceGUIDKey:          Equal(build.Namespace),
					korifiv1alpha1.CFDropletGUIDLabelKey: Equal(build.Name),
					korifiv1alpha1.CFAppGUIDLabelKey:     Equal(build.Spec.AppRef.Name),
					korifiv1alpha1.CFPackageGUIDLabelKey: Equal(build.Spec.PackageRef.Name),
					korifiv1alpha1.CFBuildStateLabelKey:  Equal(korifiv1alpha1.BuildStateStaged),
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
					Type:     "user-provided",
					PlanGUID: uuid.NewString(),
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
					korifiv1alpha1.SpaceGUIDKey:     Equal(instance.Namespace),
					korifiv1alpha1.PlanGUIDLabelKey: Equal(instance.Spec.PlanGUID),
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
					Service: corev1.ObjectReference{
						Name: uuid.NewString(),
					},
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
					korifiv1alpha1.SpaceGUIDKey:                  Equal(binding.Namespace),
					korifiv1alpha1.CFServiceInstanceGUIDLabelKey: Equal(binding.Spec.Service.Name),
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

	Describe("CFOrg", func() {
		var org *korifiv1alpha1.CFOrg

		BeforeEach(func() {
			org = &korifiv1alpha1.CFOrg{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFOrgSpec{
					DisplayName: "my-awesome-org",
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, org)).To(Succeed())
			Expect(k8s.Patch(ctx, adminClient, org, func() {
				org.Status.GUID = uuid.NewString()
				meta.SetStatusCondition(&org.Status.Conditions, metav1.Condition{
					Type:    korifiv1alpha1.StatusConditionReady,
					Status:  metav1.ConditionTrue,
					Reason:  "Ready",
					Message: "Reconciled",
				})
			})).To(Succeed())
		})

		It("labels the CFOrg with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(org), org)).To(Succeed())
				g.Expect(org.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.CFOrgDisplayNameKey: Equal("02305ec6ce38ed4d9b654b58bca88c90a824558ecafac09ecc3acbae"), // SHA224 hash of "my-awesome-org"
					korifiv1alpha1.GUIDLabelKey:        Equal(org.Name),
					korifiv1alpha1.ReadyLabelKey:       Equal("True"),
				}))
			}).Should(Succeed())
		})
	})

	Describe("CFSpace", func() {
		var space *korifiv1alpha1.CFSpace

		BeforeEach(func() {
			space = &korifiv1alpha1.CFSpace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFSpaceSpec{
					DisplayName: "my-awesome-space",
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, space)).To(Succeed())
			Expect(k8s.Patch(ctx, adminClient, space, func() {
				space.Status.GUID = uuid.NewString()
				meta.SetStatusCondition(&space.Status.Conditions, metav1.Condition{
					Type:    korifiv1alpha1.StatusConditionReady,
					Status:  metav1.ConditionTrue,
					Reason:  "Ready",
					Message: "Reconciled",
				})
			})).To(Succeed())
		})

		It("labels the CFSpace with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(space), space)).To(Succeed())
				g.Expect(space.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.CFSpaceDisplayNameKey: Equal("e2ff2d1641634dc2e43603c6e294926801d785b9715ccd61a26c2aca"), // SHA224 hash of "my-awesome-space"
					korifiv1alpha1.GUIDLabelKey:          Equal(space.Name),
					korifiv1alpha1.CFOrgGUIDKey:          Equal(space.Namespace),
					korifiv1alpha1.ReadyLabelKey:         Equal("True"),
				}))
			}).Should(Succeed())
		})
	})

	Describe("CFServiceOffering", func() {
		var offering *korifiv1alpha1.CFServiceOffering

		BeforeEach(func() {
			offering = &korifiv1alpha1.CFServiceOffering{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFServiceOfferingSpec{
					Name: "my-awesome-offering",
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, offering)).To(Succeed())
		})

		It("labels the CFServiceOffering with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(offering), offering)).To(Succeed())
				g.Expect(offering.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.GUIDLabelKey:             Equal(offering.Name),
					korifiv1alpha1.CFServiceOfferingNameKey: Equal("dfcda624de1c07d0ecde233a93cccc75171934a53876dd616f321cae"), // SHA224 hash of "my-awesome-offering"
				}))
			}).Should(Succeed())
		})
	})

	Describe("CFServicePlan", func() {
		var plan *korifiv1alpha1.CFServicePlan

		BeforeEach(func() {
			plan = &korifiv1alpha1.CFServicePlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFServicePlanSpec{
					Name: "my-awesome-plan",
					Visibility: korifiv1alpha1.ServicePlanVisibility{
						Type: korifiv1alpha1.PublicServicePlanVisibilityType,
					},
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, plan)).To(Succeed())
		})

		It("labels the CFServicePlan with the expected index labels", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(plan), plan)).To(Succeed())
				g.Expect(plan.Labels).To(MatchKeys(IgnoreExtras, Keys{
					korifiv1alpha1.GUIDLabelKey:              Equal(plan.Name),
					korifiv1alpha1.CFServicePlanNameKey:      Equal("17f65f9393e1826ece32c78a186b397b259ae4317ada0891021d9ea9"), // SHA224 hash of "my-awesome-plan"
					korifiv1alpha1.CFServicePlanAvailableKey: Equal("true"),
				}))
			}).Should(Succeed())
		})

		When("the plan visibility is org", func() {
			BeforeEach(func() {
				plan.Spec.Visibility.Type = korifiv1alpha1.OrganizationServicePlanVisibilityType
			})

			It("labels the CFServicePlan as available", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(plan), plan)).To(Succeed())
					g.Expect(plan.Labels).To(MatchKeys(IgnoreExtras, Keys{
						korifiv1alpha1.CFServicePlanAvailableKey: Equal("true"),
					}))
				}).Should(Succeed())
			})
		})

		When("the plan visibility is admin", func() {
			BeforeEach(func() {
				plan.Spec.Visibility.Type = korifiv1alpha1.AdminServicePlanVisibilityType
			})

			It("labels the CFServicePlan as unavailable", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(plan), plan)).To(Succeed())
					g.Expect(plan.Labels).To(MatchKeys(IgnoreExtras, Keys{
						korifiv1alpha1.CFServicePlanAvailableKey: Equal("false"),
					}))
				}).Should(Succeed())
			})
		})
	})
})
