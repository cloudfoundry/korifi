package repositories_test

import (
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "code.cloudfoundry.org/korifi/tests/matchers"
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
			createMsg    repositories.CreateServiceBrokerMessage
			brokerRecord repositories.ServiceBrokerRecord
			createErr    error
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
			brokerRecord, createErr = repo.CreateServiceBroker(ctx, authInfo, createMsg)
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

			It("returns a ServiceBrokerRecord", func() {
				Expect(createErr).NotTo(HaveOccurred())
				Expect(brokerRecord.ServiceBroker).To(Equal(services.ServiceBroker{
					Name: "my-broker",
					URL:  "https://my.broker.com",
				}))
				Expect(brokerRecord.Metadata).To(Equal(model.Metadata{
					Labels: map[string]string{
						"label": "label-value",
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				}))
				Expect(brokerRecord.CFResource.GUID).NotTo(BeEmpty())
				Expect(brokerRecord.CFResource.CreatedAt).NotTo(BeZero())
			})

			It("creates a CFServiceBroker resource in Kubernetes", func() {
				Expect(createErr).NotTo(HaveOccurred())
				cfServiceBroker := &korifiv1alpha1.CFServiceBroker{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      brokerRecord.GUID,
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
				Expect(createErr).NotTo(HaveOccurred())
				cfServiceBroker := &korifiv1alpha1.CFServiceBroker{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      brokerRecord.GUID,
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
			cfServiceBroker *korifiv1alpha1.CFServiceBroker
			state           model.CFResourceState
			getStateErr     error
		)

		BeforeEach(func() {
			cfServiceBroker = &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
			}
			Expect(k8sClient.Create(ctx, cfServiceBroker)).To(Succeed())
		})

		JustBeforeEach(func() {
			state, getStateErr = repo.GetState(ctx, authInfo, cfServiceBroker.Name)
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
				Expect(state).To(Equal(model.CFResourceStateUnknown))
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
						cfServiceBroker.Status.ObservedGeneration = cfServiceBroker.Generation
					})).To(Succeed())
				})

				It("returns ready state", func() {
					Expect(getStateErr).NotTo(HaveOccurred())
					Expect(state).To(Equal(model.CFResourceStateReady))
				})

				When("the ready status is stale ", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, cfServiceBroker, func() {
							cfServiceBroker.Status.ObservedGeneration = -1
						})).To(Succeed())
					})

					It("returns unknown state", func() {
						Expect(getStateErr).NotTo(HaveOccurred())
						Expect(state).To(Equal(model.CFResourceStateUnknown))
					})
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
					Expect(state).To(Equal(model.CFResourceStateUnknown))
				})
			})
		})
	})

	Describe("ListServiceBrokers", func() {
		var (
			brokers []repositories.ServiceBrokerRecord
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

		When("a guid filter is applied", func() {
			BeforeEach(func() {
				message.GUIDs = []string{"broker-2"}
			})

			It("only returns the matching brokers", func() {
				Expect(brokers).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"CFResource": MatchFields(IgnoreExtras, Fields{
							"GUID": Equal("broker-2"),
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
			serviceBroker repositories.ServiceBrokerRecord
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

	Describe("UpdateServiceBroker", func() {
		var (
			cfServiceBroker *korifiv1alpha1.CFServiceBroker
			brokerRecord    repositories.ServiceBrokerRecord
			updateMessage   repositories.UpdateServiceBrokerMessage
			updateErr       error
		)

		BeforeEach(func() {
			credentialsData, err := tools.ToCredentialsSecretData(map[string]string{
				"username": "user",
				"password": "pass",
			})
			Expect(err).NotTo(HaveOccurred())

			credentialsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: rootNamespace,
				},
				Data: credentialsData,
			}
			Expect(k8sClient.Create(ctx, credentialsSecret)).To(Succeed())

			cfServiceBroker = &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceBrokerSpec{
					ServiceBroker: services.ServiceBroker{
						Name: "my-broker",
						URL:  "https://my.broker",
					},
					Credentials: corev1.LocalObjectReference{
						Name: credentialsSecret.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfServiceBroker)).To(Succeed())

			updateMessage = repositories.UpdateServiceBrokerMessage{
				GUID: cfServiceBroker.Name,
				Name: tools.PtrTo("your-broker"),
				URL:  tools.PtrTo("https://your.broker"),
				Credentials: &services.BrokerCredentials{
					Username: "another-user",
					Password: "another-pass",
				},
				MetadataPatch: repositories.MetadataPatch{
					Labels: map[string]*string{
						"foo": tools.PtrTo("bar"),
					},
					Annotations: map[string]*string{
						"baz": tools.PtrTo("qux"),
					},
				},
			}
		})

		JustBeforeEach(func() {
			brokerRecord, updateErr = repo.UpdateServiceBroker(ctx, authInfo, updateMessage)
		})

		It("returns a forbidden error", func() {
			Expect(updateErr).To(WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user has permissions to update brokers", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			It("returns an updated ServiceBrokerRecord", func() {
				Expect(updateErr).NotTo(HaveOccurred())
				Expect(brokerRecord.ServiceBroker).To(Equal(services.ServiceBroker{
					Name: "your-broker",
					URL:  "https://your.broker",
				}))

				Expect(brokerRecord.Metadata.Labels).To(HaveKeyWithValue("foo", "bar"))
				Expect(brokerRecord.Metadata.Annotations).To(HaveKeyWithValue("baz", "qux"))
			})

			It("updates the CFServiceBroker resource in Kubernetes", func() {
				Expect(updateErr).NotTo(HaveOccurred())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBroker), cfServiceBroker)).To(Succeed())

				Expect(cfServiceBroker.Spec.ServiceBroker).To(Equal(services.ServiceBroker{
					Name: "your-broker",
					URL:  "https://your.broker",
				}))

				Expect(cfServiceBroker.Labels).To(HaveKeyWithValue("foo", "bar"))
				Expect(cfServiceBroker.Annotations).To(HaveKeyWithValue("baz", "qux"))
			})

			It("updates the service borker credentials secret", func() {
				Expect(updateErr).NotTo(HaveOccurred())
				credentialsSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfServiceBroker.Spec.Credentials.Name,
						Namespace: rootNamespace,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)).To(Succeed())
				Expect(credentialsSecret.Data).To(SatisfyAll(
					HaveLen(1),
					HaveKeyWithValue(tools.CredentialsSecretKey,
						MatchJSON(`{"username" : "another-user", "password": "another-pass"}`),
					),
				))
			})

			When("credentials are not updated", func() {
				BeforeEach(func() {
					updateMessage.Credentials = nil
				})

				It("does not update the credentials secret", func() {
					Expect(updateErr).NotTo(HaveOccurred())
					credentialsSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      cfServiceBroker.Spec.Credentials.Name,
							Namespace: rootNamespace,
						},
					}
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)).To(Succeed())
					Expect(credentialsSecret.Data).To(SatisfyAll(
						HaveLen(1),
						HaveKeyWithValue(tools.CredentialsSecretKey,
							MatchJSON(`{"username" : "user", "password": "pass"}`),
						),
					))
				})
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
				Expect(deleteErr).To(WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
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
				Expect(getErr).To(WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})
})
