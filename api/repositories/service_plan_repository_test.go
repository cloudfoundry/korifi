package repositories_test

import (
	"context"
	"errors"
	"fmt"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	"code.cloudfoundry.org/korifi/api/repositories/fakeawaiter"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/gomega/gstruct"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServicePlanRepo", func() {
	var (
		repo     *repositories.ServicePlanRepo
		orgRepo  *repositories.OrgRepo
		planGUID string
	)

	BeforeEach(func() {
		orgRepo = repositories.NewOrgRepo(rootNSKlient, rootNamespace, nsPerms, &fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFOrg,
			korifiv1alpha1.CFOrgList,
			*korifiv1alpha1.CFOrgList,
		]{})
		repo = repositories.NewServicePlanRepo(rootNSKlient, rootNamespace, orgRepo)

		planGUID = uuid.NewString()
		metadata, err := korifiv1alpha1.AsRawExtension(map[string]any{
			"foo": "bar",
		})
		Expect(err).NotTo(HaveOccurred())
		instanceCreateParameters, err := korifiv1alpha1.AsRawExtension(map[string]any{
			"create-param": "create-value",
		})
		Expect(err).NotTo(HaveOccurred())
		instanceUpdateParameters, err := korifiv1alpha1.AsRawExtension(map[string]any{
			"update-param": "update-value",
		})
		Expect(err).NotTo(HaveOccurred())
		bindingCreateParameters, err := korifiv1alpha1.AsRawExtension(map[string]any{
			"binding-create-param": "binding-create-value",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServicePlan{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      planGUID,
				Labels: map[string]string{
					korifiv1alpha1.RelServiceOfferingGUIDLabel: "offering-guid",
					korifiv1alpha1.RelServiceBrokerNameLabel:   tools.EncodeValueToSha224("broker-name"),
					korifiv1alpha1.RelServiceOfferingNameLabel: tools.EncodeValueToSha224("offering-name"),
					korifiv1alpha1.RelServiceBrokerGUIDLabel:   "broker-guid",
				},
				Annotations: map[string]string{
					"annotation": "annotation-value",
				},
			},
			Spec: korifiv1alpha1.CFServicePlanSpec{
				Name:        "my-service-plan",
				Free:        true,
				Description: "service plan description",
				BrokerCatalog: korifiv1alpha1.ServicePlanBrokerCatalog{
					ID:       "broker-plan-guid",
					Metadata: metadata,
					Features: korifiv1alpha1.ServicePlanFeatures{
						PlanUpdateable: true,
						Bindable:       true,
					},
				},
				Schemas: korifiv1alpha1.ServicePlanSchemas{
					ServiceInstance: korifiv1alpha1.ServiceInstanceSchema{
						Create: korifiv1alpha1.InputParameterSchema{
							Parameters: instanceCreateParameters,
						},
						Update: korifiv1alpha1.InputParameterSchema{
							Parameters: instanceUpdateParameters,
						},
					},
					ServiceBinding: korifiv1alpha1.ServiceBindingSchema{
						Create: korifiv1alpha1.InputParameterSchema{
							Parameters: bindingCreateParameters,
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
				"Name":        Equal("my-service-plan"),
				"Description": Equal("service plan description"),
				"Free":        BeTrue(),
				"BrokerCatalog": MatchFields(IgnoreExtras, Fields{
					"ID": Equal("broker-plan-guid"),
					"Metadata": MatchAllKeys(Keys{
						"foo": Equal("bar"),
					}),
					"Features": MatchFields(IgnoreExtras, Fields{
						"PlanUpdateable": BeTrue(),
						"Bindable":       BeTrue(),
					}),
				}),
				"Schemas": MatchFields(IgnoreExtras, Fields{
					"ServiceInstance": MatchFields(IgnoreExtras, Fields{
						"Create": MatchFields(IgnoreExtras, Fields{
							"Parameters": MatchAllKeys(Keys{
								"create-param": Equal("create-value"),
							}),
						}),
						"Update": MatchFields(IgnoreExtras, Fields{
							"Parameters": MatchAllKeys(Keys{
								"update-param": Equal("update-value"),
							}),
						}),
					}),
					"ServiceBinding": MatchFields(IgnoreExtras, Fields{
						"Create": MatchFields(IgnoreExtras, Fields{
							"Parameters": MatchAllKeys(Keys{
								"binding-create-param": Equal("binding-create-value"),
							}),
						}),
					}),
				}),
				"GUID":      Equal(planGUID),
				"CreatedAt": Not(BeZero()),
				"UpdatedAt": BeNil(),
				"Metadata": MatchAllFields(Fields{
					"Labels":      HaveKeyWithValue(korifiv1alpha1.RelServiceOfferingGUIDLabel, "offering-guid"),
					"Annotations": HaveKeyWithValue("annotation", "annotation-value"),
				}),
				"Visibility": MatchAllFields(Fields{
					"Type":          Equal(korifiv1alpha1.AdminServicePlanVisibilityType),
					"Organizations": BeEmpty(),
				}),
				"Available":           BeFalse(),
				"ServiceOfferingGUID": Equal("offering-guid"),
			}))
		})

		When("the plan is available", func() {
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
						korifiv1alpha1.RelServiceBrokerNameLabel:   tools.EncodeValueToSha224("other-broker-name"),
						korifiv1alpha1.RelServiceOfferingNameLabel: tools.EncodeValueToSha224("other-offering-name"),
						korifiv1alpha1.RelServiceBrokerGUIDLabel:   "other-broker-guid",
					},
				},
				Spec: korifiv1alpha1.CFServicePlanSpec{
					Visibility: korifiv1alpha1.ServicePlanVisibility{
						Type: korifiv1alpha1.PublicServicePlanVisibilityType,
					},
					Name: "other-plan",
				},
			})).To(Succeed())
			message = repositories.ListServicePlanMessage{}
		})

		JustBeforeEach(func() {
			listedPlans, listErr = repo.ListPlans(ctx, authInfo, message)
		})

		It("lists service plans", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(listedPlans).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"GUID": Equal(planGUID),
				}), MatchFields(IgnoreExtras, Fields{
					"GUID": Equal(otherPlanGUID),
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

		Describe("filter parameters to list options", func() {
			var fakeKlient *fake.Klient

			BeforeEach(func() {
				fakeKlient = new(fake.Klient)
				repo = repositories.NewServicePlanRepo(fakeKlient, rootNamespace, orgRepo)
				message = repositories.ListServicePlanMessage{
					GUIDs:                []string{"g1", "g2"},
					Names:                []string{"n1", "n2"},
					ServiceOfferingGUIDs: []string{"sog1", "sog2"},
					ServiceOfferingNames: []string{"son1", "son2"},
					BrokerNames:          []string{"bn1", "bn2"},
					BrokerGUIDs:          []string{"bg1", "bg2"},
				}
			})

			It("translates filter parameters to klient list options", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(fakeKlient.ListCallCount()).To(Equal(1))
				_, _, listOptions := fakeKlient.ListArgsForCall(0)
				Expect(listOptions).To(ConsistOf(
					repositories.WithLabelIn(korifiv1alpha1.GUIDLabelKey, []string{"g1", "g2"}),
					repositories.WithLabelIn(korifiv1alpha1.CFServicePlanNameKey, tools.EncodeValuesToSha224("n1", "n2")),
					repositories.WithLabelIn(korifiv1alpha1.RelServiceOfferingGUIDLabel, []string{"sog1", "sog2"}),
					repositories.WithLabelIn(korifiv1alpha1.RelServiceBrokerNameLabel, tools.EncodeValuesToSha224("bn1", "bn2")),
					repositories.WithLabelIn(korifiv1alpha1.RelServiceBrokerGUIDLabel, []string{"bg1", "bg2"}),
					repositories.WithLabelIn(korifiv1alpha1.RelServiceOfferingNameLabel, tools.EncodeValuesToSha224("son1", "son2")),
					repositories.NoopListOption{},
				))
			})

			When("filtering by available service plans", func() {
				BeforeEach(func() {
					message.Available = tools.PtrTo(true)
				})

				It("translates the available parameters field to a list options", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(fakeKlient.ListCallCount()).To(Equal(1))
					_, _, listOptions := fakeKlient.ListArgsForCall(0)
					Expect(listOptions).To(ContainElement(
						repositories.WithLabel(korifiv1alpha1.CFServicePlanAvailableKey, "true"),
					))
				})
			})

			When("filtering by unavailable service plans", func() {
				BeforeEach(func() {
					message.Available = tools.PtrTo(false)
				})

				It("translates the available parameters field to a list options", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(fakeKlient.ListCallCount()).To(Equal(1))
					_, _, listOptions := fakeKlient.ListArgsForCall(0)
					Expect(listOptions).To(ContainElement(
						repositories.WithLabel(korifiv1alpha1.CFServicePlanAvailableKey, "false"),
					))
				})
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
						Organizations: []repositories.VisibilityOrganization{{
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
							repositories.VisibilityOrganization{
								GUID: anotherOrg.Name,
								Name: anotherOrg.Spec.DisplayName,
							},
							repositories.VisibilityOrganization{
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
						Organizations: []repositories.VisibilityOrganization{{
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
						anotherOrg = createOrgWithCleanup(ctx, uuid.NewString())
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
						Expect(plan.Visibility.Organizations).To(ConsistOf(repositories.VisibilityOrganization{
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

		Describe("DeletePlanVisibility", func() {
			var deleteMessage repositories.DeleteServicePlanVisibilityMessage

			BeforeEach(func() {
				cfServicePlan := &korifiv1alpha1.CFServicePlan{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      planGUID,
					},
				}
				Expect(k8s.PatchResource(ctx, k8sClient, cfServicePlan, func() {
					cfServicePlan.Spec.Visibility.Type = visibilityType
					cfServicePlan.Spec.Visibility.Organizations = []string{cfOrg.Name, anotherOrg.Name}
				})).To(Succeed())

				deleteMessage = repositories.DeleteServicePlanVisibilityMessage{
					PlanGUID: planGUID,
					OrgGUID:  anotherOrg.Name,
				}
			})

			JustBeforeEach(func() {
				visibilityErr = repo.DeletePlanVisibility(ctx, authInfo, deleteMessage)
			})

			When("the user is not authorized", func() {
				It("returns unauthorized error", func() {
					Expect(visibilityErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})
			})

			When("The user has persmissions", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
				})

				When("the plan and org visibility exist", func() {
					It("deletes the plan visibility in kubernetes", func() {
						Expect(visibilityErr).ToNot(HaveOccurred())

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

				When("the plan does not exist", func() {
					BeforeEach(func() {
						deleteMessage.PlanGUID = "does-not-exist"
					})

					It("returns an NotFoundError", func() {
						Expect(errors.As(visibilityErr, &apierrors.NotFoundError{})).To(BeTrue())
					})
				})

				When("the org does not exist", func() {
					BeforeEach(func() {
						deleteMessage.OrgGUID = "does-not-exist"
					})

					It("does not change the visibility orgs", func() {
						Expect(visibilityErr).ToNot(HaveOccurred())

						servicePlan := &korifiv1alpha1.CFServicePlan{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: rootNamespace,
								Name:      planGUID,
							},
						}
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(servicePlan), servicePlan)).To(Succeed())
						Expect(servicePlan.Spec.Visibility.Organizations).To(ConsistOf(cfOrg.Name, anotherOrg.Name))
					})
				})
			})
		})
	})

	Describe("Delete", func() {
		var deleteErr error

		JustBeforeEach(func() {
			createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			deleteErr = repo.DeletePlan(ctx, authInfo, planGUID)
		})

		It("deletes the service plan", func() {
			Expect(deleteErr).ToNot(HaveOccurred())

			namespacedName := types.NamespacedName{
				Name:      planGUID,
				Namespace: rootNamespace,
			}

			err := k8sClient.Get(context.Background(), namespacedName, &korifiv1alpha1.CFServicePlan{})
			Expect(k8serrors.IsNotFound(err)).To(BeTrue(), fmt.Sprintf("error: %+v", err))
		})

		When("the service plan does not exist", func() {
			BeforeEach(func() {
				planGUID = "does-not-exist"
			})

			It("returns a not found error", func() {
				Expect(errors.As(deleteErr, &apierrors.NotFoundError{})).To(BeTrue())
			})
		})
	})
})
