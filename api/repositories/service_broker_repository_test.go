package repositories_test

import (
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("ServiceBrokerRepo", func() {
	var repo *repositories.ServiceBrokerRepo

	BeforeEach(func() {
		repo = repositories.NewServiceBrokerRepo(userClientFactory, rootNamespace)
	})

	Describe("Create", func() {
		var (
			createMsg repositories.CreateServiceBrokerMessage
			broker    repositories.ServiceBrokerResource
			createErr error
		)

		BeforeEach(func() {
			createMsg = repositories.CreateServiceBrokerMessage{
				Metadata: model.Metadata{
					Labels: map[string]string{
						"label": "label-value",
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				},
				Broker: services.ServiceBroker{
					Name: "my-broker",
					URL:  "https://my.broker.com",
				},
				Credentials: services.BrokerCredentials{
					Username: "broker-user",
					Password: "broker-password",
				},
			}
		})

		JustBeforeEach(func() {
			broker, createErr = repo.CreateServiceBroker(ctx, authInfo, createMsg)
		})

		It("returns a forbidden error", func() {
			Expect(createErr).To(SatisfyAll(
				BeAssignableToTypeOf(apierrors.ForbiddenError{}),
				MatchError(ContainSubstring("cfservicebrokers.korifi.cloudfoundry.org is forbidden")),
			))
		})

		It("does not create a credentials secret", func() {
			secrets := &corev1.SecretList{}
			Expect(k8sClient.List(ctx, secrets, client.InNamespace(rootNamespace))).To(Succeed())
			Expect(secrets.Items).To(BeEmpty())
		})

		When("the user can create CFServiceBrokers", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			JustBeforeEach(func() {
				Expect(createErr).NotTo(HaveOccurred())
			})

			It("returns a ServiceBrokerResource", func() {
				Expect(broker.ServiceBroker).To(Equal(services.ServiceBroker{
					Name: "my-broker",
					URL:  "https://my.broker.com",
				}))
				Expect(broker.Metadata).To(Equal(model.Metadata{
					Labels: map[string]string{
						"label": "label-value",
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				}))
				Expect(broker.CFResource.GUID).NotTo(BeEmpty())
				Expect(broker.CFResource.CreatedAt).NotTo(BeZero())
			})

			It("creates a CFServiceBroker resource in Kubernetes", func() {
				cfServiceBroker := &korifiv1alpha1.CFServiceBroker{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      broker.GUID,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBroker), cfServiceBroker)).To(Succeed())
				Expect(cfServiceBroker.Labels).To(HaveKeyWithValue("label", "label-value"))
				Expect(cfServiceBroker.Annotations).To(HaveKeyWithValue("annotation", "annotation-value"))
				Expect(cfServiceBroker.Spec.Name).To(Equal("my-broker"))
				Expect(cfServiceBroker.Spec.URL).To(Equal("https://my.broker.com"))
				Expect(cfServiceBroker.Spec.Credentials.Name).NotTo(BeEmpty())
			})

			It("stores broker credentials in a k8s secret", func() {
				cfServiceBroker := &korifiv1alpha1.CFServiceBroker{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      broker.GUID,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBroker), cfServiceBroker)).To(Succeed())

				Expect(cfServiceBroker.Spec.Credentials.Name).NotTo(BeEmpty())
				credentialsSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      cfServiceBroker.Spec.Credentials.Name,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)).To(Succeed())
				Expect(credentialsSecret.Data).To(SatisfyAll(
					HaveLen(1),
					HaveKeyWithValue(tools.CredentialsSecretKey,
						MatchJSON(`{"username" : "broker-user", "password": "broker-password"}`),
					),
				))
				Expect(credentialsSecret.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Kind": Equal("CFServiceBroker"),
					"Name": Equal(cfServiceBroker.Name),
				})))
			})
		})
	})

	Describe("GetState", func() {
		var (
			brokerGUID      string
			cfServiceBroker *korifiv1alpha1.CFServiceBroker
			state           model.CFResourceState
			getStateErr     error
		)

		BeforeEach(func() {
			brokerGUID = uuid.NewString()
			cfServiceBroker = &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      brokerGUID,
				},
			}
			Expect(k8sClient.Create(ctx, cfServiceBroker)).To(Succeed())
		})

		JustBeforeEach(func() {
			state, getStateErr = repo.GetState(ctx, authInfo, brokerGUID)
		})

		It("returns a forbidden error", func() {
			Expect(getStateErr).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user can get CFServiceBrokers", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			JustBeforeEach(func() {
				Expect(getStateErr).NotTo(HaveOccurred())
			})

			It("returns unknown state", func() {
				Expect(getStateErr).NotTo(HaveOccurred())
				Expect(state).To(Equal(model.CFResourceState{
					Status: model.CFResourceStatusUnknown,
				}))
			})

			When("the broker is ready", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, cfServiceBroker, func() {
						meta.SetStatusCondition(&cfServiceBroker.Status.Conditions, metav1.Condition{
							Type:    korifiv1alpha1.StatusConditionReady,
							Status:  metav1.ConditionTrue,
							Message: "Ready",
							Reason:  "Ready",
						})
					})).To(Succeed())
				})

				It("returns ready state", func() {
					Expect(getStateErr).NotTo(HaveOccurred())
					Expect(state).To(Equal(model.CFResourceState{
						Status: model.CFResourceStatusReady,
					}))
				})
			})

			When("the broker is not ready", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, cfServiceBroker, func() {
						meta.SetStatusCondition(&cfServiceBroker.Status.Conditions, metav1.Condition{
							Type:    korifiv1alpha1.StatusConditionReady,
							Status:  metav1.ConditionFalse,
							Message: "NotReady",
							Reason:  "NotReady",
						})
					})).To(Succeed())
				})

				It("returns unknown state", func() {
					Expect(getStateErr).NotTo(HaveOccurred())
					Expect(state).To(Equal(model.CFResourceState{
						Status: model.CFResourceStatusUnknown,
					}))
				})
			})
		})
	})

	Describe("ListServiceBrokers", func() {
		var (
			brokers []repositories.ServiceBrokerResource
			message repositories.ListServiceBrokerMessage
		)

		BeforeEach(func() {
			message = repositories.ListServiceBrokerMessage{}

			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      "broker-1",
					Labels: map[string]string{
						"broker-label": "broker-label-value",
					},
					Annotations: map[string]string{
						"broker-annotation": "broker-annotation-value",
					},
				},
				Spec: korifiv1alpha1.CFServiceBrokerSpec{
					ServiceBroker: services.ServiceBroker{
						Name: "first-broker",
						URL:  "https://first.broker",
					},
				},
			})).To(Succeed())

			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      "broker-2",
				},
				Spec: korifiv1alpha1.CFServiceBrokerSpec{
					ServiceBroker: services.ServiceBroker{
						Name: "second-broker",
						URL:  "https://second.broker",
					},
				},
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			var err error
			brokers, err = repo.ListServiceBrokers(ctx, authInfo, message)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns a list of brokers", func() {
			Expect(brokers).To(ConsistOf(
				MatchAllFields(Fields{
					"ServiceBroker": MatchAllFields(Fields{
						"Name": Equal("first-broker"),
						"URL":  Equal("https://first.broker"),
					}),
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID":      Equal("broker-1"),
						"CreatedAt": Not(BeZero()),
						"Metadata": MatchAllFields(Fields{
							"Labels":      HaveKeyWithValue("broker-label", "broker-label-value"),
							"Annotations": HaveKeyWithValue("broker-annotation", "broker-annotation-value"),
						}),
					}),
				}),
				MatchAllFields(Fields{
					"ServiceBroker": MatchAllFields(Fields{
						"Name": Equal("second-broker"),
						"URL":  Equal("https://second.broker"),
					}),
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID":      Equal("broker-2"),
						"CreatedAt": Not(BeZero()),
						"Metadata": MatchAllFields(Fields{
							"Labels":      BeEmpty(),
							"Annotations": BeEmpty(),
						}),
					}),
				}),
			))
		})

		When("a name filter is applied", func() {
			BeforeEach(func() {
				message.Names = []string{"second-broker"}
			})

			It("only returns the matching brokers", func() {
				Expect(brokers).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"ServiceBroker": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("second-broker"),
						}),
					}),
				))
			})
		})

		When("a nonexistent name filter is applied", func() {
			BeforeEach(func() {
				message.Names = []string{"no-such-broker"}
			})

			It("returns an empty list", func() {
				Expect(brokers).To(BeEmpty())
			})
		})
	})

	Describe("GetServiceBroker", func() {
		var (
			serviceBroker repositories.ServiceBrokerResource
			getErr        error
		)

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      "broker-1",
					Labels: map[string]string{
						"broker-label": "broker-label-value",
					},
					Annotations: map[string]string{
						"broker-annotation": "broker-annotation-value",
					},
				},
				Spec: korifiv1alpha1.CFServiceBrokerSpec{
					ServiceBroker: services.ServiceBroker{
						Name: "first-broker",
						URL:  "https://first.broker",
					},
				},
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			serviceBroker, getErr = repo.GetServiceBroker(ctx, authInfo, "broker-1")
		})

		It("returns a forbidden error", func() {
			Expect(getErr).To(SatisfyAll(
				BeAssignableToTypeOf(apierrors.ForbiddenError{}),
				MatchError(ContainSubstring(`cfservicebrokers.korifi.cloudfoundry.org "broker-1" is forbidden`)),
			))
		})

		When("the user can get CFServiceBrokers", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			It("returns the broker", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(serviceBroker).To(MatchAllFields(Fields{
					"ServiceBroker": MatchAllFields(Fields{
						"Name": Equal("first-broker"),
						"URL":  Equal("https://first.broker"),
					}),
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID":      Equal("broker-1"),
						"CreatedAt": Not(BeZero()),
						"Metadata": MatchAllFields(Fields{
							"Labels":      HaveKeyWithValue("broker-label", "broker-label-value"),
							"Annotations": HaveKeyWithValue("broker-annotation", "broker-annotation-value"),
						}),
					}),
				}))
			})
		})
	})

	Describe("DeleteServiceBroker", func() {
		var (
			deleteErr  error
			brokerGUID string
		)

		BeforeEach(func() {
			createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)

			brokerGUID = uuid.NewString()
			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      brokerGUID,
				},
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			deleteErr = repo.DeleteServiceBroker(ctx, authInfo, brokerGUID)
		})

		It("deletes the CFServiceBroker resource", func() {
			Expect(deleteErr).NotTo(HaveOccurred())

			brokers := &korifiv1alpha1.CFServiceBrokerList{}
			Expect(k8sClient.List(ctx, brokers, client.InNamespace(rootNamespace))).To(Succeed())
			Expect(brokers.Items).To(BeEmpty())
		})

		When("the broker doesn't exist", func() {
			BeforeEach(func() {
				brokerGUID = "no-such-broker"
			})

			It("errors", func() {
				Expect(deleteErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("GetDeletedAt", func() {
		var (
			broker    *korifiv1alpha1.CFServiceBroker
			deletedAt *time.Time
			getErr    error
		)

		BeforeEach(func() {
			createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)

			broker = &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
			}

			Expect(k8sClient.Create(ctx, broker)).To(Succeed())
		})

		JustBeforeEach(func() {
			deletedAt, getErr = repo.GetDeletedAt(ctx, authInfo, broker.Name)
		})

		It("returns nil", func() {
			Expect(getErr).NotTo(HaveOccurred())
			Expect(deletedAt).To(BeNil())
		})

		When("the broker is being deleted", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, k8sClient, broker, func() {
					broker.Finalizers = append(broker.Finalizers, "kubernetes")
				})).To(Succeed())

				Expect(k8sClient.Delete(ctx, broker)).To(Succeed())
			})

			It("returns the deletion time", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(deletedAt).To(PointTo(BeTemporally("~", time.Now(), time.Minute)))
			})
		})

		When("the broker isn't found", func() {
			BeforeEach(func() {
				Expect(k8sClient.Delete(ctx, broker)).To(Succeed())
			})

			It("errors", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})
})
