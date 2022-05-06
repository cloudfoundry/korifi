package integration_test

import (
	"context"
	"fmt"

	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/integration/helpers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFSpaceValidatingWebhook", func() {
	var (
		ctx               context.Context
		cfSpace, cfSpace2 *workloadsv1alpha1.CFSpace
		orgNamespace      string
	)

	BeforeEach(func() {
		ctx = context.Background()

		orgNamespace = "test-org" + uuid.NewString()
		Expect(k8sClient.Create(ctx, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: orgNamespace,
			},
		})).To(Succeed())
	})

	Describe("creating a space", func() {
		var err error

		BeforeEach(func() {
			cfSpace = MakeCFSpace(orgNamespace, "my-space")
		})

		JustBeforeEach(func() {
			err = k8sClient.Create(ctx, cfSpace)
		})

		It("succeeds", func() {
			Expect(err).To(Succeed())
		})

		When("the name already exists in the org namespace", func() {
			BeforeEach(func() {
				cfSpace2 = MakeCFSpace(orgNamespace, "my-space")
				Expect(k8sClient.Create(ctx, cfSpace2)).To(Succeed())
			})

			It("fails", func() {
				Expect(err).To(MatchError(ContainSubstring("Name must be unique per organization")))
			})
		})

		When("another CFSpace exists with the same name(case insensitive) in the same namespace", func() {
			BeforeEach(func() {
				cfSpace2 = MakeCFSpace(orgNamespace, "My-Space")
				Expect(k8sClient.Create(ctx, cfSpace2)).To(Succeed())
			})

			It("should fail", func() {
				Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("Space '%s' already exists.", cfSpace.Spec.DisplayName))))
			})
		})
	})

	Describe("updating a space", func() {
		var (
			updatedCFSpace *workloadsv1alpha1.CFSpace
			err            error
		)

		BeforeEach(func() {
			cfSpace = MakeCFSpace(orgNamespace, "my-space")
			err = k8sClient.Create(ctx, cfSpace)
			Expect(err).NotTo(HaveOccurred())
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
				Expect(k8sClient.Create(ctx, cfSpace2)).To(Succeed())
				updatedCFSpace.Spec.DisplayName = "another-space"
			})

			It("fails", func() {
				Expect(k8sClient.Patch(ctx, updatedCFSpace, client.MergeFrom(cfSpace))).To(MatchError(ContainSubstring("Name must be unique per organization")))
			})
		})
	})

	Describe("deleting a space", func() {
		var (
			err error
		)
		BeforeEach(func() {
			cfSpace = MakeCFSpace(orgNamespace, "my-space")
			err = k8sClient.Create(ctx, cfSpace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("can delete the space", func() {
			Expect(k8sClient.Delete(ctx, cfSpace)).To(Succeed())
		})
	})
})
