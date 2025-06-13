package repositories_test

import (
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("StackRepository", func() {
	var (
		builderInfo *korifiv1alpha1.BuilderInfo
		stackRepo   *repositories.StackRepository
	)

	BeforeEach(func() {
		builderName := uuid.NewString()
		stackRepo = repositories.NewStackRepository(rootNSKlient, builderName, rootNamespace)

		builderInfo = &korifiv1alpha1.BuilderInfo{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      builderName,
			},
			Spec:   korifiv1alpha1.BuilderInfoSpec{},
			Status: korifiv1alpha1.BuilderInfoStatus{},
		}
		Expect(k8sClient.Create(ctx, builderInfo)).To(Succeed())

		Expect(k8s.Patch(ctx, k8sClient, builderInfo, func() {
			builderInfo.Status = korifiv1alpha1.BuilderInfoStatus{
				Buildpacks: []korifiv1alpha1.BuilderInfoStatusBuildpack{},
				Stacks: []korifiv1alpha1.BuilderInfoStatusStack{{
					Name:              "my-stack",
					Description:       "my stack",
					CreationTimestamp: metav1.NewTime(time.UnixMilli(2000).UTC()),
					UpdatedTimestamp:  metav1.NewTime(time.UnixMilli(3000).UTC()),
				}},
				Conditions: []metav1.Condition{{
					Type:               korifiv1alpha1.StatusConditionReady,
					Status:             metav1.ConditionTrue,
					Reason:             "Ready",
					LastTransitionTime: metav1.NewTime(time.Now()),
				}},
			}
		})).To(Succeed())
	})

	Describe("ListStacks", func() {
		var (
			stacks  []repositories.StackRecord
			listErr error
		)

		JustBeforeEach(func() {
			stacks, listErr = stackRepo.ListStacks(ctx, authInfo)
		})

		It("returns the stacks from the builder info", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(stacks).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
				"Name":        Equal("my-stack"),
				"Description": Equal("my stack"),
				"CreatedAt":   BeTemporally("~", time.UnixMilli(2000).UTC()),
				"UpdatedAt":   PointTo(BeTemporally("~", time.UnixMilli(3000).UTC())),
			})))
		})

		When("the builderInfo is not ready", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, builderInfo, func() {
					meta.SetStatusCondition(&builderInfo.Status.Conditions, metav1.Condition{
						Type:               korifiv1alpha1.StatusConditionReady,
						Status:             metav1.ConditionFalse,
						Reason:             "NotReady",
						LastTransitionTime: metav1.NewTime(time.Now()),
					})
				})).To(Succeed())
			})

			It("returns a not ready error", func() {
				Expect(listErr).To(BeAssignableToTypeOf(apierrors.ResourceNotReadyError{}))
			})
		})

		When("the builderInfo does not exist", func() {
			BeforeEach(func() {
				stackRepo = repositories.NewStackRepository(rootNSKlient, uuid.NewString(), rootNamespace)
			})

			It("returns a not found error", func() {
				Expect(listErr).To(BeAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})
})
