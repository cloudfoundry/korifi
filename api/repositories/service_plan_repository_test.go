package repositories_test

import (
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fakeawaiter"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServicePlanRepo", func() {
	var (
		repo     *repositories.ServicePlanRepo
		planGUID string
	)

	BeforeEach(func() {
		orgRepo := repositories.NewOrgRepo(rootNamespace, k8sClient, userClientFactory, nsPerms, &fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFOrg,
			korifiv1alpha1.CFOrg,
			korifiv1alpha1.CFOrgList,
			*korifiv1alpha1.CFOrgList,
		]{})
		repo = repositories.NewServicePlanRepo(userClientFactory, rootNamespace, orgRepo)

		planGUID = uuid.NewString()
		Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServicePlan{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      planGUID,
				Labels: map[string]string{
					korifiv1alpha1.RelServiceOfferingGUIDLabel: "offering-guid",
					korifiv1alpha1.RelServiceBrokerNameLabel:   "broker-name",
					korifiv1alpha1.RelServiceOfferingNameLabel: "offering-name",
					korifiv1alpha1.RelServiceBrokerGUIDLabel:   "broker-guid",
				},
				Annotations: map[string]string{
					"annotation": "annotation-value",
				},
			},
			Spec: korifiv1alpha1.CFServicePlanSpec{
				ServicePlan: services.ServicePlan{
					Name:        "my-service-plan",
					Free:        true,
					Description: "service plan description",
					BrokerCatalog: services.ServicePlanBrokerCatalog{
						ID: "broker-plan-guid",
						Metadata: &runtime.RawExtension{
							Raw: []byte(`{"foo":"bar"}`),
						},
						Features: services.ServicePlanFeatures{
							PlanUpdateable: true,
							Bindable:       true,
						},
					},
					Schemas: services.ServicePlanSchemas{
						ServiceInstance: services.ServiceInstanceSchema{
							Create: services.InputParameterSchema{
								Parameters: &runtime.RawExtension{
									Raw: []byte(`{"create-param":"create-value"}`),
								},
							},
							Update: services.InputParameterSchema{
								Parameters: &runtime.RawExtension{
									Raw: []byte(`{"update-param":"update-value"}`),
								},
							},
						},
						ServiceBinding: services.ServiceBindingSchema{
							Create: services.InputParameterSchema{
								Parameters: &runtime.RawExtension{
									Raw: []byte(`{"binding-create-param":"binding-create-value"}`),
								},
							},
						},
					},
				},
				Visibility: korifiv1alpha1.ServicePlanVisibility{
					Type: korifiv1alpha1.AdminServicePlanVisibilityType,
				},
			},
		})).To(Succeed())
	})

	Describe("Get", func() {
		var plan repositories.ServicePlanRecord

		JustBeforeEach(func() {
			var err error
			plan, err = repo.GetPlan(ctx, authInfo, planGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the plan", func() {
			Expect(plan).To(MatchFields(IgnoreExtras, Fields{
				"ServicePlan": MatchFields(IgnoreExtras, Fields{
					"Name":        Equal("my-service-plan"),
					"Description": Equal("service plan description"),
					"Free":        BeTrue(),
					"BrokerCatalog": MatchFields(IgnoreExtras, Fields{
						"ID": Equal("broker-plan-guid"),
						"Metadata": PointTo(MatchFields(IgnoreExtras, Fields{
							"Raw": MatchJSON(`{"foo": "bar"}`),
						})),

						"Features": MatchFields(IgnoreExtras, Fields{
							"PlanUpdateable": BeTrue(),
							"Bindable":       BeTrue(),
						}),
					}),
					"Schemas": MatchFields(IgnoreExtras, Fields{
						"ServiceInstance": MatchFields(IgnoreExtras, Fields{
							"Create": MatchFields(IgnoreExtras, Fields{
								"Parameters": PointTo(MatchFields(IgnoreExtras, Fields{
									"Raw": MatchJSON(`{"create-param":"create-value"}`),
								})),
							}),
							"Update": MatchFields(IgnoreExtras, Fields{
								"Parameters": PointTo(MatchFields(IgnoreExtras, Fields{
									"Raw": MatchJSON(`{"update-param":"update-value"}`),
								})),
							}),
						}),
						"ServiceBinding": MatchFields(IgnoreExtras, Fields{
							"Create": MatchFields(IgnoreExtras, Fields{
								"Parameters": PointTo(MatchFields(IgnoreExtras, Fields{
									"Raw": MatchJSON(`{"binding-create-param": "binding-create-value"}`),
								})),
							}),
						}),
					}),
				}),
				"CFResource": MatchFields(IgnoreExtras, Fields{
					"GUID":      Equal(planGUID),
					"CreatedAt": Not(BeZero()),
					"UpdatedAt": BeNil(),
					"Metadata": MatchAllFields(Fields{
						"Labels":      HaveKeyWithValue(korifiv1alpha1.RelServiceOfferingGUIDLabel, "offering-guid"),
						"Annotations": HaveKeyWithValue("annotation", "annotation-value"),
					}),
				}),
				"Visibility": MatchAllFields(Fields{
					"Type":          Equal(korifiv1alpha1.AdminServicePlanVisibilityType),
					"Organizations": BeEmpty(),
				}),
				"Available":           BeFalse(),
				"ServiceOfferingGUID": Equal("offering-guid"),
			}))
		})

		When("the visibility type is not admin", func() {
			BeforeEach(func() {
				cfServicePlan := &korifiv1alpha1.CFServicePlan{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      planGUID,
					},
				}
				Expect(k8s.PatchResource(ctx, k8sClient, cfServicePlan, func() {
					cfServicePlan.Spec.Visibility.Type = korifiv1alpha1.PublicServicePlanVisibilityType
				})).To(Succeed())
			})

			It("returns an available plan", func() {
				Expect(plan.Available).To(BeTrue())
			})
		})
	})

	Describe("List", func() {
		var (
			otherPlanGUID string
			listedPlans   []repositories.ServicePlanRecord
			message       repositories.ListServicePlanMessage
			listErr       error
		)

		BeforeEach(func() {
			otherPlanGUID = uuid.NewString()
			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServicePlan{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      otherPlanGUID,
					Labels: map[string]string{
						korifiv1alpha1.RelServiceOfferingGUIDLabel: "other-offering-guid",
						korifiv1alpha1.RelServiceBrokerNameLabel:   "other-broker-name",
						korifiv1alpha1.RelServiceOfferingNameLabel: "other-offering-name",
						korifiv1alpha1.RelServiceBrokerGUIDLabel:   "other-broker-guid",
					},
				},
				Spec: korifiv1alpha1.CFServicePlanSpec{
					Visibility: korifiv1alpha1.ServicePlanVisibility{
						Type: korifiv1alpha1.PublicServicePlanVisibilityType,
					},
					ServicePlan: services.ServicePlan{
						Name: "other-plan",
					},
				},
			})).To(Succeed())
			message = repositories.ListServicePlanMessage{}
		})

		JustBeforeEach(func() {
			listedPlans, listErr = repo.ListPlans(ctx, authInfo, message)
		})

		It("lists service offerings", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(listedPlans).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(planGUID),
					}),
				}), MatchFields(IgnoreExtras, Fields{
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(otherPlanGUID),
					}),
				}),
			))
		})

		When("filtering by service_offering_guid", func() {
			BeforeEach(func() {
				message.ServiceOfferingGUIDs = []string{"other-offering-guid"}
			})

			It("returns matching service plans", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedPlans).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"ServiceOfferingGUID": Equal("other-offering-guid"),
				})))
			})
		})

		When("filtering by guids", func() {
			BeforeEach(func() {
				message.GUIDs = []string{otherPlanGUID}
			})

			It("returns matching service plans", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedPlans).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"ServicePlan": MatchFields(IgnoreExtras, Fields{
						"Name": Equal("other-plan"),
					}),
				})))
			})
		})

		When("filtering by names", func() {
			BeforeEach(func() {
				message.Names = []string{"other-plan"}
			})

			It("returns matching service plans", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedPlans).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"ServicePlan": MatchFields(IgnoreExtras, Fields{
						"Name": Equal("other-plan"),
					}),
				})))
			})
		})

		When("filtering by availability", func() {
			BeforeEach(func() {
				message.Available = tools.PtrTo(true)
			})

			It("returns matching service plans", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedPlans).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(otherPlanGUID),
					}),
				})))
			})
		})

		When("filtering by broker name", func() {
			BeforeEach(func() {
				message.BrokerNames = []string{"other-broker-name"}
			})

			It("returns matching service plans", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedPlans).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(otherPlanGUID),
					}),
				})))
			})
		})

		When("filtering by broker guid", func() {
			BeforeEach(func() {
				message.BrokerGUIDs = []string{"other-broker-guid"}
			})

			It("returns matching service plans", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedPlans).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(otherPlanGUID),
					}),
				})))
			})
		})

		When("filtering by service offering name", func() {
			BeforeEach(func() {
				message.ServiceOfferingNames = []string{"other-offering-name"}
			})

			It("returns matching service plans", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedPlans).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(otherPlanGUID),
					}),
				})))
			})
		})
	})

	Describe("PlanVisibility", func() {
		var (
			visibilityType string
			orgGUIDs       []string
			cfOrg          *korifiv1alpha1.CFOrg
			anotherOrg     *korifiv1alpha1.CFOrg
			plan           repositories.ServicePlanRecord
			visibilityErr  error
		)

		BeforeEach(func() {
			cfOrg = createOrgWithCleanup(ctx, uuid.NewString())
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg.Name)

			anotherOrg = createOrgWithCleanup(ctx, uuid.NewString())
			createRoleBinding(ctx, userName, orgUserRole.Name, anotherOrg.Name)

			visibilityType = korifiv1alpha1.OrganizationServicePlanVisibilityType
			orgGUIDs = []string{cfOrg.Name}
		})

		Describe("ApplyPlanVisibility", func() {
			JustBeforeEach(func() {
				plan, visibilityErr = repo.ApplyPlanVisibility(ctx, authInfo, repositories.ApplyServicePlanVisibilityMessage{
					PlanGUID:      planGUID,
					Type:          visibilityType,
					Organizations: orgGUIDs,
				})
			})

			It("returns unauthorized error", func() {
				Expect(visibilityErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})

			When("the user has permissions", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
				})

				It("returns the patched plan visibility", func() {
					Expect(visibilityErr).NotTo(HaveOccurred())
					Expect(plan.Visibility).To(Equal(repositories.PlanVisibility{
						Type: korifiv1alpha1.OrganizationServicePlanVisibilityType,
						Organizations: []services.VisibilityOrganization{{
							GUID: cfOrg.Name,
							Name: cfOrg.Spec.DisplayName,
						}},
					}))
				})

				It("patches the plan visibility in kubernetes", func() {
					Expect(visibilityErr).NotTo(HaveOccurred())

					servicePlan := &korifiv1alpha1.CFServicePlan{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: rootNamespace,
							Name:      planGUID,
						},
					}
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)).To(Succeed())

					Expect(servicePlan.Spec.Visibility).To(Equal(korifiv1alpha1.ServicePlanVisibility{
						Type:          korifiv1alpha1.OrganizationServicePlanVisibilityType,
						Organizations: []string{cfOrg.Name},
					}))
				})

				When("the plan already has the org visibility type", func() {
					BeforeEach(func() {
						cfServicePlan := &korifiv1alpha1.CFServicePlan{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: rootNamespace,
								Name:      planGUID,
							},
						}

						Expect(k8s.PatchResource(ctx, k8sClient, cfServicePlan, func() {
							cfServicePlan.Spec.Visibility = korifiv1alpha1.ServicePlanVisibility{
								Type:          korifiv1alpha1.OrganizationServicePlanVisibilityType,
								Organizations: []string{anotherOrg.Name},
							}
						})).To(Succeed())
					})

					It("returns plan visibility with merged orgs", func() {
						Expect(visibilityErr).NotTo(HaveOccurred())
						Expect(plan.Visibility.Organizations).To(ConsistOf(
							services.VisibilityOrganization{
								GUID: anotherOrg.Name,
								Name: anotherOrg.Spec.DisplayName,
							},
							services.VisibilityOrganization{
								GUID: cfOrg.Name,
								Name: cfOrg.Spec.DisplayName,
							},
						))
					})

					It("patches the plan visibility in kubernetes by merging org guids", func() {
						Expect(visibilityErr).NotTo(HaveOccurred())

						servicePlan := &korifiv1alpha1.CFServicePlan{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: rootNamespace,
								Name:      planGUID,
							},
						}
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)).To(Succeed())

						Expect(servicePlan.Spec.Visibility.Organizations).To(ConsistOf(anotherOrg.Name, cfOrg.Name))
					})

					When("the visibility type is set to non-org", func() {
						BeforeEach(func() {
							visibilityType = korifiv1alpha1.AdminServicePlanVisibilityType
							orgGUIDs = []string{}
						})

						It("clears the visibility organizations in kubernetes", func() {
							Expect(visibilityErr).NotTo(HaveOccurred())

							servicePlan := &korifiv1alpha1.CFServicePlan{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: rootNamespace,
									Name:      planGUID,
								},
							}
							Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)).To(Succeed())

							Expect(servicePlan.Spec.Visibility.Organizations).To(BeEmpty())
						})
					})
				})

				When("the visibility org list contains duplicates", func() {
					BeforeEach(func() {
						orgGUIDs = []string{cfOrg.Name, cfOrg.Name}
					})

					It("deduplicates it", func() {
						Expect(visibilityErr).NotTo(HaveOccurred())

						servicePlan := &korifiv1alpha1.CFServicePlan{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: rootNamespace,
								Name:      planGUID,
							},
						}
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)).To(Succeed())

						Expect(servicePlan.Spec.Visibility.Organizations).To(ConsistOf(cfOrg.Name))
					})
				})
			})
		})

		Describe("UpdatePlanVisibility", func() {
			JustBeforeEach(func() {
				plan, visibilityErr = repo.UpdatePlanVisibility(ctx, authInfo, repositories.UpdateServicePlanVisibilityMessage{
					PlanGUID:      planGUID,
					Type:          visibilityType,
					Organizations: orgGUIDs,
				})
			})

			It("returns unauthorized error", func() {
				Expect(visibilityErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})

			When("the user has permissions", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
				})

				It("returns the patched plan visibility", func() {
					Expect(visibilityErr).NotTo(HaveOccurred())
					Expect(plan.Visibility).To(Equal(repositories.PlanVisibility{
						Type: korifiv1alpha1.OrganizationServicePlanVisibilityType,
						Organizations: []services.VisibilityOrganization{{
							GUID: cfOrg.Name,
							Name: cfOrg.Spec.DisplayName,
						}},
					}))
				})

				It("patches the plan visibility in kubernetes", func() {
					Expect(visibilityErr).NotTo(HaveOccurred())

					servicePlan := &korifiv1alpha1.CFServicePlan{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: rootNamespace,
							Name:      planGUID,
						},
					}
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)).To(Succeed())

					Expect(servicePlan.Spec.Visibility).To(Equal(korifiv1alpha1.ServicePlanVisibility{
						Type:          korifiv1alpha1.OrganizationServicePlanVisibilityType,
						Organizations: []string{cfOrg.Name},
					}))
				})

				When("the plan already has the org visibility type", func() {
					BeforeEach(func() {
						anotherOrg := createOrgWithCleanup(ctx, uuid.NewString())
						createRoleBinding(ctx, userName, orgUserRole.Name, anotherOrg.Name)

						cfServicePlan := &korifiv1alpha1.CFServicePlan{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: rootNamespace,
								Name:      planGUID,
							},
						}

						Expect(k8s.PatchResource(ctx, k8sClient, cfServicePlan, func() {
							cfServicePlan.Spec.Visibility = korifiv1alpha1.ServicePlanVisibility{
								Type:          korifiv1alpha1.OrganizationServicePlanVisibilityType,
								Organizations: []string{anotherOrg.Name},
							}
						})).To(Succeed())
					})

					It("returns plan visibility with replaced orgs", func() {
						Expect(visibilityErr).NotTo(HaveOccurred())
						Expect(plan.Visibility.Organizations).To(ConsistOf(services.VisibilityOrganization{
							GUID: cfOrg.Name,
							Name: cfOrg.Spec.DisplayName,
						}))
					})

					It("patches the plan visibility in kubernetes by replacing org guids", func() {
						Expect(visibilityErr).NotTo(HaveOccurred())

						servicePlan := &korifiv1alpha1.CFServicePlan{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: rootNamespace,
								Name:      planGUID,
							},
						}
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)).To(Succeed())

						Expect(servicePlan.Spec.Visibility.Organizations).To(ConsistOf(cfOrg.Name))
					})
				})

				When("the visibility org list contains duplicates", func() {
					BeforeEach(func() {
						orgGUIDs = []string{cfOrg.Name, cfOrg.Name}
					})

					It("deduplicates it", func() {
						Expect(visibilityErr).NotTo(HaveOccurred())

						servicePlan := &korifiv1alpha1.CFServicePlan{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: rootNamespace,
								Name:      planGUID,
							},
						}
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)).To(Succeed())

						Expect(servicePlan.Spec.Visibility.Organizations).To(ConsistOf(cfOrg.Name))
					})
				})
			})
		})
	})
})
