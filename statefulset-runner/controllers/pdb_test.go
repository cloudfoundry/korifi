package controllers_test

import (
	"context"

	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/tools"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("PDB", func() {
	var (
		creator       *controllers.PDBUpdater
		stSet         *appsv1.StatefulSet
		ctx           context.Context
		namespaceName string
	)

	BeforeEach(func() {
		ctx = context.Background()
		creator = controllers.NewPDBUpdater(k8sClient)

		namespaceName = prefixedGUID("ns")
		createNamespace(ctx, k8sClient, namespaceName)

		stSet = &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "name",
				Namespace: namespaceName,
				UID:       "uid",
				Labels: map[string]string{
					controllers.LabelGUID:    "label-guid",
					controllers.LabelVersion: "label-version",
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: tools.PtrTo(int32(2)),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						controllers.LabelGUID:    "label-guid",
						controllers.LabelVersion: "label-version",
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(creator.Update(ctx, stSet)).To(Succeed())
	})

	It("creates a pod disruption budget", func() {
		Eventually(func(g Gomega) {
			pdb := &policyv1.PodDisruptionBudget{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(stSet), pdb)).To(Succeed())
			g.Expect(pdb.Spec.MinAvailable).To(PointTo(Equal(intstr.FromString("50%"))))
			g.Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue(controllers.LabelGUID, stSet.Labels[controllers.LabelGUID]))
			g.Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue(controllers.LabelVersion, stSet.Labels[controllers.LabelVersion]))
			g.Expect(pdb.OwnerReferences).To(HaveLen(1))
			g.Expect(pdb.OwnerReferences[0].Name).To(Equal(stSet.Name))
			g.Expect(pdb.OwnerReferences[0].UID).To(Equal(stSet.UID))
		}).Should(Succeed())
	})

	When("the statefulset has been scaled down to less than 2 instances", func() {
		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(stSet), &policyv1.PodDisruptionBudget{}))
			}).Should(Succeed())

			stSet.Spec.Replicas = tools.PtrTo(int32(1))
			Expect(creator.Update(ctx, stSet)).To(Succeed())
		})

		It("deletes the PDB", func() {
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(stSet), &policyv1.PodDisruptionBudget{})
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})
	})
})
