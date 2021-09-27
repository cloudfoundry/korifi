package repositories_test

import (
	"context"
	"testing"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = SuiteDescribe("Org Repo", func(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	var (
		orgRepo       *repositories.OrgRepo
		ctx           context.Context
		rootNamespace string

		org1Ns, org2Ns, org3Ns *hnsv1alpha2.SubnamespaceAnchor
	)

	it.Before(func() {
		rootNamespace = generateGUID()
		g.Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())

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

		g.Expect(k8sClient.Create(ctx, org1Ns)).To(Succeed())
		g.Expect(k8sClient.Create(ctx, org2Ns)).To(Succeed())
		g.Expect(k8sClient.Create(ctx, org3Ns)).To(Succeed())
	})

	it("returns the 3 orgs", func() {
		orgs, err := orgRepo.FetchOrgs(ctx, nil)
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(orgs).To(ConsistOf(
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

	when("we filter for org1 and org3", func() {
		it("returns just those", func() {
			orgs, err := orgRepo.FetchOrgs(ctx, []string{"org1", "org3"})
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(orgs).To(ConsistOf(
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
