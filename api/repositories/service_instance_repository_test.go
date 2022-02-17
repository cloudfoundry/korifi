package repositories_test

import (
	"context"
	"time"

	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"

	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ServiceInstanceRepository", func() {
	var (
		testCtx                   context.Context
		serviceInstanceRepo       *ServiceInstanceRepo
		spaceDeveloperClusterRole *rbacv1.ClusterRole
		org                       *hnsv1alpha2.SubnamespaceAnchor
	)

	BeforeEach(func() {
		testCtx = context.Background()
		serviceInstanceRepo = NewServiceInstanceRepo(userClientFactory, nsPerms)
		spaceDeveloperClusterRole = createClusterRole(testCtx, SpaceDeveloperClusterRoleRules)

		rootNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}}
		Expect(k8sClient.Create(testCtx, rootNs)).To(Succeed())
		org = createOrgAnchorAndNamespace(testCtx, rootNamespace, prefixedGUID("org"))
	})

	Describe("CreateServiceInstance", func() {
		const (
			testServiceInstanceName = "my-uspi"
		)

		var (
			serviceInstanceCreateMessage CreateServiceInstanceMessage
			spaceGUID                    string
			serviceInstanceTags          []string
		)

		BeforeEach(func() {
			spaceGUID = generateGUID()

			serviceInstanceTags = []string{"foo", "bar"}

			Expect(k8sClient.Create(testCtx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: spaceGUID},
			})).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(testCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}})
			})

			serviceInstanceCreateMessage = initializeServiceInstanceCreateMessage(testServiceInstanceName, spaceGUID, serviceInstanceTags)
		})

		When("user has permissions to create ServiceInstances", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperClusterRole.Name, spaceGUID)
			})

			It("creates a new ServiceInstance CR", func() {
				createdServiceInstanceRecord, err := serviceInstanceRepo.CreateServiceInstance(testCtx, authInfo, serviceInstanceCreateMessage)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdServiceInstanceRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "record GUID was not a 36 character guid")
				Expect(createdServiceInstanceRecord.SpaceGUID).To(Equal(spaceGUID), "SpaceGUID in record did not match input")
				Expect(createdServiceInstanceRecord.Name).To(Equal(testServiceInstanceName), "Name in record did not match input")
				Expect(createdServiceInstanceRecord.Type).To(Equal("user-provided"), "Type in record did not match input")
				Expect(createdServiceInstanceRecord.Tags).To(ConsistOf([]string{"foo", "bar"}), "Tags in record did not match input")

				recordCreatedTime, err := time.Parse(TimestampFormat, createdServiceInstanceRecord.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(recordCreatedTime).To(BeTemporally("~", time.Now(), 2*time.Second))

				recordUpdatedTime, err := time.Parse(TimestampFormat, createdServiceInstanceRecord.UpdatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(recordUpdatedTime).To(BeTemporally("~", time.Now(), 2*time.Second))
			})

			When("no ServiceInstance credentials are given", func() {
				It("creates a secret and sets the secret ref on the ServiceInstance", func() {
					createdServceInstanceRecord, err := serviceInstanceRepo.CreateServiceInstance(testCtx, authInfo, serviceInstanceCreateMessage)
					Expect(err).NotTo(HaveOccurred())
					Expect(createdServceInstanceRecord).NotTo(BeNil())
					Expect(createdServceInstanceRecord.SecretName).To(Equal(createdServceInstanceRecord.GUID))

					secretLookupKey := types.NamespacedName{Name: createdServceInstanceRecord.SecretName, Namespace: createdServceInstanceRecord.SpaceGUID}
					createdSecret := new(corev1.Secret)
					Eventually(func() error {
						return k8sClient.Get(context.Background(), secretLookupKey, createdSecret)
					}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

					Expect(createdSecret.Data).To(MatchAllKeys(Keys{
						"type": BeEquivalentTo("user-provided"),
					}))
					Expect(createdSecret.Type).To(Equal(corev1.SecretType("servicebinding.io/user-provided")))
				})
			})

			When("ServiceInstance credentials are given", func() {
				BeforeEach(func() {
					serviceInstanceCreateMessage.Credentials = map[string]string{
						"type": "i get clobbered",
						"foo":  "bar",
						"baz":  "baz",
					}
				})

				It("creates a secret and sets the secret ref on the ServiceInstance", func() {
					createdServceInstanceRecord, err := serviceInstanceRepo.CreateServiceInstance(testCtx, authInfo, serviceInstanceCreateMessage)
					Expect(err).NotTo(HaveOccurred())
					Expect(createdServceInstanceRecord).NotTo(BeNil())
					Expect(createdServceInstanceRecord.SecretName).To(Equal(createdServceInstanceRecord.GUID))

					secretLookupKey := types.NamespacedName{Name: createdServceInstanceRecord.SecretName, Namespace: createdServceInstanceRecord.SpaceGUID}
					createdSecret := new(corev1.Secret)
					Eventually(func() error {
						return k8sClient.Get(context.Background(), secretLookupKey, createdSecret)
					}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

					Expect(createdSecret.Data).To(MatchAllKeys(Keys{
						"type": BeEquivalentTo("user-provided"),
						"foo":  BeEquivalentTo("bar"),
						"baz":  BeEquivalentTo("baz"),
					}))
				})
			})
		})

		When("user does not have permissions to create ServiceInstances", func() {
			It("returns a Forbidden error", func() {
				_, err := serviceInstanceRepo.CreateServiceInstance(testCtx, authInfo, serviceInstanceCreateMessage)
				Expect(err).To(BeAssignableToTypeOf(ForbiddenError{}))
			})
		})
	})

	Describe("ListServiceInstances", Serial, func() {
		var (
			space1, space2, space3                                     *hnsv1alpha2.SubnamespaceAnchor
			cfServiceInstance1, cfServiceInstance2, cfServiceInstance3 *servicesv1alpha1.CFServiceInstance
			nonCFNamespace                                             string
			filters                                                    ListServiceInstanceMessage
		)

		BeforeEach(func() {
			space1 = createSpaceAnchorAndNamespace(testCtx, org.Name, prefixedGUID("space1"))
			space2 = createSpaceAnchorAndNamespace(testCtx, org.Name, prefixedGUID("space2"))
			space3 = createSpaceAnchorAndNamespace(testCtx, org.Name, prefixedGUID("space3"))

			nonCFNamespace = prefixedGUID("non-cf")
			Expect(k8sClient.Create(
				testCtx,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nonCFNamespace}},
			)).To(Succeed())

			DeferCleanup(func() {
				_ = k8sClient.Delete(testCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})
				_ = k8sClient.Delete(testCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nonCFNamespace}})
				_ = k8sClient.Delete(testCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org.Name}})
				_ = k8sClient.Delete(testCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: space1.Name}})
				_ = k8sClient.Delete(testCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: space2.Name}})
				_ = k8sClient.Delete(testCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: space3.Name}})
			})

			cfServiceInstance1 = createServiceInstance(space1.Name, "service-instance-1")
			cfServiceInstance2 = createServiceInstance(space2.Name, "service-instance-2")
			cfServiceInstance3 = createServiceInstance(space3.Name, "service-instance-3")
			createServiceInstance(nonCFNamespace, "service-instance-dummy")
		})

		When("query parameters are not provided and", func() {
			BeforeEach(func() {
				filters = ListServiceInstanceMessage{}
			})

			When("no service instances exist in spaces where the user has permission", func() {
				It("returns an empty list of ServiceInstanceRecord", func() {
					serviceInstanceList, err := serviceInstanceRepo.ListServiceInstances(testCtx, authInfo, filters)
					Expect(err).NotTo(HaveOccurred())
					Expect(serviceInstanceList).To(BeEmpty())
				})
			})

			When("multiple service instances exist in spaces where the user has permissions", func() {
				BeforeEach(func() {
					createRoleBinding(testCtx, userName, spaceDeveloperClusterRole.Name, space1.Name)
					createRoleBinding(testCtx, userName, spaceDeveloperClusterRole.Name, space2.Name)
				})

				It("eventually returns ServiceInstance records from only the spaces where the user has permission", func() {
					Eventually(func() []ServiceInstanceRecord {
						serviceInstanceList, err := serviceInstanceRepo.ListServiceInstances(testCtx, authInfo, filters)
						Expect(err).NotTo(HaveOccurred())
						return serviceInstanceList
					}, timeCheckThreshold*time.Second).Should(ConsistOf(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance1.Name)}),
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance2.Name)}),
					))
				})
			})
		})

		When("query parameters are provided", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperClusterRole.Name, space1.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperClusterRole.Name, space2.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperClusterRole.Name, space3.Name)
			})

			When("the name filter is set", func() {
				BeforeEach(func() {
					filters = ListServiceInstanceMessage{
						Names: []string{
							cfServiceInstance1.Spec.Name,
							cfServiceInstance3.Spec.Name,
						},
					}
				})
				It("eventually returns only records for the ServiceInstances with matching spec.name fields", func() {
					Eventually(func() []ServiceInstanceRecord {
						serviceInstanceList, err := serviceInstanceRepo.ListServiceInstances(testCtx, authInfo, filters)
						Expect(err).NotTo(HaveOccurred())
						return serviceInstanceList
					}, timeCheckThreshold*time.Second).Should(ConsistOf(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance1.Name)}),
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance3.Name)}),
					))
				})
			})

			When("the spaceGUID filter is set", func() {
				BeforeEach(func() {
					filters = ListServiceInstanceMessage{
						SpaceGuids: []string{
							cfServiceInstance2.Namespace,
							cfServiceInstance3.Namespace,
						},
					}
				})
				It("eventually returns only records for the ServiceInstances within the matching spaces", func() {
					Eventually(func() []ServiceInstanceRecord {
						serviceInstanceList, err := serviceInstanceRepo.ListServiceInstances(testCtx, authInfo, filters)
						Expect(err).NotTo(HaveOccurred())
						return serviceInstanceList
					}, timeCheckThreshold*time.Second).Should(ConsistOf(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance2.Name)}),
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance3.Name)}),
					))
				})
			})

			When("the OrderBy field is set to 'name'", func() {
				BeforeEach(func() {
					filters = ListServiceInstanceMessage{
						SpaceGuids: []string{
							cfServiceInstance1.Namespace,
							cfServiceInstance2.Namespace,
							cfServiceInstance3.Namespace,
						},
						OrderBy: "name",
					}
				})

				It("eventually returns the ServiceBindings in ascending name order", func() {
					var serviceInstanceList []ServiceInstanceRecord
					Eventually(func() []ServiceInstanceRecord {
						var err error
						serviceInstanceList, err = serviceInstanceRepo.ListServiceInstances(testCtx, authInfo, filters)
						Expect(err).NotTo(HaveOccurred())
						return serviceInstanceList
					}, timeCheckThreshold*time.Second).Should(HaveLen(3))
					Expect(serviceInstanceList[0]).To(MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance1.Name)}))
					Expect(serviceInstanceList[1]).To(MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance2.Name)}))
					Expect(serviceInstanceList[2]).To(MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance3.Name)}))
				})

				When("the DescendingOrder field is true", func() {
					BeforeEach(func() {
						filters.DescendingOrder = true
					})

					It("eventually returns the ServiceBindings in descending name order", func() {
						var serviceInstanceList []ServiceInstanceRecord
						Eventually(func() []ServiceInstanceRecord {
							var err error
							serviceInstanceList, err = serviceInstanceRepo.ListServiceInstances(testCtx, authInfo, filters)
							Expect(err).NotTo(HaveOccurred())
							return serviceInstanceList
						}, timeCheckThreshold*time.Second).Should(HaveLen(3))
						Expect(serviceInstanceList[0]).To(MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance3.Name)}))
						Expect(serviceInstanceList[1]).To(MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance2.Name)}))
						Expect(serviceInstanceList[2]).To(MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfServiceInstance1.Name)}))
					})
				})
			})

			When("the OrderBy field is set to 'created_at'", func() {
				BeforeEach(func() {
					filters = ListServiceInstanceMessage{
						SpaceGuids: []string{
							cfServiceInstance1.Namespace,
							cfServiceInstance2.Namespace,
							cfServiceInstance3.Namespace,
						},
						OrderBy: "created_at",
					}
					time.Sleep(time.Second)
					createServiceInstance(space3.Name, "another-service-instance")
				})

				It("eventually returns the ServiceBindings in ascending creation order", func() {
					var serviceInstanceList []ServiceInstanceRecord
					Eventually(func() []ServiceInstanceRecord {
						var err error
						serviceInstanceList, err = serviceInstanceRepo.ListServiceInstances(testCtx, authInfo, filters)
						Expect(err).NotTo(HaveOccurred())
						return serviceInstanceList
					}, timeCheckThreshold*time.Second).Should(HaveLen(4))
					createTime1, err := time.Parse(time.RFC3339, serviceInstanceList[0].CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					createTime2, err := time.Parse(time.RFC3339, serviceInstanceList[1].CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					createTime3, err := time.Parse(time.RFC3339, serviceInstanceList[2].CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					createTime4, err := time.Parse(time.RFC3339, serviceInstanceList[3].CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(createTime1).To(BeTemporally("<=", createTime2))
					Expect(createTime2).To(BeTemporally("<=", createTime3))
					Expect(createTime3).To(BeTemporally("<=", createTime4))
				})

				When("the DescendingOrder field is true", func() {
					BeforeEach(func() {
						filters.DescendingOrder = true
					})

					It("eventually returns the ServiceBindings in descending creation order", func() {
						var serviceInstanceList []ServiceInstanceRecord
						Eventually(func() []ServiceInstanceRecord {
							var err error
							serviceInstanceList, err = serviceInstanceRepo.ListServiceInstances(testCtx, authInfo, filters)
							Expect(err).NotTo(HaveOccurred())
							return serviceInstanceList
						}, timeCheckThreshold*time.Second).Should(HaveLen(4))
						createTime1, err := time.Parse(time.RFC3339, serviceInstanceList[0].CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						createTime2, err := time.Parse(time.RFC3339, serviceInstanceList[1].CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						createTime3, err := time.Parse(time.RFC3339, serviceInstanceList[2].CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						createTime4, err := time.Parse(time.RFC3339, serviceInstanceList[3].CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(createTime1).To(BeTemporally(">=", createTime2))
						Expect(createTime2).To(BeTemporally(">=", createTime3))
						Expect(createTime3).To(BeTemporally(">=", createTime4))
					})
				})
			})

			When("the OrderBy field is set to 'updated_at'", func() {
				BeforeEach(func() {
					filters = ListServiceInstanceMessage{
						SpaceGuids: []string{
							cfServiceInstance1.Namespace,
							cfServiceInstance2.Namespace,
							cfServiceInstance3.Namespace,
						},
						OrderBy: "updated_at",
					}
					time.Sleep(time.Second)
					createServiceInstance(space3.Name, "another-service-instance")
				})

				It("eventually returns the ServiceBindings in ascending update order", func() {
					var serviceInstanceList []ServiceInstanceRecord
					Eventually(func() []ServiceInstanceRecord {
						var err error
						serviceInstanceList, err = serviceInstanceRepo.ListServiceInstances(testCtx, authInfo, filters)
						Expect(err).NotTo(HaveOccurred())
						return serviceInstanceList
					}, timeCheckThreshold*time.Second).Should(HaveLen(4))
					updateTime1, err := time.Parse(time.RFC3339, serviceInstanceList[0].UpdatedAt)
					Expect(err).NotTo(HaveOccurred())
					updateTime2, err := time.Parse(time.RFC3339, serviceInstanceList[1].UpdatedAt)
					Expect(err).NotTo(HaveOccurred())
					updateTime3, err := time.Parse(time.RFC3339, serviceInstanceList[2].UpdatedAt)
					Expect(err).NotTo(HaveOccurred())
					updateTime4, err := time.Parse(time.RFC3339, serviceInstanceList[3].UpdatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(updateTime1).To(BeTemporally("<=", updateTime2))
					Expect(updateTime2).To(BeTemporally("<=", updateTime3))
					Expect(updateTime3).To(BeTemporally("<=", updateTime4))
				})

				When("the DescendingOrder field is true", func() {
					BeforeEach(func() {
						filters.DescendingOrder = true
					})

					It("eventually returns the ServiceBindings in descending update order", func() {
						var serviceInstanceList []ServiceInstanceRecord
						Eventually(func() []ServiceInstanceRecord {
							var err error
							serviceInstanceList, err = serviceInstanceRepo.ListServiceInstances(testCtx, authInfo, filters)
							Expect(err).NotTo(HaveOccurred())
							return serviceInstanceList
						}, timeCheckThreshold*time.Second).Should(HaveLen(4))
						updateTime1, err := time.Parse(time.RFC3339, serviceInstanceList[0].UpdatedAt)
						Expect(err).NotTo(HaveOccurred())
						updateTime2, err := time.Parse(time.RFC3339, serviceInstanceList[1].UpdatedAt)
						Expect(err).NotTo(HaveOccurred())
						updateTime3, err := time.Parse(time.RFC3339, serviceInstanceList[2].UpdatedAt)
						Expect(err).NotTo(HaveOccurred())
						updateTime4, err := time.Parse(time.RFC3339, serviceInstanceList[3].UpdatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(updateTime1).To(BeTemporally(">=", updateTime2))
						Expect(updateTime2).To(BeTemporally(">=", updateTime3))
						Expect(updateTime3).To(BeTemporally(">=", updateTime4))
					})
				})
			})
		})
	})

	Describe("GetServiceInstance", Serial, func() {
		var (
			cfServiceInstance *servicesv1alpha1.CFServiceInstance
			space             *hnsv1alpha2.SubnamespaceAnchor
		)

		const (
			serviceInstanceName = "test-service-instance"
		)

		BeforeEach(func() {
			space = createSpaceAnchorAndNamespace(testCtx, org.Name, prefixedGUID("space1"))
			cfServiceInstance = createServiceInstance(space.Name, serviceInstanceName)
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(testCtx, cfServiceInstance)).To(Succeed())
			Expect(k8sClient.Delete(testCtx, space)).To(Succeed())
		})

		When("the user is authorized in the namespace", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperClusterRole.Name, space.Name)
			})

			It("returns the matching ServiceInstance record", func() {
				record, err := serviceInstanceRepo.GetServiceInstance(testCtx, authInfo, cfServiceInstance.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(record.GUID).To(Equal(cfServiceInstance.Name))
				Expect(record.Name).To(Equal(serviceInstanceName))
				Expect(record.SpaceGUID).To(Equal(space.Name))
				Expect(record.Tags).To(BeEquivalentTo(cfServiceInstance.Spec.Tags))
				Expect(record.Type).To(BeEquivalentTo(cfServiceInstance.Spec.Type))

				createdAt, err := time.Parse(time.RFC3339, record.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

				updatedAt, err := time.Parse(time.RFC3339, record.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
			})
		})

		When("user is not authorized to get a package", func() {
			It("returns a not found error", func() {
				_, err := serviceInstanceRepo.GetServiceInstance(testCtx, authInfo, cfServiceInstance.Name)
				Expect(err).To(BeAssignableToTypeOf(repositories.NotFoundError{}))
			})
		})

		When("the Service Instance doesn't exist", func() {
			It("returns a not found error", func() {
				_, err := serviceInstanceRepo.GetServiceInstance(testCtx, authInfo, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(repositories.NotFoundError{}))
			})
		})
	})
})

func initializeServiceInstanceCreateMessage(serviceInstanceName string, spaceGUID string, tags []string) CreateServiceInstanceMessage {
	return CreateServiceInstanceMessage{
		Name:      serviceInstanceName,
		SpaceGUID: spaceGUID,
		Type:      "user-provided",
		Tags:      tags,
	}
}

func createServiceInstance(space, name string) *servicesv1alpha1.CFServiceInstance {
	serviceInstanceGUID := generateGUID()
	cfServiceInstance := &servicesv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceInstanceGUID,
			Namespace: space,
		},
		Spec: servicesv1alpha1.CFServiceInstanceSpec{
			Name:       name,
			SecretName: serviceInstanceGUID,
			Type:       "user-provided",
		},
	}
	Expect(k8sClient.Create(context.Background(), cfServiceInstance)).To(Succeed())

	return cfServiceInstance
}
