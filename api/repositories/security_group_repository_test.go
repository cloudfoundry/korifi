package repositories_test

import (
	"errors"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("SecurityGroupRepo", func() {
	var (
		repo  *repositories.SecurityGroupRepo
		org   *korifiv1alpha1.CFOrg
		space *korifiv1alpha1.CFSpace
	)

	BeforeEach(func() {
		repo = repositories.NewSecurityGroupRepo(klient, rootNamespace)
		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))
	})

	Describe("GetSecurityGroup", func() {
		var (
			securityGroup       *korifiv1alpha1.CFSecurityGroup
			securityGroupRecord repositories.SecurityGroupRecord
			securityGroupGUID   string
			getErr              error
		)

		BeforeEach(func() {
			securityGroupGUID = uuid.NewString()

			securityGroup = &korifiv1alpha1.CFSecurityGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      securityGroupGUID,
					Namespace: rootNamespace,
				},
				Spec: korifiv1alpha1.CFSecurityGroupSpec{
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
				},
			}
			Expect(k8sClient.Create(ctx, securityGroup)).To(Succeed())
		})

		JustBeforeEach(func() {
			securityGroupRecord, getErr = repo.GetSecurityGroup(ctx, authInfo, securityGroupGUID)
		})

		It("errors with forbidden for users with no permissions", func() {
			Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a CF admin", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			It("returns a security group record", func() {
				Expect(getErr).ToNot(HaveOccurred())

				Expect(securityGroupRecord.GUID).To(Equal(securityGroupGUID))
				Expect(securityGroupRecord.Name).To(Equal("test-security-group"))
				Expect(securityGroupRecord.GloballyEnabled.Running).To(BeFalse())
				Expect(securityGroupRecord.GloballyEnabled.Staging).To(BeFalse())
				Expect(securityGroupRecord.RunningSpaces).To(ConsistOf(space.Name))
				Expect(securityGroupRecord.StagingSpaces).To(ConsistOf(space.Name))
				Expect(securityGroupRecord.Rules).To(ConsistOf(repositories.SecurityGroupRule{
					Protocol:    korifiv1alpha1.ProtocolTCP,
					Ports:       "80",
					Destination: "192.168.1.1",
				}))
			})

			When("the security group does not exist", func() {
				BeforeEach(func() {
					securityGroupGUID = "does-not-exist"
				})

				It("returns a not found error", func() {
					notFoundErr := apierrors.NotFoundError{}
					Expect(errors.As(getErr, &notFoundErr)).To(BeTrue())
				})
			})
		})
	})

	Describe("CreateSecurityGroup", func() {
		var (
			securityGroupRecord        repositories.SecurityGroupRecord
			securityGroupCreateMessage repositories.CreateSecurityGroupMessage
			createErr                  error
		)

		BeforeEach(func() {
			securityGroupCreateMessage = repositories.CreateSecurityGroupMessage{
				DisplayName: "test-security-group",
				Rules: []repositories.SecurityGroupRule{
					{
						Protocol:    korifiv1alpha1.ProtocolTCP,
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				},
				Spaces: map[string]repositories.SecurityGroupWorkloads{
					space.Name: {Running: true, Staging: true},
				},
			}
		})

		JustBeforeEach(func() {
			securityGroupRecord, createErr = repo.CreateSecurityGroup(ctx, authInfo, securityGroupCreateMessage)
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

				cfSecurityGroup := &korifiv1alpha1.CFSecurityGroup{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      securityGroupRecord.GUID,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfSecurityGroup), cfSecurityGroup)).To(Succeed())

				Expect(cfSecurityGroup.Spec.DisplayName).To(Equal("test-security-group"))
				Expect(cfSecurityGroup.Spec.GloballyEnabled.Running).To(BeFalse())
				Expect(cfSecurityGroup.Spec.GloballyEnabled.Staging).To(BeFalse())
				Expect(cfSecurityGroup.Spec.Spaces).To(Equal(map[string]korifiv1alpha1.SecurityGroupWorkloads{
					space.Name: {Running: true, Staging: true},
				}))
				Expect(cfSecurityGroup.Spec.Rules).To(ConsistOf(korifiv1alpha1.SecurityGroupRule{
					Protocol:    korifiv1alpha1.ProtocolTCP,
					Ports:       "80",
					Destination: "192.168.1.1",
				}))
			})

			It("returns a security group record", func() {
				Expect(createErr).ToNot(HaveOccurred())

				Expect(securityGroupRecord.GUID).To(matchers.BeValidUUID())
				Expect(securityGroupRecord.Name).To(Equal("test-security-group"))
				Expect(securityGroupRecord.GloballyEnabled.Running).To(BeFalse())
				Expect(securityGroupRecord.GloballyEnabled.Staging).To(BeFalse())
				Expect(securityGroupRecord.RunningSpaces).To(ConsistOf(space.Name))
				Expect(securityGroupRecord.StagingSpaces).To(ConsistOf(space.Name))
				Expect(securityGroupRecord.Rules).To(ConsistOf(repositories.SecurityGroupRule{
					Protocol:    korifiv1alpha1.ProtocolTCP,
					Ports:       "80",
					Destination: "192.168.1.1",
				}))
			})
		})
	})

	Describe("BindSecurityGroup", func() {
		var (
			securityGroup       *korifiv1alpha1.CFSecurityGroup
			securityGroupRecord repositories.SecurityGroupRecord
			bindMessage         repositories.BindSecurityGroupMessage
			bindErr             error
		)

		BeforeEach(func() {
			securityGroup = &korifiv1alpha1.CFSecurityGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: rootNamespace,
				},
				Spec: korifiv1alpha1.CFSecurityGroupSpec{
					DisplayName: "test-security-group",
					Rules: []korifiv1alpha1.SecurityGroupRule{
						{
							Protocol:    korifiv1alpha1.ProtocolTCP,
							Ports:       "80",
							Destination: "192.168.1.1",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, securityGroup)).To(Succeed())

			bindMessage = repositories.BindSecurityGroupMessage{
				GUID:     securityGroup.Name,
				Spaces:   []string{space.Name},
				Workload: repositories.SecurityGroupRunningSpaceType,
			}
		})

		JustBeforeEach(func() {
			securityGroupRecord, bindErr = repo.BindSecurityGroup(ctx, authInfo, bindMessage)
		})

		It("errors with forbidden for users with no permissions", func() {
			Expect(bindErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a CF admin", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			It("binds the space to the cfSecurityGroup", func() {
				Expect(bindErr).ToNot(HaveOccurred())

				cfSecurityGroup := &korifiv1alpha1.CFSecurityGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      securityGroup.Name,
						Namespace: rootNamespace,
					},
				}

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfSecurityGroup), cfSecurityGroup)).To(Succeed())
				Expect(cfSecurityGroup.Spec.Spaces).To(Equal(map[string]korifiv1alpha1.SecurityGroupWorkloads{
					space.Name: {Running: true, Staging: false},
				}))
			})

			It("returns the correct record", func() {
				Expect(bindErr).ToNot(HaveOccurred())
				Expect(securityGroupRecord.RunningSpaces).To(ConsistOf([]string{space.Name}))
				Expect(securityGroupRecord.StagingSpaces).To(BeEmpty())
			})

			When("the security group does not exist", func() {
				BeforeEach(func() {
					bindMessage.GUID = "does-not-ecist"
				})

				It("returns a not found error", func() {
					notFoundErr := apierrors.NotFoundError{}
					Expect(errors.As(bindErr, &notFoundErr)).To(BeTrue())
				})
			})
		})
	})
})
