package integration_test

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks/workloads/integration/helpers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("SubnamespaceanchorValidation", func() {
	var ctx context.Context
	var namespace *v1.Namespace
	var otherNamespace *v1.Namespace

	BeforeEach(func() {
		ctx = context.Background()

		namespace = &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: uuid.NewString()},
		}
		otherNamespace = &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: uuid.NewString()},
		}

		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		Expect(k8sClient.Create(ctx, otherNamespace)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
		Expect(k8sClient.Delete(ctx, otherNamespace)).To(Succeed())
	})

	createOrg := func(rootNamespace, name string) (*hnsv1alpha2.SubnamespaceAnchor, error) {
		anchor := helpers.MakeOrg(rootNamespace, name)
		return anchor, k8sClient.Create(ctx, anchor)
	}

	createSpace := func(orgNamespace, name string) (*hnsv1alpha2.SubnamespaceAnchor, error) {
		anchor := helpers.MakeSpace(orgNamespace, name)
		return anchor, k8sClient.Create(ctx, anchor)
	}

	Describe("creating an org", func() {
		BeforeEach(func() {
			_, err := createOrg(otherNamespace.Name, "my-org")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the org name is unique in its namespace", func() {
			It("succeeds", func() {
				_, err := createOrg(namespace.Name, "my-org")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("the name already exists in its namespace", func() {
			It("fails", func() {
				_, err := createOrg(otherNamespace.Name, "my-org")
				Expect(err).To(MatchError(ContainSubstring("Org with same name exists")))
			})
		})
	})

	Describe("creating a space", func() {
		BeforeEach(func() {
			_, err := createSpace(otherNamespace.Name, "my-space")
			Expect(err).To(Succeed())
		})

		When("the space name is unique in the org namespace", func() {
			It("succeeds", func() {
				_, err := createSpace(namespace.Name, "my-space")
				Expect(err).To(Succeed())
			})
		})

		When("the name already exists in the org namespace", func() {
			It("fails", func() {
				_, err := createSpace(otherNamespace.Name, "my-space")
				Expect(err).To(MatchError(ContainSubstring("Space with same name exists")))
			})
		})
	})

	Describe("updating an org", func() {
		var (
			org        *hnsv1alpha2.SubnamespaceAnchor
			updatedOrg *hnsv1alpha2.SubnamespaceAnchor
		)

		BeforeEach(func() {
			var err error
			org, err = createOrg(namespace.Name, "my-org")
			Expect(err).NotTo(HaveOccurred())
			updatedOrg = org.DeepCopy()
		})

		When("not changing the org label", func() {
			BeforeEach(func() {
				updatedOrg.Labels["foo"] = "bar"
			})

			It("succeeds", func() {
				Expect(k8sClient.Patch(ctx, updatedOrg, client.MergeFrom(org))).To(Succeed())

				var retrievedOrg hnsv1alpha2.SubnamespaceAnchor

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(updatedOrg), &retrievedOrg)).To(Succeed())
				Expect(retrievedOrg.Labels).To(HaveKeyWithValue("foo", "bar"))
			})
		})

		When("the org name is changed to another which is unique in the root CF namespace", func() {
			BeforeEach(func() {
				updatedOrg.Labels[workloads.OrgNameLabel] = "another-org"
			})

			It("succeeds", func() {
				Expect(k8sClient.Patch(ctx, updatedOrg, client.MergeFrom(org))).To(Succeed())

				var retrievedOrg hnsv1alpha2.SubnamespaceAnchor

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(updatedOrg), &retrievedOrg)).To(Succeed())
				Expect(retrievedOrg.Labels).To(HaveKeyWithValue(workloads.OrgNameLabel, "another-org"))
			})
		})

		When("the new org name already exists in the root CF namespace", func() {
			BeforeEach(func() {
				_, err := createOrg(namespace.Name, "another-org")
				Expect(err).NotTo(HaveOccurred())

				updatedOrg.Labels[workloads.OrgNameLabel] = "another-org"
			})

			It("fails", func() {
				Expect(k8sClient.Patch(ctx, updatedOrg, client.MergeFrom(org))).To(MatchError(ContainSubstring("Org with same name exists")))
			})
		})
	})

	Describe("updating a space", func() {
		var (
			space        *hnsv1alpha2.SubnamespaceAnchor
			updatedSpace *hnsv1alpha2.SubnamespaceAnchor
		)

		BeforeEach(func() {
			var err error
			space, err = createSpace(namespace.Name, "my-space")
			Expect(err).NotTo(HaveOccurred())
			updatedSpace = space.DeepCopy()
		})

		When("not changing the space label", func() {
			BeforeEach(func() {
				updatedSpace.Labels["foo"] = "bar"
			})

			It("succeeds", func() {
				Expect(k8sClient.Patch(ctx, updatedSpace, client.MergeFrom(space))).To(Succeed())

				var retrievedSpace hnsv1alpha2.SubnamespaceAnchor

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(updatedSpace), &retrievedSpace)).To(Succeed())
				Expect(retrievedSpace.Labels).To(HaveKeyWithValue("foo", "bar"))
			})
		})

		When("the space name is changed to another which is unique in the root CF namespace", func() {
			BeforeEach(func() {
				updatedSpace.Labels[workloads.SpaceNameLabel] = "another-space"
			})

			It("succeeds", func() {
				Expect(k8sClient.Patch(ctx, updatedSpace, client.MergeFrom(space))).To(Succeed())

				var retrievedSpace hnsv1alpha2.SubnamespaceAnchor

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(updatedSpace), &retrievedSpace)).To(Succeed())
				Expect(retrievedSpace.Labels).To(HaveKeyWithValue(workloads.SpaceNameLabel, "another-space"))
			})
		})

		When("the new space name already exists in the root CF namespace", func() {
			BeforeEach(func() {
				_, err := createSpace(namespace.Name, "another-space")
				Expect(err).NotTo(HaveOccurred())

				updatedSpace.Labels[workloads.SpaceNameLabel] = "another-space"
			})

			It("fails", func() {
				Expect(k8sClient.Patch(ctx, updatedSpace, client.MergeFrom(space))).To(MatchError(ContainSubstring("Space with same name exists")))
			})
		})
	})

	Describe("deleting an org", func() {
		var org *hnsv1alpha2.SubnamespaceAnchor

		BeforeEach(func() {
			var err error
			org, err = createOrg(namespace.Name, "my-org")
			Expect(err).NotTo(HaveOccurred())
		})

		It("can delete the org", func() {
			Expect(k8sClient.Delete(ctx, org)).To(Succeed())
		})
	})

	Describe("deleting a space", func() {
		var space *hnsv1alpha2.SubnamespaceAnchor

		BeforeEach(func() {
			var err error
			space, err = createSpace(namespace.Name, "my-space")
			Expect(err).NotTo(HaveOccurred())
		})

		It("can delete the space", func() {
			Expect(k8sClient.Delete(ctx, space)).To(Succeed())
		})
	})
})
