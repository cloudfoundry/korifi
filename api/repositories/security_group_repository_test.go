package repositories_test

import (
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("SecurityGroupRepo", func() {
	var (
		repo  *repositories.SecurityGroupRepo
		org   *korifiv1alpha1.CFOrg
		space *korifiv1alpha1.CFSpace
	)

	BeforeEach(func() {
		repo = repositories.NewSecurityGroupRepo(userClientFactory, rootNamespace)
		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))
	})

	Describe("CreateSecurityGroup", func() {
		var (
			securityGroupRecord repositories.SecurityGroupRecord
			createErr           error
		)

		JustBeforeEach(func() {
			securityGroupRecord, createErr = repo.CreateSecurityGroup(ctx, authInfo, repositories.CreateSecurityGroupMessage{
				DisplayName: "test-security-group",
				Rules: []korifiv1alpha1.SecurityGroupRule{
					{
						Protocol:    korifiv1alpha1.ProtocolTCP,
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				},
				Spaces: map[string]korifiv1alpha1.SecurityGroupWorkloads{
					space.Name: {Running: true, Staging: true},
				},
			})
		})

		It("errors with forbidden for users with no permissions", func() {
			Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a CF admin", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			It("creates a CFSecurityGroup successfully", func() {
				Expect(createErr).ToNot(HaveOccurred())
				securityGroup := new(korifiv1alpha1.CFSecurityGroup)
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: securityGroupRecord.GUID, Namespace: rootNamespace}, securityGroup)).To(Succeed())

				Expect(securityGroup.Spec.DisplayName).To(Equal("test-security-group"))
				Expect(securityGroup.Spec.Rules).To(Equal([]korifiv1alpha1.SecurityGroupRule{
					{
						Protocol:    korifiv1alpha1.ProtocolTCP,
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				}))

				Expect(securityGroup.Spec.Spaces).To(Equal(map[string]korifiv1alpha1.SecurityGroupWorkloads{
					space.Name: {Running: true, Staging: true},
				}))
			})
		})
	})
})
