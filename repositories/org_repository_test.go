package repositories_test

import (
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("OrgRepository", func() {
	Describe("ListOrgs", func() {
		var (
			orgRepo       *repositories.OrgRepo
			ctx           context.Context
			rootNamespace string

			org1Ns, org2Ns, org3Ns *hnsv1alpha2.SubnamespaceAnchor
		)

		BeforeEach(func() {
			rootNamespace = generateGUID()
			Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())

			orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient)

			ctx = context.Background()

			org1Ns = &hnsv1alpha2.SubnamespaceAnchor{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "org1",
					Namespace:    rootNamespace,
					Labels:       map[string]string{repositories.OrgNameLabel: "org1"},
				},
			}
			org2Ns = &hnsv1alpha2.SubnamespaceAnchor{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "org2",
					Namespace:    rootNamespace,
					Labels:       map[string]string{repositories.OrgNameLabel: "org2"},
				},
			}
			org3Ns = &hnsv1alpha2.SubnamespaceAnchor{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "org3",
					Namespace:    rootNamespace,
					Labels:       map[string]string{repositories.OrgNameLabel: "org3"},
				},
			}

			Expect(k8sClient.Create(ctx, org1Ns)).To(Succeed())
			Expect(k8sClient.Create(ctx, org2Ns)).To(Succeed())
			Expect(k8sClient.Create(ctx, org3Ns)).To(Succeed())
		})

		It("returns the 3 orgs", func() {
			orgs, err := orgRepo.FetchOrgs(ctx, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(orgs).To(ConsistOf(
				repositories.OrgRecord{
					Name:      "org1",
					CreatedAt: org1Ns.CreationTimestamp.Time,
					UpdatedAt: org1Ns.CreationTimestamp.Time,
					GUID:      string(org1Ns.UID),
				},
				repositories.OrgRecord{
					Name:      "org2",
					CreatedAt: org2Ns.CreationTimestamp.Time,
					UpdatedAt: org2Ns.CreationTimestamp.Time,
					GUID:      string(org2Ns.UID),
				},
				repositories.OrgRecord{
					Name:      "org3",
					CreatedAt: org3Ns.CreationTimestamp.Time,
					UpdatedAt: org3Ns.CreationTimestamp.Time,
					GUID:      string(org3Ns.UID),
				},
			))
		})

		When("we filter for org1 and org3", func() {
			It("returns just those", func() {
				orgs, err := orgRepo.FetchOrgs(ctx, []string{"org1", "org3"})
				Expect(err).NotTo(HaveOccurred())

				Expect(orgs).To(ConsistOf(
					repositories.OrgRecord{
						Name:      "org1",
						CreatedAt: org1Ns.CreationTimestamp.Time,
						UpdatedAt: org1Ns.CreationTimestamp.Time,
						GUID:      string(org1Ns.UID),
					},
					repositories.OrgRecord{
						Name:      "org3",
						CreatedAt: org3Ns.CreationTimestamp.Time,
						UpdatedAt: org3Ns.CreationTimestamp.Time,
						GUID:      string(org3Ns.UID),
					},
				))
			})
		})
	})
})
