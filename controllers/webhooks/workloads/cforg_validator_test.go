package workloads_test

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

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
		org1     *korifiv1alpha1.CFOrg
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
			org1 = makeCFOrg(org1Guid, rootNamespace, org1Name)
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
				Expect(createErr).To(MatchError(ContainSubstring(fmt.Sprintf("Organization '%s' must be placed in the root 'cf' namespace", org1Name))))
			})
		})

		When("the CFOrg name would not be a valid label value (>63 chars)", func() {
			BeforeEach(func() {
				org1.Name = strings.Repeat("a", 64)
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring("org name cannot be longer than 63 chars")))
			})
		})

		When("another CFOrg exists with a different name in the same namespace", func() {
			BeforeEach(func() {
				org2 := makeCFOrg(org2Guid, rootNamespace, org2Name)
				Expect(k8sClient.Create(ctx, org2)).To(Succeed())
			})

			It("should succeed", func() {
				Expect(createErr).NotTo(HaveOccurred())
			})
		})

		When("another CFOrg exists with the same name in the same namespace", func() {
			BeforeEach(func() {
				org2 := makeCFOrg(org2Guid, rootNamespace, org1Name)
				Expect(k8sClient.Create(ctx, org2)).To(Succeed())
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring("Organization '%s' already exists.", org1Name)))
			})
		})

		When("another CFOrg exists with the same name(case insensitive) in the same namespace", func() {
			BeforeEach(func() {
				org2 := makeCFOrg(org2Guid, rootNamespace, strings.ToUpper(org1Name))
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
		var (
			originalOrg1 *korifiv1alpha1.CFOrg
			updateErr    error
		)

		BeforeEach(func() {
			org1 = makeCFOrg(org1Guid, rootNamespace, org1Name)
			Expect(k8sClient.Create(ctx, org1)).To(Succeed())
			originalOrg1 = org1.DeepCopy()
		})

		JustBeforeEach(func() {
			updateErr = k8sClient.Patch(context.Background(), org1, client.MergeFrom(originalOrg1))
		})

		When("changing the name", func() {
			var newName string

			BeforeEach(func() {
				newName = uuid.NewString()
				org1.Spec.DisplayName = newName
			})

			It("should succeed", func() {
				Expect(updateErr).NotTo(HaveOccurred())
				org1Actual := korifiv1alpha1.CFOrg{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(org1), &org1Actual)).To(Succeed())
				Expect(org1Actual.Spec.DisplayName).To(Equal(newName))
			})

			When("reusing an old name", func() {
				It("allows creating another org with the old name", func() {
					Expect(updateErr).NotTo(HaveOccurred())

					reuseOldNameOrg := makeCFOrg(uuid.NewString(), rootNamespace, org1Name)
					Expect(k8sClient.Create(ctx, reuseOldNameOrg)).To(Succeed())
				})
			})
		})

		When("not changing the name", func() {
			It("should succeed", func() {
				Expect(updateErr).NotTo(HaveOccurred())
				org1Actual := korifiv1alpha1.CFOrg{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(org1), &org1Actual)).To(Succeed())
			})
		})

		When("modifying spec.DisplayName to match another CFOrg spec.DisplayName", func() {
			BeforeEach(func() {
				org2 := makeCFOrg(org2Guid, rootNamespace, org2Name)
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
			org1 = makeCFOrg(org1Guid, rootNamespace, org1Name)
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

func makeCFOrg(cfOrgGUID string, namespace string, name string) *korifiv1alpha1.CFOrg {
	return &korifiv1alpha1.CFOrg{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CFOrg",
			APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfOrgGUID,
			Namespace: namespace,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: name,
		},
	}
}
