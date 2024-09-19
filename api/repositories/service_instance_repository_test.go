package repositories_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fakeawaiter"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ServiceInstanceRepository", func() {
	var (
		serviceInstanceRepo *repositories.ServiceInstanceRepo
		conditionAwaiter    *fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFServiceInstance,
			korifiv1alpha1.CFServiceInstance,
			korifiv1alpha1.CFServiceInstanceList,
			*korifiv1alpha1.CFServiceInstanceList,
		]

		org                 *korifiv1alpha1.CFOrg
		space               *korifiv1alpha1.CFSpace
		serviceInstanceName string
	)

	BeforeEach(func() {
		conditionAwaiter = &fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFServiceInstance,
			korifiv1alpha1.CFServiceInstance,
			korifiv1alpha1.CFServiceInstanceList,
			*korifiv1alpha1.CFServiceInstanceList,
		]{}
		serviceInstanceRepo = repositories.NewServiceInstanceRepo(namespaceRetriever, userClientFactory, nsPerms, conditionAwaiter)

		org = createOrgWithCleanup(ctx, uuid.NewString())
		space = createSpaceWithCleanup(ctx, org.Name, uuid.NewString())
		serviceInstanceName = uuid.NewString()
	})

	Describe("CreateUserProvidedServiceInstance", func() {
		var (
			serviceInstanceCreateMessage repositories.CreateUPSIMessage
			record                       repositories.ServiceInstanceRecord
			createErr                    error
		)

		BeforeEach(func() {
			serviceInstanceCreateMessage = repositories.CreateUPSIMessage{
				Name:      serviceInstanceName,
				SpaceGUID: space.Name,
				Credentials: map[string]any{
					"object": map[string]any{"a": "b"},
				},
				Tags: []string{"foo", "bar"},
			}
		})

		JustBeforeEach(func() {
			record, createErr = serviceInstanceRepo.CreateUserProvidedServiceInstance(ctx, authInfo, serviceInstanceCreateMessage)
		})

		It("returns a Forbidden error", func() {
			Expect(createErr).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("user has permissions to create ServiceInstances", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns a service instance record", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(record.GUID).NotTo(BeEmpty())
				Expect(record.SpaceGUID).To(Equal(space.Name))
				Expect(record.Name).To(Equal(serviceInstanceName))
				Expect(record.Type).To(Equal("user-provided"))
				Expect(record.Tags).To(ConsistOf([]string{"foo", "bar"}))
				Expect(record.SecretName).NotTo(BeEmpty())
				Expect(record.Relationships()).To(Equal(map[string]string{
					"space": space.Name,
				}))

				Expect(record.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(record.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
			})

			It("creates a CFServiceInstance resource", func() {
				Expect(createErr).NotTo(HaveOccurred())

				cfServiceInstance := &korifiv1alpha1.CFServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: record.SpaceGUID,
						Name:      record.GUID,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())

				Expect(cfServiceInstance.Spec.DisplayName).To(Equal(serviceInstanceName))
				Expect(cfServiceInstance.Spec.SecretName).NotTo(BeEmpty())
				Expect(cfServiceInstance.Spec.Type).To(BeEquivalentTo(korifiv1alpha1.UserProvidedType))
				Expect(cfServiceInstance.Spec.Tags).To(ConsistOf("foo", "bar"))
			})

			It("creates the credentials secret", func() {
				Expect(createErr).NotTo(HaveOccurred())

				credentialsSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: record.SpaceGUID,
						Name:      record.SecretName,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)).To(Succeed())

				Expect(credentialsSecret.Type).To(Equal(corev1.SecretTypeOpaque))
				Expect(credentialsSecret.Data).To(MatchAllKeys(Keys{tools.CredentialsSecretKey: Not(BeEmpty())}))
				credentials := map[string]any{}
				Expect(json.Unmarshal(credentialsSecret.Data[tools.CredentialsSecretKey], &credentials)).To(Succeed())
				Expect(credentials).To(Equal(map[string]any{
					"object": map[string]any{"a": "b"},
				}))
			})
		})
	})

	Describe("CreateManagedServiceInstance", func() {
		var (
			serviceInstanceCreateMessage repositories.CreateManagedSIMessage
			record                       repositories.ServiceInstanceRecord
			createErr                    error
		)

		BeforeEach(func() {
			serviceInstanceCreateMessage = repositories.CreateManagedSIMessage{
				Name:      serviceInstanceName,
				SpaceGUID: space.Name,
				PlanGUID:  "plan-guid",
				Tags:      []string{"foo", "bar"},
				Parameters: map[string]any{
					"p1": map[string]any{
						"p11": "v11",
					},
				},
			}
		})

		JustBeforeEach(func() {
			record, createErr = serviceInstanceRepo.CreateManagedServiceInstance(ctx, authInfo, serviceInstanceCreateMessage)
		})

		It("returns a Forbidden error", func() {
			Expect(createErr).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("user has permissions to create ServiceInstances", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns a service instance record", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(record.GUID).NotTo(BeEmpty())
				Expect(record.SpaceGUID).To(Equal(space.Name))
				Expect(record.Name).To(Equal(serviceInstanceName))
				Expect(record.Type).To(Equal("managed"))
				Expect(record.Tags).To(ConsistOf([]string{"foo", "bar"}))
				Expect(record.SecretName).To(BeEmpty())
				Expect(record.Relationships()).To(Equal(map[string]string{
					"service_plan": "plan-guid",
					"space":        space.Name,
				}))
				Expect(record.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(record.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
			})

			It("creates a CFServiceInstance resource", func() {
				Expect(createErr).NotTo(HaveOccurred())

				cfServiceInstance := &korifiv1alpha1.CFServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: record.SpaceGUID,
						Name:      record.GUID,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())

				Expect(cfServiceInstance.Spec.DisplayName).To(Equal(serviceInstanceName))
				Expect(cfServiceInstance.Spec.SecretName).To(BeEmpty())
				Expect(cfServiceInstance.Spec.Type).To(BeEquivalentTo(korifiv1alpha1.ManagedType))
				Expect(cfServiceInstance.Spec.Tags).To(ConsistOf("foo", "bar"))
				Expect(cfServiceInstance.Spec.PlanGUID).To(Equal("plan-guid"))
				Expect(cfServiceInstance.Spec.Parameters).NotTo(BeNil())

				actualParams := map[string]any{}
				Expect(json.Unmarshal(cfServiceInstance.Spec.Parameters.Raw, &actualParams)).To(Succeed())
				Expect(actualParams).To(Equal(map[string]any{
					"p1": map[string]any{
						"p11": "v11",
					},
				}))
			})
		})
	})

	Describe("GetDeletedAt", func() {
		var (
			cfServiceInstance *korifiv1alpha1.CFServiceInstance
			deletionTime      *time.Time
			getErr            error
		)

		BeforeEach(func() {
			cfServiceInstance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					Type: "managed",
				},
			}

			Expect(k8sClient.Create(ctx, cfServiceInstance)).To(Succeed())
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
		})

		JustBeforeEach(func() {
			deletionTime, getErr = serviceInstanceRepo.GetDeletedAt(ctx, authInfo, cfServiceInstance.Name)
		})

		It("returns nil", func() {
			Expect(getErr).NotTo(HaveOccurred())
			Expect(deletionTime).To(BeNil())
		})

		When("the instance is being deleted", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, k8sClient, cfServiceInstance, func() {
					cfServiceInstance.Finalizers = append(cfServiceInstance.Finalizers, "foo")
				})).To(Succeed())

				Expect(k8sClient.Delete(ctx, cfServiceInstance)).To(Succeed())
			})

			It("returns the deletion time", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(deletionTime).To(PointTo(BeTemporally("~", time.Now(), time.Minute)))
			})
		})

		When("the instance isn't found", func() {
			BeforeEach(func() {
				Expect(k8sClient.Delete(ctx, cfServiceInstance)).To(Succeed())
			})

			It("errors", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("GetState", func() {
		var (
			cfServiceInstance *korifiv1alpha1.CFServiceInstance
			state             model.CFResourceState
			stateErr          error
		)
		BeforeEach(func() {
			cfServiceInstance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					Type: "managed",
				},
			}

			Expect(k8sClient.Create(ctx, cfServiceInstance)).To(Succeed())
		})

		JustBeforeEach(func() {
			state, stateErr = serviceInstanceRepo.GetState(ctx, authInfo, cfServiceInstance.Name)
		})

		It("returns a forbidden error", func() {
			Expect(stateErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user can get CFServiceInstance", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, cfServiceInstance.Namespace)
			})

			It("returns unknown state", func() {
				Expect(stateErr).NotTo(HaveOccurred())
				Expect(state).To(Equal(model.CFResourceStateUnknown))
			})

			When("the service instance is ready", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, cfServiceInstance, func() {
						meta.SetStatusCondition(&cfServiceInstance.Status.Conditions, metav1.Condition{
							Type:    korifiv1alpha1.StatusConditionReady,
							Status:  metav1.ConditionTrue,
							Message: "Ready",
							Reason:  "Ready",
						})
						cfServiceInstance.Status.ObservedGeneration = cfServiceInstance.Generation
					})).To(Succeed())
				})

				It("returns ready state", func() {
					Expect(stateErr).NotTo(HaveOccurred())
					Expect(state).To(Equal(model.CFResourceStateReady))
				})

				When("the ready status is stale ", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, cfServiceInstance, func() {
							cfServiceInstance.Status.ObservedGeneration = -1
						})).To(Succeed())
					})

					It("returns unknown state", func() {
						Expect(stateErr).NotTo(HaveOccurred())
						Expect(state).To(Equal(model.CFResourceStateUnknown))
					})
				})
			})
		})
	})

	Describe("PatchServiceInstance", func() {
		var (
			cfServiceInstance     *korifiv1alpha1.CFServiceInstance
			secret                *corev1.Secret
			serviceInstanceRecord repositories.ServiceInstanceRecord
			patchMessage          repositories.PatchServiceInstanceMessage
			err                   error
		)

		BeforeEach(func() {
			serviceInstanceGUID := uuid.NewString()
			secretName := uuid.NewString()
			cfServiceInstance = createServiceInstanceCR(ctx, k8sClient, serviceInstanceGUID, space.Name, serviceInstanceName, secretName)
			conditionAwaiter.AwaitConditionReturns(cfServiceInstance, nil)
			Expect(k8s.Patch(ctx, k8sClient, cfServiceInstance, func() {
				cfServiceInstance.Status.Credentials.Name = secretName
			})).To(Succeed())

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: space.Name,
				},
				StringData: map[string]string{
					tools.CredentialsSecretKey: `{"a": "b"}`,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			patchMessage = repositories.PatchServiceInstanceMessage{
				GUID:        cfServiceInstance.Name,
				SpaceGUID:   space.Name,
				Name:        tools.PtrTo("new-name"),
				Credentials: nil,
				Tags:        &[]string{"new"},
				MetadataPatch: repositories.MetadataPatch{
					Labels:      map[string]*string{"new-label": tools.PtrTo("new-label-value")},
					Annotations: map[string]*string{"new-annotation": tools.PtrTo("new-annotation-value")},
				},
			}
		})

		JustBeforeEach(func() {
			serviceInstanceRecord, err = serviceInstanceRepo.PatchServiceInstance(ctx, authInfo, patchMessage)
		})

		When("authorized in the space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, orgUserRole.Name, org.Name)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns the updated record", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceInstanceRecord.Name).To(Equal("new-name"))
				Expect(serviceInstanceRecord.Tags).To(ConsistOf("new"))
				Expect(serviceInstanceRecord.Labels).To(HaveLen(2))
				Expect(serviceInstanceRecord.Labels).To(HaveKeyWithValue("a-label", "a-label-value"))
				Expect(serviceInstanceRecord.Labels).To(HaveKeyWithValue("new-label", "new-label-value"))
				Expect(serviceInstanceRecord.Annotations).To(HaveLen(2))
				Expect(serviceInstanceRecord.Annotations).To(HaveKeyWithValue("an-annotation", "an-annotation-value"))
				Expect(serviceInstanceRecord.Annotations).To(HaveKeyWithValue("new-annotation", "new-annotation-value"))
			})

			It("updates the service instance", func() {
				Expect(err).NotTo(HaveOccurred())
				serviceInstance := new(korifiv1alpha1.CFServiceInstance)

				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), serviceInstance)).To(Succeed())
					g.Expect(serviceInstance.Spec.DisplayName).To(Equal("new-name"))
					g.Expect(serviceInstance.Spec.Tags).To(ConsistOf("new"))
					g.Expect(serviceInstance.Labels).To(HaveLen(2))
					g.Expect(serviceInstance.Labels).To(HaveKeyWithValue("a-label", "a-label-value"))
					g.Expect(serviceInstance.Labels).To(HaveKeyWithValue("new-label", "new-label-value"))
					g.Expect(serviceInstance.Annotations).To(HaveLen(2))
					g.Expect(serviceInstance.Annotations).To(HaveKeyWithValue("an-annotation", "an-annotation-value"))
					g.Expect(serviceInstance.Annotations).To(HaveKeyWithValue("new-annotation", "new-annotation-value"))
				}).Should(Succeed())
			})

			When("tags is an empty list", func() {
				BeforeEach(func() {
					patchMessage.Tags = &[]string{}
				})

				It("clears the tags", func() {
					Expect(err).NotTo(HaveOccurred())
					serviceInstance := new(korifiv1alpha1.CFServiceInstance)

					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), serviceInstance)).To(Succeed())
						g.Expect(serviceInstance.Spec.Tags).To(BeEmpty())
					}).Should(Succeed())
				})
			})

			When("tags is nil", func() {
				BeforeEach(func() {
					patchMessage.Tags = nil
				})

				It("preserves the tags", func() {
					Expect(err).NotTo(HaveOccurred())
					serviceInstance := new(korifiv1alpha1.CFServiceInstance)

					Consistently(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), serviceInstance)).To(Succeed())
						g.Expect(serviceInstance.Spec.Tags).To(ConsistOf("database", "mysql"))
					}).Should(Succeed())
				})
			})

			It("does not change the credential secret", func() {
				Consistently(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)).To(Succeed())
					g.Expect(secret.Data).To(MatchAllKeys(Keys{tools.CredentialsSecretKey: Not(BeEmpty())}))
					credentials := map[string]any{}
					g.Expect(json.Unmarshal(secret.Data[tools.CredentialsSecretKey], &credentials)).To(Succeed())
					g.Expect(credentials).To(MatchAllKeys(Keys{
						"a": Equal("b"),
					}))
				}).Should(Succeed())
			})

			When("ServiceInstance credentials are provided", func() {
				BeforeEach(func() {
					patchMessage.Credentials = &map[string]any{
						"object": map[string]any{"c": "d"},
					}
				})

				It("awaits credentials secret available condition", func() {
					Expect(conditionAwaiter.AwaitConditionCallCount()).To(Equal(1))
					obj, conditionType := conditionAwaiter.AwaitConditionArgsForCall(0)
					Expect(obj.GetName()).To(Equal(cfServiceInstance.Name))
					Expect(obj.GetNamespace()).To(Equal(cfServiceInstance.Namespace))
					Expect(conditionType).To(Equal(korifiv1alpha1.StatusConditionReady))
				})

				It("updates the creds", func() {
					Expect(err).NotTo(HaveOccurred())
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)).To(Succeed())
						g.Expect(secret.Data).To(MatchAllKeys(Keys{tools.CredentialsSecretKey: Not(BeEmpty())}))
						credentials := map[string]any{}
						Expect(json.Unmarshal(secret.Data[tools.CredentialsSecretKey], &credentials)).To(Succeed())
						Expect(credentials).To(MatchAllKeys(Keys{
							"object": MatchAllKeys(Keys{"c": Equal("d")}),
						}))
					}).Should(Succeed())
				})

				When("the credentials secret available condition is not met", func() {
					BeforeEach(func() {
						conditionAwaiter.AwaitConditionReturns(&korifiv1alpha1.CFServiceInstance{}, errors.New("timed-out"))
					})

					It("returns an error", func() {
						Expect(err).To(MatchError(ContainSubstring("timed-out")))
					})
				})

				When("the credentials secret in the spec does not match the credentials secret in the status", func() {
					BeforeEach(func() {
						Expect(k8sClient.Create(ctx, &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: cfServiceInstance.Namespace,
								Name:      "foo",
							},
							Data: map[string][]byte{
								tools.CredentialsSecretKey: []byte(`{"type":"database"}`),
							},
						})).To(Succeed())
						Expect(k8s.Patch(ctx, k8sClient, cfServiceInstance, func() {
							cfServiceInstance.Status.Credentials.Name = "foo"
						})).To(Succeed())
					})

					It("updates the secret in the record", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(serviceInstanceRecord.SecretName).To(Equal("foo"))
					})

					It("updates the secret in the spec", func() {
						Expect(err).NotTo(HaveOccurred())
						Eventually(func(g Gomega) {
							g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())
							g.Expect(cfServiceInstance.Spec.SecretName).To(Equal("foo"))
						}).Should(Succeed())
					})
				})
			})
		})
	})

	Describe("ListServiceInstances", func() {
		var (
			space2                                                     *korifiv1alpha1.CFSpace
			cfServiceInstance1, cfServiceInstance2, cfServiceInstance3 *korifiv1alpha1.CFServiceInstance
			filters                                                    repositories.ListServiceInstanceMessage
			listErr                                                    error

			serviceInstanceList []repositories.ServiceInstanceRecord
		)

		BeforeEach(func() {
			space2 = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space2"))

			cfServiceInstance1 = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space.Name,
					Name:      "service-instance-1" + uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: "service-instance-1",
					Type:        korifiv1alpha1.UserProvidedType,
				},
			}
			Expect(k8sClient.Create(ctx, cfServiceInstance1)).To(Succeed())

			cfServiceInstance2 = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space2.Name,
					Name:      "service-instance-2" + uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: "service-instance-2",
					Type:        korifiv1alpha1.UserProvidedType,
				},
			}
			Expect(k8sClient.Create(ctx, cfServiceInstance2)).To(Succeed())

			cfServiceInstance3 = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space2.Name,
					Name:      "service-instance-3" + uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: "service-instance-3",
					Type:        korifiv1alpha1.UserProvidedType,
				},
			}
			Expect(k8sClient.Create(ctx, cfServiceInstance3)).To(Succeed())

			space3 := createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space3"))
			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space3.Name,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: uuid.NewString(),
					Type:        korifiv1alpha1.UserProvidedType,
				},
			})).To(Succeed())

			nonCFNamespace := uuid.NewString()
			Expect(k8sClient.Create(
				ctx,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nonCFNamespace}},
			)).To(Succeed())

			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nonCFNamespace,
					Name:      "service-instance-4" + uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: "service-instance-4",
					Type:        korifiv1alpha1.UserProvidedType,
				},
			})).To(Succeed())

			filters = repositories.ListServiceInstanceMessage{}
		})

		JustBeforeEach(func() {
			serviceInstanceList, listErr = serviceInstanceRepo.ListServiceInstances(ctx, authInfo, filters)
		})

		It("returns an empty list of ServiceInstanceRecord", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(serviceInstanceList).To(BeEmpty())
		})

		When("user is allowed to list service instances ", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space2.Name)
			})

			It("returns the service instances from the allowed namespaces", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(serviceInstanceList).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance1.Name)}),
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance2.Name)}),
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance3.Name)}),
				))
			})

			When("the name filter is set", func() {
				BeforeEach(func() {
					filters = repositories.ListServiceInstanceMessage{
						Names: []string{
							cfServiceInstance1.Spec.DisplayName,
							cfServiceInstance3.Spec.DisplayName,
						},
					}
				})

				It("returns only records for the ServiceInstances with matching spec.name fields", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(serviceInstanceList).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance1.Name)}),
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance3.Name)}),
					))
				})
			})

			When("the spaceGUID filter is set", func() {
				BeforeEach(func() {
					filters = repositories.ListServiceInstanceMessage{
						SpaceGUIDs: []string{
							cfServiceInstance2.Namespace,
							cfServiceInstance3.Namespace,
						},
					}
				})

				It("returns only records for the ServiceInstances within the matching spaces", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(serviceInstanceList).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance2.Name)}),
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance3.Name)}),
					))
				})
			})

			When("the serviceGUID filter is set", func() {
				BeforeEach(func() {
					filters = repositories.ListServiceInstanceMessage{
						GUIDs: []string{cfServiceInstance1.Name, cfServiceInstance3.Name},
					}
				})
				It("returns only records for the ServiceInstances within the matching spaces", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(serviceInstanceList).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance1.Name)}),
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance3.Name)}),
					))
				})
			})

			When("filtered by label selector", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, k8sClient, cfServiceInstance1, func() {
						cfServiceInstance1.Labels = map[string]string{"foo": "FOO1"}
					})).To(Succeed())
					Expect(k8s.PatchResource(ctx, k8sClient, cfServiceInstance2, func() {
						cfServiceInstance2.Labels = map[string]string{"foo": "FOO2"}
					})).To(Succeed())
					Expect(k8s.PatchResource(ctx, k8sClient, cfServiceInstance3, func() {
						cfServiceInstance3.Labels = map[string]string{"not_foo": "NOT_FOO"}
					})).To(Succeed())
				})

				DescribeTable("valid label selectors",
					func(selector string, serviceBindingGUIDPrefixes ...string) {
						serviceInstances, err := serviceInstanceRepo.ListServiceInstances(context.Background(), authInfo, repositories.ListServiceInstanceMessage{
							LabelSelector: selector,
						})
						Expect(err).NotTo(HaveOccurred())

						matchers := []any{}
						for _, prefix := range serviceBindingGUIDPrefixes {
							matchers = append(matchers, MatchFields(IgnoreExtras, Fields{"GUID": HavePrefix(prefix)}))
						}

						Expect(serviceInstances).To(ConsistOf(matchers...))
					},
					Entry("key", "foo", "service-instance-1", "service-instance-2"),
					Entry("!key", "!foo", "service-instance-3"),
					Entry("key=value", "foo=FOO1", "service-instance-1"),
					Entry("key==value", "foo==FOO2", "service-instance-2"),
					Entry("key!=value", "foo!=FOO1", "service-instance-2", "service-instance-3"),
					Entry("key in (value1,value2)", "foo in (FOO1,FOO2)", "service-instance-1", "service-instance-2"),
					Entry("key notin (value1,value2)", "foo notin (FOO2)", "service-instance-1", "service-instance-3"),
				)

				When("the label selector is invalid", func() {
					BeforeEach(func() {
						filters = repositories.ListServiceInstanceMessage{LabelSelector: "~"}
					})

					It("returns an error", func() {
						Expect(listErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
					})
				})
			})
		})
	})

	Describe("GetServiceInstance", func() {
		var (
			space2          *korifiv1alpha1.CFSpace
			serviceInstance *korifiv1alpha1.CFServiceInstance
			record          repositories.ServiceInstanceRecord
			getErr          error
			getGUID         string
		)

		BeforeEach(func() {
			space2 = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space2"))

			serviceInstance = createServiceInstanceCR(ctx, k8sClient, prefixedGUID("service-instance"), space.Name, "the-service-instance", prefixedGUID("secret"))
			createServiceInstanceCR(ctx, k8sClient, prefixedGUID("service-instance"), space2.Name, "some-other-service-instance", prefixedGUID("secret"))
			getGUID = serviceInstance.Name
		})

		JustBeforeEach(func() {
			record, getErr = serviceInstanceRepo.GetServiceInstance(ctx, authInfo, getGUID)
		})

		When("there are no permissions on service instances", func() {
			It("returns a forbidden error", func() {
				Expect(errors.As(getErr, &apierrors.ForbiddenError{})).To(BeTrue())
			})
		})

		When("the user has permissions to get the service instance", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space2.Name)
			})

			It("returns the correct service instance", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(record.Name).To(Equal(serviceInstance.Spec.DisplayName))
				Expect(record.GUID).To(Equal(serviceInstance.Name))
				Expect(record.SpaceGUID).To(Equal(serviceInstance.Namespace))
				Expect(record.SecretName).To(Equal(serviceInstance.Spec.SecretName))
				Expect(record.Tags).To(Equal(serviceInstance.Spec.Tags))
				Expect(record.Type).To(Equal(string(serviceInstance.Spec.Type)))
				Expect(record.Labels).To(Equal(map[string]string{"a-label": "a-label-value"}))
				Expect(record.Annotations).To(Equal(map[string]string{"an-annotation": "an-annotation-value"}))
				Expect(record.Relationships()).To(Equal(map[string]string{
					"space": serviceInstance.Namespace,
				}))
			})

			When("the service is managed", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, serviceInstance, func() {
						serviceInstance.Spec.Type = korifiv1alpha1.ManagedType
						serviceInstance.Spec.PlanGUID = "plan-guid"
					})).To(Succeed())
				})

				It("returns service plan relationships for user provided provided services", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(record.Relationships()).To(Equal(map[string]string{
						"space":        serviceInstance.Namespace,
						"service_plan": "plan-guid",
					}))
				})
			})
		})

		When("the service instance does not exist", func() {
			BeforeEach(func() {
				getGUID = "does-not-exist"
			})

			It("returns a not found error", func() {
				notFoundErr := apierrors.NotFoundError{}
				Expect(errors.As(getErr, &notFoundErr)).To(BeTrue())
			})
		})

		When("more than one service instance with the same guid exists", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space2.Name)
				createServiceInstanceCR(ctx, k8sClient, getGUID, space2.Name, "the-service-instance", prefixedGUID("secret"))
			})

			It("returns a error", func() {
				Expect(getErr).To(MatchError(ContainSubstring("get-service instance duplicate records exist")))
			})
		})

		When("the context has expired", func() {
			BeforeEach(func() {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			})

			It("returns a error", func() {
				Expect(getErr).To(HaveOccurred())
			})
		})
	})

	Describe("DeleteServiceInstance", func() {
		var (
			serviceInstance *korifiv1alpha1.CFServiceInstance
			deleteMessage   repositories.DeleteServiceInstanceMessage
			deleteErr       error
		)

		BeforeEach(func() {
			serviceInstance = createServiceInstanceCR(ctx, k8sClient, prefixedGUID("service-instance"), space.Name, "the-service-instance", prefixedGUID("secret"))

			deleteMessage = repositories.DeleteServiceInstanceMessage{
				GUID:      serviceInstance.Name,
				SpaceGUID: space.Name,
			}
		})

		JustBeforeEach(func() {
			deleteErr = serviceInstanceRepo.DeleteServiceInstance(ctx, authInfo, deleteMessage)
		})

		When("the user has permissions to delete service instances", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("deletes the service instance", func() {
				Expect(deleteErr).NotTo(HaveOccurred())

				namespacedName := types.NamespacedName{
					Name:      serviceInstance.Name,
					Namespace: space.Name,
				}

				err := k8sClient.Get(context.Background(), namespacedName, &korifiv1alpha1.CFServiceInstance{})
				Expect(k8serrors.IsNotFound(err)).To(BeTrue(), fmt.Sprintf("error: %+v", err))
			})

			When("the service instances does not exist", func() {
				BeforeEach(func() {
					deleteMessage.GUID = "does-not-exist"
				})

				It("returns a not found error", func() {
					Expect(errors.As(deleteErr, &apierrors.NotFoundError{})).To(BeTrue())
				})
			})
		})

		When("there are no permissions on service instances", func() {
			It("returns a forbidden error", func() {
				Expect(errors.As(deleteErr, &apierrors.ForbiddenError{})).To(BeTrue())
			})
		})
	})
})
