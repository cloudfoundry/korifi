package repositories_test

import (
	"context"
	"errors"
	"fmt"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceOfferingRepo", func() {
	var (
		repo  *repositories.ServiceOfferingRepo
		org   *korifiv1alpha1.CFOrg
		space *korifiv1alpha1.CFSpace
	)

	BeforeEach(func() {
		repo = repositories.NewServiceOfferingRepo(
			rootNSKlient,
			spaceScopedKlient,
			rootNamespace,
		)

		org = createOrgWithCleanup(ctx, uuid.NewString())
		space = createSpaceWithCleanup(ctx, org.Name, uuid.NewString())
	})

	Describe("Get", func() {
		var (
			offeringGUID    string
			broker          *korifiv1alpha1.CFServiceBroker
			desiredOffering repositories.ServiceOfferingRecord
			getErr          error
		)

		BeforeEach(func() {
			offeringGUID = uuid.NewString()

			brokerGUID := uuid.NewString()
			broker = &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      brokerGUID,
				},
				Spec: korifiv1alpha1.CFServiceBrokerSpec{
					Name: uuid.NewString(),
				},
			}
			Expect(k8sClient.Create(ctx, broker)).To(Succeed())

			metadata, err := korifiv1alpha1.AsRawExtension(map[string]any{
				"offering-md": "offering-md-value",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceOffering{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      offeringGUID,
					Labels: map[string]string{
						korifiv1alpha1.RelServiceBrokerGUIDLabel: broker.Name,
						korifiv1alpha1.RelServiceBrokerNameLabel: tools.EncodeValueToSha224(broker.Spec.Name),
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				},
				Spec: korifiv1alpha1.CFServiceOfferingSpec{
					Name:             "my-offering",
					Description:      "my offering description",
					Tags:             []string{"t1"},
					Requires:         []string{"r1"},
					DocumentationURL: tools.PtrTo("https://my.offering.com"),
					BrokerCatalog: korifiv1alpha1.ServiceBrokerCatalog{
						ID:       "offering-catalog-guid",
						Metadata: metadata,
						Features: korifiv1alpha1.BrokerCatalogFeatures{
							PlanUpdateable:       true,
							Bindable:             true,
							InstancesRetrievable: true,
							BindingsRetrievable:  true,
							AllowContextUpdates:  true,
						},
					},
				},
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			desiredOffering, getErr = repo.GetServiceOffering(ctx, authInfo, offeringGUID)
		})

		It("gets the service offering", func() {
			Expect(getErr).NotTo(HaveOccurred())
			Expect(desiredOffering).To(
				MatchFields(IgnoreExtras, Fields{
					"Name":             Equal("my-offering"),
					"Description":      Equal("my offering description"),
					"Tags":             ConsistOf("t1"),
					"Requires":         ConsistOf("r1"),
					"DocumentationURL": PointTo(Equal("https://my.offering.com")),
					"BrokerCatalog": MatchFields(IgnoreExtras, Fields{
						"ID": Equal("offering-catalog-guid"),
						"Metadata": MatchAllKeys(Keys{
							"offering-md": Equal("offering-md-value"),
						}),
						"Features": MatchFields(IgnoreExtras, Fields{
							"PlanUpdateable":       BeTrue(),
							"Bindable":             BeTrue(),
							"InstancesRetrievable": BeTrue(),
							"BindingsRetrievable":  BeTrue(),
							"AllowContextUpdates":  BeTrue(),
						}),
					}),
					"GUID":      Equal(offeringGUID),
					"CreatedAt": Not(BeZero()),
					"UpdatedAt": BeNil(),
					"Metadata": MatchAllFields(Fields{
						"Labels":      HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerGUIDLabel, broker.Name),
						"Annotations": HaveKeyWithValue("annotation", "annotation-value"),
					}),
					"ServiceBrokerGUID": Equal(broker.Name),
				}),
			)
		})

		When("the service offering does not exist", func() {
			BeforeEach(func() {
				offeringGUID = "does-not-exist"
			})
			It("returns a not found error", func() {
				notFoundError := apierrors.NotFoundError{}
				Expect(errors.As(getErr, &notFoundError)).To(BeTrue())
			})
		})
	})

	Describe("List", func() {
		var (
			offeringGUID        string
			anotherOfferingGUID string
			broker              *korifiv1alpha1.CFServiceBroker
			listedOfferings     []repositories.ServiceOfferingRecord
			message             repositories.ListServiceOfferingMessage
			listErr             error
		)

		BeforeEach(func() {
			offeringGUID = uuid.NewString()
			anotherOfferingGUID = uuid.NewString()

			brokerGUID := uuid.NewString()
			broker = &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      brokerGUID,
				},
				Spec: korifiv1alpha1.CFServiceBrokerSpec{
					Name: uuid.NewString(),
				},
			}
			Expect(k8sClient.Create(ctx, broker)).To(Succeed())

			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceOffering{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      offeringGUID,
					Labels: map[string]string{
						korifiv1alpha1.RelServiceBrokerGUIDLabel: broker.Name,
						korifiv1alpha1.RelServiceBrokerNameLabel: tools.EncodeValueToSha224(broker.Spec.Name),
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				},
				Spec: korifiv1alpha1.CFServiceOfferingSpec{
					Name:             "my-offering",
					Description:      "my offering description",
					Tags:             []string{"t1"},
					Requires:         []string{"r1"},
					DocumentationURL: tools.PtrTo("https://my.offering.com"),
					BrokerCatalog: korifiv1alpha1.ServiceBrokerCatalog{
						ID: "offering-catalog-guid",
						Metadata: &runtime.RawExtension{
							Raw: []byte(`{"offering-md": "offering-md-value"}`),
						},
						Features: korifiv1alpha1.BrokerCatalogFeatures{
							PlanUpdateable:       true,
							Bindable:             true,
							InstancesRetrievable: true,
							BindingsRetrievable:  true,
							AllowContextUpdates:  true,
						},
					},
				},
			})).To(Succeed())

			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceOffering{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      anotherOfferingGUID,
					Labels: map[string]string{
						korifiv1alpha1.RelServiceBrokerGUIDLabel: "another-broker",
						korifiv1alpha1.RelServiceBrokerNameLabel: "another-broker-name",
					},
				},
				Spec: korifiv1alpha1.CFServiceOfferingSpec{
					Name: "another-offering",
				},
			})).To(Succeed())

			message = repositories.ListServiceOfferingMessage{}
		})

		JustBeforeEach(func() {
			listedOfferings, listErr = repo.ListOfferings(ctx, authInfo, message)
		})

		It("lists service offerings", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(listedOfferings).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"Name":             Equal("my-offering"),
					"Description":      Equal("my offering description"),
					"Tags":             ConsistOf("t1"),
					"Requires":         ConsistOf("r1"),
					"DocumentationURL": PointTo(Equal("https://my.offering.com")),
					"BrokerCatalog": MatchFields(IgnoreExtras, Fields{
						"ID": Equal("offering-catalog-guid"),
						"Metadata": MatchAllKeys(Keys{
							"offering-md": Equal("offering-md-value"),
						}),
						"Features": MatchFields(IgnoreExtras, Fields{
							"PlanUpdateable":       BeTrue(),
							"Bindable":             BeTrue(),
							"InstancesRetrievable": BeTrue(),
							"BindingsRetrievable":  BeTrue(),
							"AllowContextUpdates":  BeTrue(),
						}),
					}),
					"GUID":      Equal(offeringGUID),
					"CreatedAt": Not(BeZero()),
					"UpdatedAt": BeNil(),
					"Metadata": MatchAllFields(Fields{
						"Labels":      HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerGUIDLabel, broker.Name),
						"Annotations": HaveKeyWithValue("annotation", "annotation-value"),
					}),
					"ServiceBrokerGUID": Equal(broker.Name),
				}),
				MatchFields(IgnoreExtras, Fields{
					"GUID": Equal(anotherOfferingGUID),
				}),
			))
		})

		When("filtering by name", func() {
			BeforeEach(func() {
				message.Names = []string{"my-offering"}
			})

			It("returns the matching offerings", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedOfferings).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Name": Equal("my-offering"),
				})))
			})
		})

		Describe("filter parameters to list options", func() {
			var fakeKlient *fake.Klient

			BeforeEach(func() {
				fakeKlient = new(fake.Klient)
				repo = repositories.NewServiceOfferingRepo(
					fakeKlient,
					nil,
					rootNamespace,
				)
				message = repositories.ListServiceOfferingMessage{
					Names:       []string{"n1", "n2"},
					GUIDs:       []string{"g1", "g2"},
					BrokerNames: []string{"b1", "b2"},
				}
			})

			It("translates filter parameters to klient list options", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(fakeKlient.ListCallCount()).To(Equal(1))
				_, _, listOptions := fakeKlient.ListArgsForCall(0)
				Expect(listOptions).To(ConsistOf(
					repositories.WithLabelIn(korifiv1alpha1.CFServiceOfferingNameKey, tools.EncodeValuesToSha224("n1", "n2")),
					repositories.WithLabelIn(korifiv1alpha1.GUIDLabelKey, []string{"g1", "g2"}),
					repositories.WithLabelIn(korifiv1alpha1.RelServiceBrokerNameLabel, tools.EncodeValuesToSha224("b1", "b2")),
				))
			})
		})
	})

	Describe("DeleteOffering", func() {
		var (
			plan      *korifiv1alpha1.CFServicePlan
			offering  *korifiv1alpha1.CFServiceOffering
			instance  *korifiv1alpha1.CFServiceInstance
			binding   *korifiv1alpha1.CFServiceBinding
			message   repositories.DeleteServiceOfferingMessage
			deleteErr error
		)

		BeforeEach(func() {
			offering = &korifiv1alpha1.CFServiceOffering{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceOfferingSpec{
					Name:        "my-offering",
					Description: "my offering description",
					Tags:        []string{"t1"},
					Requires:    []string{"r1"},
				},
			}
			Expect(k8sClient.Create(ctx, offering)).To(Succeed())

			plan = &korifiv1alpha1.CFServicePlan{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
					Labels: map[string]string{
						korifiv1alpha1.RelServiceOfferingGUIDLabel: offering.Name,
					},
				},
				Spec: korifiv1alpha1.CFServicePlanSpec{
					Name:        "my-service-plan",
					Free:        true,
					Description: "service plan description",
					Visibility: korifiv1alpha1.ServicePlanVisibility{
						Type: korifiv1alpha1.PublicServicePlanVisibilityType,
					},
				},
			}
			Expect(k8sClient.Create(ctx, plan)).To(Succeed())

			instance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space.Name,
					Name:      uuid.NewString(),
					Finalizers: []string{
						korifiv1alpha1.CFServiceInstanceFinalizerName,
					},
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					PlanGUID: plan.Name,
					Type:     "user-provided",
				},
			}
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			binding = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space.Name,
					Finalizers: []string{
						korifiv1alpha1.CFServiceBindingFinalizerName,
					},
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "CFServiceInstance",
						APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
						Name:       instance.Name,
					},
					AppRef: corev1.LocalObjectReference{
						Name: "some-app-guid",
					},
					Type: korifiv1alpha1.CFServiceBindingTypeApp,
				},
			}
			Expect(k8sClient.Create(ctx, binding)).To(Succeed())

			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)

			message = repositories.DeleteServiceOfferingMessage{GUID: offering.Name}
		})

		JustBeforeEach(func() {
			deleteErr = repo.DeleteOffering(ctx, authInfo, message)
		})

		It("successfully deletes the offering", func() {
			Expect(deleteErr).ToNot(HaveOccurred())

			namespacedName := types.NamespacedName{
				Name:      offering.Name,
				Namespace: rootNamespace,
			}

			err := k8sClient.Get(context.Background(), namespacedName, &korifiv1alpha1.CFServiceOffering{})
			Expect(k8serrors.IsNotFound(err)).To(BeTrue(), fmt.Sprintf("error: %+v", err))
		})

		When("the service offering does not exist", func() {
			BeforeEach(func() {
				message.GUID = "does-not-exist"
			})

			It("returns a error", func() {
				Expect(errors.As(deleteErr, &apierrors.NotFoundError{})).To(BeTrue())
			})
		})

		When("Purge is set to true", func() {
			BeforeEach(func() {
				message.Purge = true
			})

			It("successfully deletes the offering and all related resources", func() {
				Expect(deleteErr).ToNot(HaveOccurred())

				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: offering.Name, Namespace: rootNamespace}, &korifiv1alpha1.CFServiceOffering{})
				Expect(k8serrors.IsNotFound(err)).To(BeTrue(), fmt.Sprintf("error: %+v", err))

				err = k8sClient.Get(context.Background(), types.NamespacedName{Name: plan.Name, Namespace: rootNamespace}, &korifiv1alpha1.CFServicePlan{})
				Expect(k8serrors.IsNotFound(err)).To(BeTrue(), fmt.Sprintf("error: %+v", err))

				err = k8sClient.Get(context.Background(), types.NamespacedName{Name: instance.Name, Namespace: space.Name}, &korifiv1alpha1.CFServiceInstance{})
				Expect(k8serrors.IsNotFound(err)).To(BeTrue(), fmt.Sprintf("error: %+v", err))

				serviceBinding := new(korifiv1alpha1.CFServiceBinding)
				err = k8sClient.Get(context.Background(), types.NamespacedName{Name: binding.Name, Namespace: space.Name}, serviceBinding)

				Expect(err).ToNot(HaveOccurred())
				Expect(serviceBinding.Finalizers).To(BeEmpty())
			})
		})
	})

	Describe("UpdateServiceOffering", func() {
		var (
			offeringGUID           string
			brokerGUID             string
			serviceOffering        *korifiv1alpha1.CFServiceOffering
			updatedServiceOffering repositories.ServiceOfferingRecord
			updateErr              error
			updateMessage          repositories.UpdateServiceOfferingMessage
		)

		BeforeEach(func() {
			offeringGUID = uuid.NewString()
			brokerGUID = uuid.NewString()
			offeringName := uuid.NewString()

			metadata, err := korifiv1alpha1.AsRawExtension(map[string]any{
				"offering-md": "offering-md-value",
			})

			Expect(err).NotTo(HaveOccurred())
			serviceOffering = &korifiv1alpha1.CFServiceOffering{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      offeringGUID,
					Labels: map[string]string{
						korifiv1alpha1.RelServiceBrokerGUIDLabel: brokerGUID,
						korifiv1alpha1.RelServiceBrokerNameLabel: tools.EncodeValueToSha224("my-broker"),
						korifiv1alpha1.CFServiceOfferingNameKey:  tools.EncodeValueToSha224(offeringName),
						korifiv1alpha1.GUIDLabelKey:              offeringGUID,
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				},
				Spec: korifiv1alpha1.CFServiceOfferingSpec{
					Name:             "my-offering",
					Description:      "my offering description",
					Tags:             []string{"t1"},
					Requires:         []string{"r1"},
					DocumentationURL: tools.PtrTo("https://my.offering.com"),
					BrokerCatalog: korifiv1alpha1.ServiceBrokerCatalog{
						ID:       "offering-catalog-guid",
						Metadata: metadata,
						Features: korifiv1alpha1.BrokerCatalogFeatures{
							PlanUpdateable:       true,
							Bindable:             true,
							InstancesRetrievable: true,
							BindingsRetrievable:  true,
							AllowContextUpdates:  true,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, serviceOffering)).To(Succeed())

			updateMessage = repositories.UpdateServiceOfferingMessage{
				GUID: offeringGUID,
				MetadataPatch: repositories.MetadataPatch{
					Labels: map[string]*string{
						"new-offering-label": tools.PtrTo("new-offering-label-value"),
					},
					Annotations: map[string]*string{
						"new-offering-annotation": tools.PtrTo("new-offering-annotation-value"),
					},
				},
			}
		})

		JustBeforeEach(func() {
			updatedServiceOffering, updateErr = repo.UpdateServiceOffering(ctx, authInfo, updateMessage)
		})

		It("fails because the user has no offerings", func() {
			Expect(updateErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a CFAdmin", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			It("updates the service offering metadata", func() {
				Expect(updateErr).NotTo(HaveOccurred())
				Expect(updatedServiceOffering.Metadata.Labels).To(SatisfyAll(
					HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerGUIDLabel, brokerGUID),
					HaveKeyWithValue("new-offering-label", "new-offering-label-value"),
				))
				Expect(updatedServiceOffering.Metadata.Annotations).To(HaveKeyWithValue("new-offering-annotation", "new-offering-annotation-value"))
			})

			It("updates the service offering metadata in kubernetes", func() {
				updatedServiceOffering := new(korifiv1alpha1.CFServiceOffering)
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceOffering), updatedServiceOffering)).To(Succeed())
				Expect(updatedServiceOffering.Labels).To(SatisfyAll(
					HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerGUIDLabel, brokerGUID),
					HaveKeyWithValue("new-offering-label", "new-offering-label-value"),
				))
				Expect(updatedServiceOffering.Annotations).To(HaveKeyWithValue("new-offering-annotation", "new-offering-annotation-value"))
			})
		})
	})
})
