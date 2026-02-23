package repositories_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	"code.cloudfoundry.org/korifi/api/repositories/fakeawaiter"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	gomega_types "github.com/onsi/gomega/types"
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
			korifiv1alpha1.CFServiceInstanceList,
			*korifiv1alpha1.CFServiceInstanceList,
		]{}

		serviceInstanceRepo = repositories.NewServiceInstanceRepo(
			spaceScopedKlient,
			conditionAwaiter,
			rootNamespace,
		)

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

				Expect(record.GUID).To(matchers.BeValidUUID())
				Expect(record.SpaceGUID).To(Equal(space.Name))
				Expect(record.Name).To(Equal(serviceInstanceName))
				Expect(record.Type).To(Equal("user-provided"))
				Expect(record.Tags).To(ConsistOf([]string{"foo", "bar"}))
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

				cfServiceInstance := &korifiv1alpha1.CFServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: record.SpaceGUID,
						Name:      record.GUID,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())

				credentialsSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: record.SpaceGUID,
						Name:      cfServiceInstance.Spec.SecretName,
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
			servicePlan                  *korifiv1alpha1.CFServicePlan
			serviceInstanceCreateMessage repositories.CreateManagedSIMessage
			record                       repositories.ServiceInstanceRecord
			createErr                    error
		)

		BeforeEach(func() {
			servicePlan = &korifiv1alpha1.CFServicePlan{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServicePlanSpec{
					Visibility: korifiv1alpha1.ServicePlanVisibility{
						Type: korifiv1alpha1.PublicServicePlanVisibilityType,
					},
				},
			}
			Expect(k8sClient.Create(ctx, servicePlan)).To(Succeed())

			serviceInstanceCreateMessage = repositories.CreateManagedSIMessage{
				Name:      serviceInstanceName,
				SpaceGUID: space.Name,
				PlanGUID:  servicePlan.Name,
				Tags:      []string{"foo", "bar"},
				Parameters: map[string]any{
					"p11": "v11",
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
				createRoleBinding(ctx, userName, orgUserRole.Name, org.Name)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns a service instance record", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(record.GUID).To(matchers.BeValidUUID())
				Expect(record.SpaceGUID).To(Equal(space.Name))
				Expect(record.Name).To(Equal(serviceInstanceName))
				Expect(record.Type).To(Equal("managed"))
				Expect(record.Tags).To(ConsistOf([]string{"foo", "bar"}))
				Expect(record.Relationships()).To(Equal(map[string]string{
					"service_plan": servicePlan.Name,
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
				Expect(cfServiceInstance.Spec.PlanGUID).To(Equal(servicePlan.Name))
				Expect(cfServiceInstance.Spec.Parameters.Name).NotTo(BeNil())
			})

			It("creates the parameters secret", func() {
				serviceInstance := new(korifiv1alpha1.CFServiceInstance)
				Expect(
					k8sClient.Get(ctx, types.NamespacedName{Name: record.GUID, Namespace: space.Name}, serviceInstance),
				).To(Succeed())

				paramsSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: serviceInstance.Namespace,
						Name:      serviceInstance.Spec.Parameters.Name,
					},
				}
				Expect(
					k8sClient.Get(ctx, client.ObjectKeyFromObject(paramsSecret), paramsSecret),
				).To(Succeed())

				Expect(paramsSecret.Data).To(Equal(map[string][]byte{
					tools.ParametersSecretKey: []byte(`{"p11":"v11"}`),
				}))

				Expect(paramsSecret.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Kind": Equal("CFServiceInstance"),
					"Name": Equal(serviceInstance.Name),
				})))
			})

			When("the service plan visibility type is admin", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, k8sClient, servicePlan, func() {
						servicePlan.Spec.Visibility.Type = korifiv1alpha1.AdminServicePlanVisibilityType
					})).To(Succeed())
				})

				It("returns unprocessable entity error", func() {
					Expect(createErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
				})
			})

			When("the service plan visibility type is organization", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, k8sClient, servicePlan, func() {
						servicePlan.Spec.Visibility.Type = korifiv1alpha1.OrganizationServicePlanVisibilityType
					})).To(Succeed())
				})

				It("returns unprocessable entity error", func() {
					Expect(createErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
				})

				When("the plan is enabled for the current organization", func() {
					BeforeEach(func() {
						Expect(k8s.PatchResource(ctx, k8sClient, servicePlan, func() {
							servicePlan.Spec.Visibility.Organizations = append(servicePlan.Spec.Visibility.Organizations, org.Name)
						})).To(Succeed())
					})

					It("succeeds", func() {
						Expect(createErr).NotTo(HaveOccurred())
					})

					When("the space does not exist", func() {
						BeforeEach(func() {
							serviceInstanceCreateMessage.SpaceGUID = "does-not-exist"
						})

						It("returns unprocessable entity error", func() {
							var unprocessableEntityErr apierrors.UnprocessableEntityError
							Expect(errors.As(createErr, &unprocessableEntityErr)).To(BeTrue())
							Expect(unprocessableEntityErr).To(MatchError(ContainSubstring("does-not-exist")))
						})
					})
				})
			})

			When("the service plan does not exist", func() {
				BeforeEach(func() {
					serviceInstanceCreateMessage.PlanGUID = "does-not-exist"
				})

				It("returns unprocessable entity error", func() {
					var unprocessableEntityErr apierrors.UnprocessableEntityError
					Expect(errors.As(createErr, &unprocessableEntityErr)).To(BeTrue())
					Expect(unprocessableEntityErr).To(MatchError(ContainSubstring("does-not-exist")))
				})
			})
		})
	})

	Describe("instance record last operation", func() {
		var (
			cfServiceInstance     *korifiv1alpha1.CFServiceInstance
			serviceInstanceRecord repositories.ServiceInstanceRecord
		)

		BeforeEach(func() {
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)

			cfServiceInstance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					Type: korifiv1alpha1.ManagedType,
				},
			}
			Expect(k8sClient.Create(ctx, cfServiceInstance)).To(Succeed())

			Expect(k8s.Patch(ctx, k8sClient, cfServiceInstance, func() {
				cfServiceInstance.Status.LastOperation = korifiv1alpha1.LastOperation{
					Type:        "create",
					State:       "failed",
					Description: "failed due to error",
				}
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			var err error
			serviceInstanceRecord, err = serviceInstanceRepo.GetServiceInstance(ctx, authInfo, cfServiceInstance.Name)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns  last operation", func() {
			Expect(serviceInstanceRecord.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
				Type:        "create",
				State:       "failed",
				Description: "failed due to error",
			}))
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
			state             repositories.ResourceState
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
				Expect(state).To(Equal(repositories.ResourceStateUnknown))
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
					Expect(state).To(Equal(repositories.ResourceStateReady))
				})

				When("the ready status is stale ", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, cfServiceInstance, func() {
							cfServiceInstance.Status.ObservedGeneration = -1
						})).To(Succeed())
					})

					It("returns unknown state", func() {
						Expect(stateErr).NotTo(HaveOccurred())
						Expect(state).To(Equal(repositories.ResourceStateUnknown))
					})
				})
			})
		})
	})

	Describe("PatchServiceInstance", func() {
		var (
			cfServiceInstance     *korifiv1alpha1.CFServiceInstance
			secret                *corev1.Secret
			serviceInstanceGUID   string
			serviceInstanceRecord repositories.ServiceInstanceRecord
			patchMessage          repositories.PatchUPSIMessage
			err                   error
		)

		BeforeEach(func() {
			serviceInstanceGUID = uuid.NewString()
			secretName := uuid.NewString()
			cfServiceInstance = createUserProvidedServiceInstanceCR(ctx, k8sClient, serviceInstanceGUID, space.Name, serviceInstanceName, secretName)
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

			patchMessage = repositories.PatchUPSIMessage{
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
			serviceInstanceRecord, err = serviceInstanceRepo.PatchUserProvidedServiceInstance(ctx, authInfo, patchMessage)
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

		When("Patching a managed service instance", func() {
			var (
				patchMessage repositories.PatchManagedSIMessage
			)

			BeforeEach(func() {
				serviceInstancePlanGUID := uuid.NewString()
				cfServiceInstance = createManagedServiceInstanceCR(ctx, k8sClient, serviceInstanceGUID, space.Name, serviceInstanceName, serviceInstancePlanGUID)
				conditionAwaiter.AwaitConditionReturns(cfServiceInstance, nil)

				Expect(k8sClient.Create(ctx, cfServiceInstance)).To(Succeed())

				patchMessage = repositories.PatchManagedSIMessage{
					GUID:      cfServiceInstance.Name,
					SpaceGUID: space.Name,
					Name:      tools.PtrTo("new-name"),
					PlanGUID:  tools.PtrTo("new-plan-guid"),
					Tags:      &[]string{"new"},
					MetadataPatch: repositories.MetadataPatch{
						Labels:      map[string]*string{"new-label": tools.PtrTo("new-label-value")},
						Annotations: map[string]*string{"new-annotation": tools.PtrTo("new-annotation-value")},
					},
				}
			})

			JustBeforeEach(func() {
				serviceInstanceRecord, err = serviceInstanceRepo.PatchManagedServiceInstance(ctx, authInfo, patchMessage)
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
					Expect(serviceInstanceRecord.Labels).To(HaveKeyWithValue("a-label", "a-label-value"))
					Expect(serviceInstanceRecord.Labels).To(HaveKeyWithValue("new-label", "new-label-value"))
					Expect(serviceInstanceRecord.Annotations).To(HaveLen(2))
					Expect(serviceInstanceRecord.Annotations).To(HaveKeyWithValue("an-annotation", "an-annotation-value"))
					Expect(serviceInstanceRecord.Annotations).To(HaveKeyWithValue("new-annotation", "new-annotation-value"))
					Expect(serviceInstanceRecord.Relationships()).To(HaveKeyWithValue("service_plan", "new-plan-guid"))
				})

				It("updates the service instance", func() {
					Expect(err).NotTo(HaveOccurred())
					serviceInstance := new(korifiv1alpha1.CFServiceInstance)

					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), serviceInstance)).To(Succeed())
						g.Expect(serviceInstance.Spec.DisplayName).To(Equal("new-name"))
						g.Expect(serviceInstance.Spec.Tags).To(ConsistOf("new"))
						g.Expect(serviceInstance.Spec.PlanGUID).To(Equal("new-plan-guid"))
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

				When("the plan GUID is nil", func() {
					BeforeEach(func() {
						patchMessage.PlanGUID = nil
					})

					It("preserves the plan GUID", func() {
						Expect(err).NotTo(HaveOccurred())
						serviceInstance := new(korifiv1alpha1.CFServiceInstance)

						Consistently(func(g Gomega) {
							g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), serviceInstance)).To(Succeed())
							g.Expect(serviceInstance.Spec.PlanGUID).To(Equal("plan-guid"))
						}).Should(Succeed())
					})
				})
			})
		})
	})

	Describe("ListServiceInstances", func() {
		var (
			cfServiceInstance1 *korifiv1alpha1.CFServiceInstance
			filters            repositories.ListServiceInstanceMessage
			listErr            error

			serviceInstanceList repositories.ListResult[repositories.ServiceInstanceRecord]
		)

		BeforeEach(func() {
			cfServiceInstance1 = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space.Name,
					Name:      "service-instance-1" + uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: "service-instance-1",
					Type:        korifiv1alpha1.UserProvidedType,
					PlanGUID:    "plan-1",
				},
			}
			Expect(k8sClient.Create(ctx, cfServiceInstance1)).To(Succeed())

			filters = repositories.ListServiceInstanceMessage{OrderBy: "name"}
		})

		JustBeforeEach(func() {
			serviceInstanceList, listErr = serviceInstanceRepo.ListServiceInstances(ctx, authInfo, filters)
		})

		It("returns an empty list of ServiceInstanceRecord", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(serviceInstanceList.Records).To(BeEmpty())
		})

		When("user is allowed to list service instances ", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns the service instances from the allowed namespaces", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(serviceInstanceList.Records).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance1.Name)}),
				))
			})

			DescribeTable("ordering",
				func(msg repositories.ListServiceInstanceMessage, match gomega_types.GomegaMatcher) {
					fakeKlient := new(fake.Klient)
					instancesRepo := repositories.NewServiceInstanceRepo(fakeKlient, conditionAwaiter, rootNamespace)

					_, err := instancesRepo.ListServiceInstances(ctx, authInfo, msg)
					Expect(err).NotTo(HaveOccurred())

					Expect(fakeKlient.ListCallCount()).To(Equal(1))
					_, _, listOptions := fakeKlient.ListArgsForCall(0)
					Expect(listOptions).To(match)
				},
				Entry("name", repositories.ListServiceInstanceMessage{OrderBy: "name"}, ContainElement(repositories.SortOpt{By: "Display Name", Desc: false})),
				Entry("-name", repositories.ListServiceInstanceMessage{OrderBy: "-name"}, ContainElement(repositories.SortOpt{By: "Display Name", Desc: true})),
			)

			Describe("list options", func() {
				var fakeKlient *fake.Klient

				BeforeEach(func() {
					fakeKlient = new(fake.Klient)
					serviceInstanceRepo = repositories.NewServiceInstanceRepo(fakeKlient, conditionAwaiter, rootNamespace)
					filters = repositories.ListServiceInstanceMessage{
						Names:         []string{"instance-1", "instance-2"},
						SpaceGUIDs:    []string{"space-guid-1", "space-guid-2"},
						GUIDs:         []string{"guid-1", "guid-2"},
						LabelSelector: "a-label=a-label-value",
						PlanGUIDs:     []string{"plan-guid-1", "plan-guid-2"},
						OrderBy:       "updated_at",
						Pagination: repositories.Pagination{
							PerPage: 1,
							Page:    10,
						},
					}
				})

				It("translates  parameters to klient list options", func() {
					Expect(fakeKlient.ListCallCount()).To(Equal(1))
					_, _, listOptions := fakeKlient.ListArgsForCall(0)
					Expect(listOptions).To(ConsistOf(
						repositories.WithLabelIn(korifiv1alpha1.DisplayNameLabelKey, tools.EncodeValuesToSha224("instance-1", "instance-2")),
						repositories.WithLabelIn(korifiv1alpha1.SpaceGUIDLabelKey, []string{"space-guid-1", "space-guid-2"}),
						repositories.WithLabelIn(korifiv1alpha1.GUIDLabelKey, []string{"guid-1", "guid-2"}),
						repositories.WithLabelSelector("a-label=a-label-value"),
						repositories.WithLabelIn(korifiv1alpha1.PlanGUIDLabelKey, []string{"plan-guid-1", "plan-guid-2"}),
						repositories.WithOrdering("updated_at"),
						repositories.WithPaging(repositories.Pagination{
							PerPage: 1,
							Page:    10,
						}),
					))
				})

				When("filtering by type", func() {
					BeforeEach(func() {
						filters.Type = "managed"
					})

					It("adds the type to the list options", func() {
						Expect(fakeKlient.ListCallCount()).To(Equal(1))
						_, _, listOptions := fakeKlient.ListArgsForCall(0)
						Expect(listOptions).To(ContainElement(repositories.WithLabel(korifiv1alpha1.CFServiceInstanceTypeLabelKey, "managed")))
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

			serviceInstance = createUserProvidedServiceInstanceCR(ctx, k8sClient, prefixedGUID("service-instance"), space.Name, "the-service-instance", prefixedGUID("secret"))
			createUserProvidedServiceInstanceCR(ctx, k8sClient, prefixedGUID("service-instance"), space2.Name, "some-other-service-instance", prefixedGUID("secret"))
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
				Expect(record.Tags).To(Equal(serviceInstance.Spec.Tags))
				Expect(record.Type).To(Equal(string(serviceInstance.Spec.Type)))
				Expect(record.Labels).To(HaveKeyWithValue("a-label", "a-label-value"))
				Expect(record.Annotations).To(Equal(map[string]string{"an-annotation": "an-annotation-value"}))
				Expect(record.Relationships()).To(Equal(map[string]string{
					"space": serviceInstance.Namespace,
				}))
				Expect(record.MaintenanceInfo).To(BeZero())
				Expect(record.UpgradeAvailable).To(BeFalse())
			})

			When("the service is managed", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, serviceInstance, func() {
						serviceInstance.Spec.Type = korifiv1alpha1.ManagedType
						serviceInstance.Spec.PlanGUID = "plan-guid"
						serviceInstance.Status.MaintenanceInfo = korifiv1alpha1.MaintenanceInfo{
							Version: "1.2.3",
						}
						serviceInstance.Status.UpgradeAvailable = true
					})).To(Succeed())
				})

				It("returns service plan relationships for user provided provided services", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(record.Relationships()).To(Equal(map[string]string{
						"space":        serviceInstance.Namespace,
						"service_plan": "plan-guid",
					}))
				})

				It("returns the maintenance info", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(record.MaintenanceInfo).To(Equal(repositories.MaintenanceInfo{
						Version: "1.2.3",
					}))
				})

				It("returns upgrade available", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(record.UpgradeAvailable).To(BeTrue())
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
				createUserProvidedServiceInstanceCR(ctx, k8sClient, getGUID, space2.Name, "the-service-instance", prefixedGUID("secret"))
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

	Describe("GetServiceInstanceCredentials", func() {
		var (
			instanceGUID string
			secretName   string
			credential   map[string]any
			secret       *corev1.Secret
			getErr       error
		)

		BeforeEach(func() {
			instanceGUID = prefixedGUID("service-instance")
			secretName = prefixedGUID("secret")

			serviceInstance := &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					SecretName: secretName,
					Type:       "user-provided",
					Tags:       []string{"database", "mysql"},
				},
			}
			Expect(k8sClient.Create(ctx, serviceInstance)).To(Succeed())

			secretData, decodeErr := tools.ToCredentialsSecretData(map[string]any{"foo": "bar"})
			Expect(decodeErr).ToNot(HaveOccurred())

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: space.Name,
				},
				Data: secretData,
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		})

		JustBeforeEach(func() {
			credential, getErr = serviceInstanceRepo.GetServiceInstanceCredentials(ctx, authInfo, instanceGUID)
		})

		When("there are not permissions on the secret", func() {
			It("returns a forbidden error", func() {
				Expect(errors.As(getErr, &apierrors.ForbiddenError{})).To(BeTrue())
			})
		})

		When("the user has permissions to get the secret", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns the correct secret", func() {
				Expect(getErr).ToNot(HaveOccurred())
				Expect(credential).To(HaveKeyWithValue("foo", "bar"))
			})

			When("the service instance does not exist", func() {
				BeforeEach(func() {
					instanceGUID = "does-not-exist"
				})

				It("returns a 404 error", func() {
					Expect(errors.As(getErr, &apierrors.NotFoundError{})).To(BeTrue())
				})
			})

			When("the secret data is invalid json", func() {
				BeforeEach(func() {
					secret.Data = map[string][]byte{tools.CredentialsSecretKey: []byte("not-json")}
					Expect(k8sClient.Update(ctx, secret)).To(Succeed())
				})

				It("returns a unprocessiable entity error", func() {
					Expect(errors.As(getErr, &apierrors.UnprocessableEntityError{})).To(BeTrue())
				})
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
			serviceInstance = createUserProvidedServiceInstanceCR(ctx, k8sClient, prefixedGUID("service-instance"), space.Name, "the-service-instance", prefixedGUID("secret"))

			deleteMessage = repositories.DeleteServiceInstanceMessage{
				GUID: serviceInstance.Name,
			}
		})

		JustBeforeEach(func() {
			_, deleteErr = serviceInstanceRepo.DeleteServiceInstance(ctx, authInfo, deleteMessage)
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

				err := k8sClient.Get(ctx, namespacedName, &korifiv1alpha1.CFServiceInstance{})
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

			When("purge parameter is set", func() {
				BeforeEach(func() {
					deleteMessage.Purge = true

					Expect(k8s.Patch(ctx, k8sClient, serviceInstance, func() {
						serviceInstance.Finalizers = []string{"do-not-delete-me"}
					})).To(Succeed())
				})

				It("requests deprovisioning without broker", func() {
					actualServiceInstance := &korifiv1alpha1.CFServiceInstance{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serviceInstance.Name, Namespace: space.Name}, actualServiceInstance)).To(Succeed())
					Expect(actualServiceInstance.Annotations).To(HaveKeyWithValue(korifiv1alpha1.DeprovisionWithoutBrokerAnnotation, "true"))
				})

				It("deletes the instance", func() {
					actualServiceInstance := &korifiv1alpha1.CFServiceInstance{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serviceInstance.Name, Namespace: space.Name}, actualServiceInstance)).To(Succeed())
					Expect(actualServiceInstance.DeletionTimestamp).NotTo(BeZero())
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
