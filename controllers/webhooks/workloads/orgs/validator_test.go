package orgs_test

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFOrgValidatingWebhook", func() {
	var (
		org       *korifiv1alpha1.CFOrg
		createErr error
	)

	BeforeEach(func() {
		org = &korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: rootNamespace,
			},
			Spec: korifiv1alpha1.CFOrgSpec{
				DisplayName: "org-" + uuid.NewString(),
			},
		}
	})

	JustBeforeEach(func() {
		createErr = adminClient.Create(ctx, org)
	})

	Describe("Create", func() {
		It("should succeed", func() {
			Expect(createErr).NotTo(HaveOccurred())
		})

		When("CFOrg is requested outside of root namespace", func() {
			BeforeEach(func() {
				org.Namespace = "default"
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring(fmt.Sprintf("Organization '%s' must be placed in the root 'cf' namespace", org.Spec.DisplayName))))
			})
		})

		When("the CFOrg name would not be a valid label value (>63 chars)", func() {
			BeforeEach(func() {
				org.Name = strings.Repeat("a", 64)
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring("org name cannot be longer than 63 chars")))
			})
		})

		When("another CFOrg exists with a different name", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &korifiv1alpha1.CFOrg{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: rootNamespace,
					},
					Spec: korifiv1alpha1.CFOrgSpec{
						DisplayName: uuid.NewString(),
					},
				})).To(Succeed())
			})

			It("should succeed", func() {
				Expect(createErr).NotTo(HaveOccurred())
			})
		})

		When("another CFOrg exists with the same name", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &korifiv1alpha1.CFOrg{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: rootNamespace,
					},
					Spec: korifiv1alpha1.CFOrgSpec{
						DisplayName: org.Spec.DisplayName,
					},
				})).To(Succeed())
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring("Organization '%s' already exists.", org.Spec.DisplayName)))
			})
		})

		When("another CFOrg exists with the same name(case insensitive)", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &korifiv1alpha1.CFOrg{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: rootNamespace,
					},
					Spec: korifiv1alpha1.CFOrgSpec{
						DisplayName: strings.ToUpper(org.Spec.DisplayName),
					},
				})).To(Succeed())
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring(fmt.Sprintf("Organization '%s' already exists.", org.Spec.DisplayName))))
			})
		})
	})

	Describe("Update", func() {
		var updateErr error

		Describe("changing the name", func() {
			var newName string

			BeforeEach(func() {
				newName = uuid.NewString()
			})

			JustBeforeEach(func() {
				updateErr = k8s.Patch(ctx, adminClient, org, func() {
					org.Spec.DisplayName = newName
				})
			})

			It("should succeed", func() {
				Expect(updateErr).NotTo(HaveOccurred())
				Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(org), org)).To(Succeed())
				Expect(org.Spec.DisplayName).To(Equal(newName))
			})

			When("reusing an old name", func() {
				var oldName string

				BeforeEach(func() {
					oldName = org.Spec.DisplayName
				})

				It("allows creating another org with the old name", func() {
					Expect(updateErr).NotTo(HaveOccurred())

					Expect(adminClient.Create(ctx, &korifiv1alpha1.CFOrg{
						ObjectMeta: metav1.ObjectMeta{
							Name:      uuid.NewString(),
							Namespace: rootNamespace,
						},
						Spec: korifiv1alpha1.CFOrgSpec{
							DisplayName: oldName,
						},
					})).To(Succeed())
				})
			})

			When("an org with the same name already exists", func() {
				BeforeEach(func() {
					Expect(adminClient.Create(ctx, &korifiv1alpha1.CFOrg{
						ObjectMeta: metav1.ObjectMeta{
							Name:      uuid.NewString(),
							Namespace: rootNamespace,
						},
						Spec: korifiv1alpha1.CFOrgSpec{
							DisplayName: newName,
						},
					})).To(Succeed())
				})

				It("should fail", func() {
					Expect(updateErr).To(MatchError(ContainSubstring(fmt.Sprintf("Organization '%s' already exists.", newName))))
				})
			})
		})

		When("not changing the name", func() {
			JustBeforeEach(func() {
				updateErr = k8s.Patch(ctx, adminClient, org, func() {
					org.Labels = map[string]string{"foo": "bar"}
				})
			})
			It("should succeed", func() {
				Expect(updateErr).NotTo(HaveOccurred())
				Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(org), org)).To(Succeed())
				Expect(org.Labels).To(HaveKeyWithValue("foo", "bar"))
			})
		})
	})

	Describe("Delete", func() {
		var deleteErr error

		JustBeforeEach(func() {
			deleteErr = adminNonSyncClient.Delete(ctx, org)
		})

		It("succeeds", func() {
			Expect(deleteErr).NotTo(HaveOccurred())
		})
	})
})
