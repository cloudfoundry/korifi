package stset_test

import (
	"context"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/pdb"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Update", func() {
	var (
		allowRunImageAsRoot bool
		desirer             *stset.Desirer
		updater             *stset.Updater
		lrp                 *eiriniv1.LRP
	)

	BeforeEach(func() {
		allowRunImageAsRoot = false
		lrp = createLRP(fixture.Namespace, "odin")
	})

	JustBeforeEach(func() {
		desirer = createDesirer(fixture.Namespace, allowRunImageAsRoot)
		updater = createUpdater(fixture.Namespace, allowRunImageAsRoot)
	})

	Describe("scaling", func() {
		var (
			instancesBefore int
			instancesAfter  int
			statefulset     *appsv1.StatefulSet
		)

		BeforeEach(func() {
			instancesBefore = 1
			instancesAfter = 2
		})

		JustBeforeEach(func() {
			lrp.Spec.Instances = instancesBefore
			Expect(desirer.Desire(ctx, lrp)).To(Succeed())

			lrp.Spec.Instances = instancesAfter
			statefulset = getStatefulSetForLRP(lrp)
			Expect(updater.Update(ctx, lrp, statefulset)).To(Succeed())
			statefulset = getStatefulSetForLRP(lrp)
		})

		It("updates instance count", func() {
			Expect(statefulset.Spec.Replicas).To(PointTo(Equal(int32(2))))
		})

		When("scaling up from 1 to 2 instances", func() {
			It("should create a pod disruption budget for the lrp", func() {
				pdb, err := podDisruptionBudgets().Get(context.Background(), statefulset.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pdb).NotTo(BeNil())
			})
		})

		When("scaling up from 2 to 3 instances", func() {
			BeforeEach(func() {
				instancesBefore = 2
				instancesAfter = 3
			})

			It("should keep the existing pod disruption budget for the lrp", func() {
				pdb, err := podDisruptionBudgets().Get(context.Background(), statefulset.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pdb).NotTo(BeNil())
			})
		})

		When("scaling down from 2 to 1 instances", func() {
			BeforeEach(func() {
				instancesBefore = 2
				instancesAfter = 1
			})

			It("should delete the pod disruption budget for the lrp", func() {
				_, err := podDisruptionBudgets().Get(context.Background(), statefulset.Name, metav1.GetOptions{})
				Expect(err).To(MatchError(ContainSubstring("not found")))
			})
		})

		When("scaling down from 1 to 0 instances", func() {
			BeforeEach(func() {
				instancesBefore = 1
				instancesAfter = 0
			})

			It("should keep the lrp without a pod disruption budget", func() {
				_, err := podDisruptionBudgets().Get(context.Background(), statefulset.Name, metav1.GetOptions{})
				Expect(err).To(MatchError(ContainSubstring("not found")))
			})
		})
	})

	Describe("updating image", func() {
		var (
			imageBefore string
			imageAfter  string
			statefulset *appsv1.StatefulSet
		)

		BeforeEach(func() {
			imageBefore = "eirini/dorini"
			imageAfter = "eirini/notdora"
		})

		JustBeforeEach(func() {
			lrp.Spec.Image = imageBefore
			Expect(desirer.Desire(ctx, lrp)).To(Succeed())

			lrp.Spec.Image = imageAfter
			statefulset = getStatefulSetForLRP(lrp)
			Expect(updater.Update(ctx, lrp, statefulset)).To(Succeed())
			statefulset = getStatefulSetForLRP(lrp)
		})

		It("updates the image", func() {
			Expect(statefulset.Spec.Template.Spec.Containers[0].Image).To(Equal(imageAfter))
		})
	})
})

func createUpdater(workloadsNamespace string, allowRunImageAsRoot bool) *stset.Updater {
	logger := tests.NewTestLogger("test-" + workloadsNamespace)

	pdbUpdater := pdb.NewUpdater(fixture.RuntimeClient)

	return stset.NewUpdater(logger, fixture.RuntimeClient, pdbUpdater)
}
