package integration_test

import (
	"context"
	"fmt"
	"strings"

	"code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/integration/helpers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFOrgValidatingWebhook", func() {
	var (
		ctx      context.Context
		org1Guid string
		org2Guid string
		org1Name string
		org2Name string
		org1     *v1alpha1.CFOrg
	)

	BeforeEach(func() {
		ctx = context.Background()

		org1Guid = "guid-1-" + uuid.NewString()
		org2Guid = "guid-2-" + uuid.NewString()
		org1Name = "name-1-" + uuid.NewString()
		org2Name = "name-2-" + uuid.NewString()
	})

	Describe("Create", func() {
		var createErr error

		BeforeEach(func() {
			org1 = MakeCFOrg(org1Guid, rootNamespace, org1Name)
		})

		JustBeforeEach(func() {
			createErr = k8sClient.Create(ctx, org1)
		})

		It("should succeed", func() {
			Expect(createErr).NotTo(HaveOccurred())
		})

		When("CFOrg is requested outside of root namespace", func() {
			BeforeEach(func() {
				org1.Namespace = "default"
			})

			It("should fail", func() {
				Expect(createErr.Error()).To(ContainSubstring(fmt.Sprintf("Organization '%s' must be placed in the root 'cf' namespace", org1Name)))
			})
		})

		When("another CFOrg exists with a different name in the same namespace", func() {
			BeforeEach(func() {
				org2 := MakeCFOrg(org2Guid, rootNamespace, org2Name)
				Expect(k8sClient.Create(ctx, org2)).To(Succeed())
			})

			It("should succeed", func() {
				Expect(createErr).NotTo(HaveOccurred())
			})
		})

		When("another CFOrg exists with the same name in the same namespace", func() {
			BeforeEach(func() {
				org2 := MakeCFOrg(org2Guid, rootNamespace, org1Name)
				Expect(k8sClient.Create(ctx, org2)).To(Succeed())
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring("Organization '%s' already exists.", org1Name)))
			})
		})

		When("another CFOrg exists with the same name(case insensitive) in the same namespace", func() {
			BeforeEach(func() {
				org2 := MakeCFOrg(org2Guid, rootNamespace, strings.ToUpper(org1Name))
				Expect(
					k8sClient.Create(ctx, org2),
				).To(Succeed())
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring(fmt.Sprintf("Organization '%s' already exists.", org1Name))))
			})
		})
	})

	Describe("Update", func() {
		var updateErr error

		BeforeEach(func() {
			org1 = MakeCFOrg(org1Guid, rootNamespace, org1Name)
			Expect(k8sClient.Create(ctx, org1)).To(Succeed())
		})

		JustBeforeEach(func() {
			updateErr = k8sClient.Update(context.Background(), org1)
		})

		When("changing the name", func() {
			var newName string

			BeforeEach(func() {
				newName = uuid.NewString()
				org1.Spec.DisplayName = newName
			})

			It("should succeed", func() {
				Expect(updateErr).NotTo(HaveOccurred())
				org1Actual := v1alpha1.CFOrg{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(org1), &org1Actual)).To(Succeed())
				Expect(org1Actual.Spec.DisplayName).To(Equal(newName))
			})

			When("reusing an old name", func() {
				It("allows creating another org with the old name", func() {
					Expect(updateErr).NotTo(HaveOccurred())

					reuseOldNameOrg := MakeCFOrg(uuid.NewString(), rootNamespace, org1Name)
					Expect(k8sClient.Create(ctx, reuseOldNameOrg)).To(Succeed())
				})
			})
		})

		When("not changing the name", func() {
			It("should succeed", func() {
				Expect(updateErr).NotTo(HaveOccurred())
				org1Actual := v1alpha1.CFOrg{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(org1), &org1Actual)).To(Succeed())
			})
		})

		When("modifying spec.DisplayName to match another CFOrg spec.DisplayName", func() {
			BeforeEach(func() {
				org2 := MakeCFOrg(org2Guid, rootNamespace, org2Name)
				org1.Spec.DisplayName = org2Name
				Expect(k8sClient.Create(ctx, org2)).To(Succeed())
			})

			It("should fail", func() {
				Expect(updateErr).To(MatchError(ContainSubstring(fmt.Sprintf("Organization '%s' already exists.", org2Name))))
			})
		})
	})

	Describe("Delete", func() {
		var deleteErr error

		BeforeEach(func() {
			org1 = MakeCFOrg(org1Guid, rootNamespace, org1Name)
			Expect(k8sClient.Create(ctx, org1)).To(Succeed())
		})

		JustBeforeEach(func() {
			deleteErr = k8sClient.Delete(ctx, org1)
		})

		It("succeeds", func() {
			Expect(deleteErr).NotTo(HaveOccurred())
		})
	})
})
