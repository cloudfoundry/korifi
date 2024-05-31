package spaces_test

import (
	"fmt"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFSpaceValidatingWebhook", func() {
	var (
		cfSpace      *korifiv1alpha1.CFSpace
		orgNamespace string
		createErr    error
	)

	BeforeEach(func() {
		orgNamespace = uuid.NewString()
		Expect(adminClient.Create(ctx, &korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgNamespace,
				Namespace: rootNamespace,
			},
			Spec: korifiv1alpha1.CFOrgSpec{
				DisplayName: orgNamespace,
			},
		})).To(Succeed())

		Expect(adminClient.Create(ctx, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: orgNamespace,
			},
		})).To(Succeed())

		cfSpace = &korifiv1alpha1.CFSpace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: orgNamespace,
			},
			Spec: korifiv1alpha1.CFSpaceSpec{
				DisplayName: "my-space",
			},
		}
	})

	JustBeforeEach(func() {
		createErr = adminClient.Create(ctx, cfSpace)
	})

	Describe("creating a space", func() {
		It("succeeds", func() {
			Expect(createErr).To(Succeed())
		})

		When("a corresponding CFOrg does not exist", func() {
			BeforeEach(func() {
				cfSpace.Namespace = "not-an-org"
				Expect(adminClient.Create(ctx, &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "not-an-org",
					},
				})).To(Succeed())
			})

			It("fails", func() {
				Expect(createErr).To(MatchError(ContainSubstring("Organization 'not-an-org' does not exist for Space 'my-space'")))
			})
		})

		When("the CFSpace name is not a valid label value (>63 chars)", func() {
			BeforeEach(func() {
				cfSpace.Name = strings.Repeat("a", 64)
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring("space name cannot be longer than 63 chars")))
			})
		})

		When("the name already exists in the org namespace", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &korifiv1alpha1.CFSpace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: orgNamespace,
					},
					Spec: korifiv1alpha1.CFSpaceSpec{
						DisplayName: "my-space",
					},
				})).To(Succeed())
			})

			It("fails", func() {
				Expect(createErr).To(MatchError(ContainSubstring("Name must be unique per organization")))
			})
		})

		When("another CFSpace exists with the same name(case insensitive) in the same namespace", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &korifiv1alpha1.CFSpace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: orgNamespace,
					},
					Spec: korifiv1alpha1.CFSpaceSpec{
						DisplayName: strings.ToUpper("my-space"),
					},
				})).To(Succeed())
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring(fmt.Sprintf("Space '%s' already exists.", cfSpace.Spec.DisplayName))))
			})
		})
	})

	Describe("renaming a space", func() {
		var updateErr error

		JustBeforeEach(func() {
			updateErr = k8s.Patch(ctx, adminClient, cfSpace, func() {
				cfSpace.Spec.DisplayName = "another-space"
			})
		})

		It("succeeds", func() {
			Expect(updateErr).NotTo(HaveOccurred())
		})

		When("the new space name already exists in the org namespace", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &korifiv1alpha1.CFSpace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: orgNamespace,
					},
					Spec: korifiv1alpha1.CFSpaceSpec{
						DisplayName: "another-space",
					},
				})).To(Succeed())
			})

			It("fails", func() {
				Expect(updateErr).To(MatchError(ContainSubstring("Name must be unique per organization")))
			})
		})
	})

	Describe("deleting a space", func() {
		It("can delete the space", func() {
			Expect(adminNonSyncClient.Delete(ctx, cfSpace)).To(Succeed())
		})
	})
})
