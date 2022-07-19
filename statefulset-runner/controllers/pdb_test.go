package controllers_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/statefulset-runner/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("PDB", func() {
	var (
		creator   *controllers.PDBUpdater
		k8sClient *fake.Client
		stSet     *appsv1.StatefulSet
		ctx       context.Context
		instances int32
	)

	BeforeEach(func() {
		k8sClient = new(fake.Client)
		creator = controllers.NewPDBUpdater(k8sClient)
		instances = 2

		stSet = &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "name",
				Namespace: "namespace",
				UID:       "uid",
				Labels: map[string]string{
					controllers.LabelGUID:    "label-guid",
					controllers.LabelVersion: "label-version",
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &instances,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						controllers.LabelGUID:    "label-guid",
						controllers.LabelVersion: "label-version",
					},
				},
			},
		}

		ctx = context.Background()
	})

	Describe("Update", func() {
		var updateErr error
		JustBeforeEach(func() {
			updateErr = creator.Update(ctx, stSet)
		})

		It("succeeds", func() {
			Expect(updateErr).NotTo(HaveOccurred())
		})

		It("creates a pod disruption budget", func() {
			Expect(k8sClient.CreateCallCount()).To(Equal(1))

			_, obj, createOpts := k8sClient.CreateArgsForCall(0)

			Expect(obj).To(BeAssignableToTypeOf(&policyv1.PodDisruptionBudget{}))
			pdb := obj.(*policyv1.PodDisruptionBudget)

			Expect(pdb.Namespace).To(Equal("namespace"))
			Expect(pdb.Name).To(Equal("name"))
			Expect(pdb.Spec.MinAvailable).To(PointTo(Equal(intstr.FromString("50%"))))
			Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue(controllers.LabelGUID, stSet.Labels[controllers.LabelGUID]))
			Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue(controllers.LabelVersion, stSet.Labels[controllers.LabelVersion]))
			Expect(pdb.OwnerReferences).To(HaveLen(1))
			Expect(pdb.OwnerReferences[0].Name).To(Equal(stSet.Name))
			Expect(pdb.OwnerReferences[0].UID).To(Equal(stSet.UID))

			Expect(createOpts).To(BeEmpty())
		})

		When("pod disruption budget creation fails", func() {
			BeforeEach(func() {
				k8sClient.CreateReturns(errors.New("boom"))
			})

			It("should propagate the error", func() {
				Expect(updateErr).To(MatchError(ContainSubstring("boom")))
			})
		})

		When("the statefulset has less than 2 target instances", func() {
			var instances int32

			BeforeEach(func() {
				instances = 1
				stSet.Spec.Replicas = &instances
			})

			It("does not create but does try to delete pdb", func() {
				Expect(k8sClient.CreateCallCount()).To(BeZero())
				Expect(k8sClient.DeleteAllOfCallCount()).To(Equal(1))
			})

			When("there is no PDB already", func() {
				BeforeEach(func() {
					k8sClient.DeleteReturns(k8serrors.NewNotFound(schema.GroupResource{}, "nope"))
				})

				It("succeeds", func() {
					Expect(updateErr).NotTo(HaveOccurred())
				})
			})

			When("deleting the PDB fails", func() {
				BeforeEach(func() {
					k8sClient.DeleteAllOfReturns(errors.New("oops"))
				})

				It("returns an error", func() {
					Expect(updateErr).To(MatchError(ContainSubstring("oops")))
				})
			})
		})

		When("the pod distruption budget already exists", func() {
			BeforeEach(func() {
				k8sClient.CreateReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "boom"))
			})

			It("succeeds", func() {
				Expect(updateErr).NotTo(HaveOccurred())
			})
		})
	})
})
