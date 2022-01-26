package repositories_test

import (
	"context"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"

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
		clientFactory             repositories.UserK8sClientFactory
		spaceDeveloperClusterRole *rbacv1.ClusterRole
	)

	BeforeEach(func() {
		testCtx = context.Background()
		clientFactory = repositories.NewUnprivilegedClientFactory(k8sConfig)
		serviceInstanceRepo = NewServiceInstanceRepo(clientFactory)
		spaceDeveloperClusterRole = createClusterRole(testCtx, SpaceDeveloperClusterRoleRules)
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
				It("creates an empty secret and sets the secret ref on the ServiceInstance", func() {
					createdServceInstanceRecord, err := serviceInstanceRepo.CreateServiceInstance(testCtx, authInfo, serviceInstanceCreateMessage)
					Expect(err).NotTo(HaveOccurred())
					Expect(createdServceInstanceRecord).NotTo(BeNil())
					Expect(createdServceInstanceRecord.SecretName).To(Equal(createdServceInstanceRecord.GUID))

					secretLookupKey := types.NamespacedName{Name: createdServceInstanceRecord.SecretName, Namespace: createdServceInstanceRecord.SpaceGUID}
					createdSecret := new(corev1.Secret)
					Eventually(func() error {
						return k8sClient.Get(context.Background(), secretLookupKey, createdSecret)
					}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

					Expect(createdSecret.Data).To(BeEmpty())
					Expect(createdSecret.Type).To(Equal(corev1.SecretType("servicebinding.io/user-provided")))
				})
			})

			When("ServiceInstance credentials are given", func() {
				BeforeEach(func() {
					serviceInstanceCreateMessage.Credentials = map[string]string{
						"foo": "bar",
						"baz": "baz",
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
						"foo": BeEquivalentTo("bar"),
						"baz": BeEquivalentTo("baz"),
					}))
				})
			})
		})

		When("user does not have permissions to create ServiceInstances", func() {
			It("returns a Forbidden error", func() {
				_, err := serviceInstanceRepo.CreateServiceInstance(testCtx, authInfo, serviceInstanceCreateMessage)
				Expect(err).To(BeAssignableToTypeOf(repositories.ForbiddenError{}))
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
