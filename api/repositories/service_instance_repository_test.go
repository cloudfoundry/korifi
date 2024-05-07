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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ServiceInstanceRepository", func() {
	var (
		testCtx             context.Context
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
		testCtx = context.Background()
		conditionAwaiter = &fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFServiceInstance,
			korifiv1alpha1.CFServiceInstanceList,
			*korifiv1alpha1.CFServiceInstanceList,
		]{}
		serviceInstanceRepo = repositories.NewServiceInstanceRepo(namespaceRetriever, userClientFactory, nsPerms, conditionAwaiter)

		org = createOrgWithCleanup(testCtx, prefixedGUID("org"))
		space = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space1"))
		serviceInstanceName = prefixedGUID("service-instance")
	})

	Describe("CreateServiceInstance", func() {
		var (
			serviceInstanceCreateMessage repositories.CreateServiceInstanceMessage
			serviceInstanceTags          []string
			serviceInstanceCredentials   map[string]any

			createdServiceInstanceRecord repositories.ServiceInstanceRecord
			createErr                    error
		)

		BeforeEach(func() {
			serviceInstanceTags = []string{"foo", "bar"}
			serviceInstanceCredentials = map[string]any{
				"object": map[string]any{"a": "b"},
			}

			serviceInstanceCreateMessage = initializeServiceInstanceCreateMessage(serviceInstanceName, space.Name, serviceInstanceTags, serviceInstanceCredentials)
		})

		JustBeforeEach(func() {
			createdServiceInstanceRecord, createErr = serviceInstanceRepo.CreateServiceInstance(testCtx, authInfo, serviceInstanceCreateMessage)
		})

		When("user has permissions to create ServiceInstances", func() {
			var createdSecret *corev1.Secret

			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			JustBeforeEach(func() {
				Expect(createErr).NotTo(HaveOccurred())

				secretLookupKey := types.NamespacedName{Name: createdServiceInstanceRecord.SecretName, Namespace: createdServiceInstanceRecord.SpaceGUID}
				createdSecret = &corev1.Secret{}
				Expect(k8sClient.Get(context.Background(), secretLookupKey, createdSecret)).To(Succeed())
			})

			It("creates a new ServiceInstance CR", func() {
				Expect(createdServiceInstanceRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "record GUID was not a 36 character guid")
				Expect(createdServiceInstanceRecord.SpaceGUID).To(Equal(space.Name), "SpaceGUID in record did not match input")
				Expect(createdServiceInstanceRecord.Name).To(Equal(serviceInstanceName), "Name in record did not match input")
				Expect(createdServiceInstanceRecord.Type).To(Equal("user-provided"), "Type in record did not match input")
				Expect(createdServiceInstanceRecord.Tags).To(ConsistOf([]string{"foo", "bar"}), "Tags in record did not match input")

				Expect(createdServiceInstanceRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(createdServiceInstanceRecord.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
			})

			It("creates the credentials secret", func() {
				Expect(createdSecret.Type).To(Equal(corev1.SecretTypeOpaque))
				Expect(createdSecret.Data).To(MatchAllKeys(Keys{korifiv1alpha1.CredentialsSecretKey: Not(BeEmpty())}))
				credentials := map[string]any{}
				Expect(json.Unmarshal(createdSecret.Data[korifiv1alpha1.CredentialsSecretKey], &credentials)).To(Succeed())
				Expect(credentials).To(Equal(serviceInstanceCredentials))
			})
		})

		When("user does not have permissions to create ServiceInstances", func() {
			It("returns a Forbidden error", func() {
				Expect(createErr).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
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
					korifiv1alpha1.CredentialsSecretKey: `{"a": "b"}`,
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
			serviceInstanceRecord, err = serviceInstanceRepo.PatchServiceInstance(testCtx, authInfo, patchMessage)
		})

		When("authorized in the space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, orgUserRole.Name, org.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
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
					g.Expect(secret.Data).To(MatchAllKeys(Keys{korifiv1alpha1.CredentialsSecretKey: Not(BeEmpty())}))
					credentials := map[string]any{}
					g.Expect(json.Unmarshal(secret.Data[korifiv1alpha1.CredentialsSecretKey], &credentials)).To(Succeed())
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
						g.Expect(secret.Data).To(MatchAllKeys(Keys{korifiv1alpha1.CredentialsSecretKey: Not(BeEmpty())}))
						credentials := map[string]any{}
						Expect(json.Unmarshal(secret.Data[korifiv1alpha1.CredentialsSecretKey], &credentials)).To(Succeed())
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
								korifiv1alpha1.CredentialsSecretKey: []byte(`{"type":"database"}`),
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
			space2, space3                                             *korifiv1alpha1.CFSpace
			cfServiceInstance1, cfServiceInstance2, cfServiceInstance3 *korifiv1alpha1.CFServiceInstance
			nonCFNamespace                                             string
			filters                                                    repositories.ListServiceInstanceMessage
			listErr                                                    error

			serviceInstanceList []repositories.ServiceInstanceRecord
		)

		BeforeEach(func() {
			space2 = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space2"))
			space3 = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space3"))

			nonCFNamespace = prefixedGUID("non-cf")
			Expect(k8sClient.Create(
				testCtx,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nonCFNamespace}},
			)).To(Succeed())

			cfServiceInstance1 = createServiceInstanceCR(testCtx, k8sClient, prefixedGUID("service-instance-1"), space.Name, "service-instance-1", prefixedGUID("secret"))
			cfServiceInstance2 = createServiceInstanceCR(testCtx, k8sClient, prefixedGUID("service-instance-2"), space2.Name, "service-instance-2", prefixedGUID("secret"))
			cfServiceInstance3 = createServiceInstanceCR(testCtx, k8sClient, prefixedGUID("service-instance-3"), space3.Name, "service-instance-3", prefixedGUID("secret"))
			createServiceInstanceCR(testCtx, k8sClient, prefixedGUID("service-instance"), nonCFNamespace, "service-instance-4", prefixedGUID("secret"))

			filters = repositories.ListServiceInstanceMessage{}
		})

		JustBeforeEach(func() {
			serviceInstanceList, listErr = serviceInstanceRepo.ListServiceInstances(testCtx, authInfo, filters)
		})

		When("no service instances exist in spaces where the user has permission", func() {
			It("returns an empty list of ServiceInstanceRecord", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(serviceInstanceList).To(BeEmpty())
			})
		})

		When("multiple service instances exist in spaces where the user has permissions", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space2.Name)
			})

			It("returns ServiceInstance records from only the spaces where the user has permission", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(serviceInstanceList).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance1.Name)}),
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance2.Name)}),
				))
			})
		})

		When("user has permissions in all spaces", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space2.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space3.Name)
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
			space2 = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space2"))

			serviceInstance = createServiceInstanceCR(testCtx, k8sClient, prefixedGUID("service-instance"), space.Name, "the-service-instance", prefixedGUID("secret"))
			createServiceInstanceCR(testCtx, k8sClient, prefixedGUID("service-instance"), space2.Name, "some-other-service-instance", prefixedGUID("secret"))
			getGUID = serviceInstance.Name
		})

		JustBeforeEach(func() {
			record, getErr = serviceInstanceRepo.GetServiceInstance(testCtx, authInfo, getGUID)
		})

		When("there are no permissions on service instances", func() {
			It("returns a forbidden error", func() {
				Expect(errors.As(getErr, &apierrors.ForbiddenError{})).To(BeTrue())
			})
		})

		When("the user has permissions to get the service instance", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space2.Name)
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
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space2.Name)
				createServiceInstanceCR(testCtx, k8sClient, getGUID, space2.Name, "the-service-instance", prefixedGUID("secret"))
			})

			It("returns a error", func() {
				Expect(getErr).To(MatchError(ContainSubstring("get-service instance duplicate records exist")))
			})
		})

		When("the context has expired", func() {
			BeforeEach(func() {
				var cancel context.CancelFunc
				testCtx, cancel = context.WithCancel(testCtx)
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
			serviceInstance = createServiceInstanceCR(testCtx, k8sClient, prefixedGUID("service-instance"), space.Name, "the-service-instance", prefixedGUID("secret"))

			deleteMessage = repositories.DeleteServiceInstanceMessage{
				GUID:      serviceInstance.Name,
				SpaceGUID: space.Name,
			}
		})

		JustBeforeEach(func() {
			deleteErr = serviceInstanceRepo.DeleteServiceInstance(testCtx, authInfo, deleteMessage)
		})

		When("the user has permissions to delete service instances", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
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

func initializeServiceInstanceCreateMessage(serviceInstanceName string, spaceGUID string, tags []string, credentials map[string]any) repositories.CreateServiceInstanceMessage {
	return repositories.CreateServiceInstanceMessage{
		Name:        serviceInstanceName,
		SpaceGUID:   spaceGUID,
		Type:        "user-provided",
		Credentials: credentials,
		Tags:        tags,
	}
}
