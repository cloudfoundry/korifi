package integration_test

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/integration/helpers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFSpaceValidatingWebhook", func() {
	var (
		ctx               context.Context
		cfSpace, cfSpace2 *korifiv1alpha1.CFSpace
		orgNamespace      string
	)

	BeforeEach(func() {
		ctx = context.Background()

		orgNamespace = "test-org-" + uuid.NewString()
		Expect(k8sClient.Create(ctx, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: orgNamespace,
			},
		})).To(Succeed())

		Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgNamespace,
				Namespace: rootNamespace,
			},
			Spec: korifiv1alpha1.CFOrgSpec{
				DisplayName: orgNamespace,
			},
		})).To(Succeed())
	})

	eventuallyCreateSpace := func(spaceToCreate *korifiv1alpha1.CFSpace) types.AsyncAssertion {
		return Eventually(func() error {
			return k8sClient.Create(ctx, spaceToCreate)
		})
	}

	Describe("creating a space", func() {
		var spaceCreation types.AsyncAssertion

		BeforeEach(func() {
			cfSpace = MakeCFSpace(orgNamespace, "my-space")
		})

		JustBeforeEach(func() {
			spaceCreation = eventuallyCreateSpace(cfSpace)
		})

		It("succeeds", func() {
			spaceCreation.Should(Succeed())
		})

		When("a corresponding CFOrg does not exit", func() {
			BeforeEach(func() {
				cfSpace.Namespace = "not-an-org"
				Expect(k8sClient.Create(ctx, &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "not-an-org",
					},
				})).To(Succeed())
			})

			It("fails", func() {
				spaceCreation.Should(MatchError(ContainSubstring("Organization 'not-an-org' does not exist for Space 'my-space'")))
			})
		})

		When("the name already exists in the org namespace", func() {
			BeforeEach(func() {
				cfSpace2 = MakeCFSpace(orgNamespace, "my-space")
				eventuallyCreateSpace(cfSpace2).Should(Succeed())
			})

			It("fails", func() {
				spaceCreation.Should(MatchError(ContainSubstring("Name must be unique per organization")))
			})
		})

		When("another CFSpace exists with the same name(case insensitive) in the same namespace", func() {
			BeforeEach(func() {
				cfSpace2 = MakeCFSpace(orgNamespace, "My-Space")
				eventuallyCreateSpace(cfSpace2).Should(Succeed())
			})

			It("should fail", func() {
				spaceCreation.Should(MatchError(ContainSubstring(fmt.Sprintf("Space '%s' already exists.", cfSpace.Spec.DisplayName))))
			})
		})
	})

	Describe("updating a space", func() {
		var updatedCFSpace *korifiv1alpha1.CFSpace

		BeforeEach(func() {
			cfSpace = MakeCFSpace(orgNamespace, "my-space")
			eventuallyCreateSpace(cfSpace).Should(Succeed())
			updatedCFSpace = cfSpace.DeepCopy()
		})

		When("the space name is changed to another which is unique in the root CF namespace", func() {
			BeforeEach(func() {
				updatedCFSpace.Spec.DisplayName = "another-space"
			})

			It("succeeds", func() {
				Expect(k8sClient.Patch(ctx, updatedCFSpace, client.MergeFrom(cfSpace))).To(Succeed())
			})
		})

		When("the new space name already exists in the org namespace", func() {
			BeforeEach(func() {
				cfSpace2 = MakeCFSpace(orgNamespace, "another-space")
				eventuallyCreateSpace(cfSpace2).Should(Succeed())
				updatedCFSpace.Spec.DisplayName = "another-space"
			})

			It("fails", func() {
				Expect(k8sClient.Patch(ctx, updatedCFSpace, client.MergeFrom(cfSpace))).To(MatchError(ContainSubstring("Name must be unique per organization")))
			})
		})
	})

	Describe("deleting a space", func() {
		BeforeEach(func() {
			cfSpace = MakeCFSpace(orgNamespace, "my-space")
			eventuallyCreateSpace(cfSpace).Should(Succeed())
		})

		It("can delete the space", func() {
			Expect(k8sClient.Delete(ctx, cfSpace)).To(Succeed())
		})
	})
})
