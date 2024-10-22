package repositories_test

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	. "code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomega_types "github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BuildpackRepository", func() {
	var (
		buildpackRepo *BuildpackRepository
		sorter        *fake.BuildpackSorter
	)

	BeforeEach(func() {
		sorter = new(fake.BuildpackSorter)
		sorter.SortStub = func(records []BuildpackRecord, _ string) []BuildpackRecord {
			return records
		}

		buildpackRepo = NewBuildpackRepository(builderName, userClientFactory, rootNamespace, sorter)
	})

	Describe("ListBuildpacks", func() {
		var message ListBuildpacksMessage

		BeforeEach(func() {
			message = ListBuildpacksMessage{OrderBy: "foo"}
		})

		When("a controller with the configured BuilderName exists", func() {
			var buildpacks []BuildpackRecord

			BeforeEach(func() {
				createBuilderInfoWithCleanup(ctx, builderName, "io.buildpacks.stacks.bionic", []buildpackInfo{
					{name: "paketo-buildpacks/buildpack-1-1", version: "1.1"},
					{name: "paketo-buildpacks/buildpack-2-1", version: "2.1"},
					{name: "paketo-buildpacks/buildpack-3-1", version: "3.1"},
				})

				var err error
				buildpacks, err = buildpackRepo.ListBuildpacks(context.Background(), authInfo, message)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns all buildpacks", func() {
				Expect(buildpacks).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name":     Equal("paketo-buildpacks/buildpack-1-1"),
						"Position": Equal(1),
						"Stack":    Equal("io.buildpacks.stacks.bionic"),
						"Version":  Equal("1.1"),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":     Equal("paketo-buildpacks/buildpack-2-1"),
						"Position": Equal(2),
						"Stack":    Equal("io.buildpacks.stacks.bionic"),
						"Version":  Equal("2.1"),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":     Equal("paketo-buildpacks/buildpack-3-1"),
						"Position": Equal(3),
						"Stack":    Equal("io.buildpacks.stacks.bionic"),
						"Version":  Equal("3.1"),
					}),
				))
			})

			It("sorts the buildpacks", func() {
				Expect(sorter.SortCallCount()).To(Equal(1))
				sortedBuildpacks, field := sorter.SortArgsForCall(0)
				Expect(field).To(Equal("foo"))
				Expect(sortedBuildpacks).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("paketo-buildpacks/buildpack-1-1"),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("paketo-buildpacks/buildpack-2-1"),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal("paketo-buildpacks/buildpack-3-1"),
					}),
				))
			})
		})

		When("no build reconcilers exist", func() {
			It("errors", func() {
				_, err := buildpackRepo.ListBuildpacks(context.Background(), authInfo, message)
				Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("BuilderInfo %q not found in namespace %q", builderName, rootNamespace))))
			})
		})

		When("the build reconciler with the configured BuilderName is not found", func() {
			BeforeEach(func() {
				createBuilderInfoWithCleanup(ctx, "ignored-name1", "io.buildpacks.stacks.bionic", []buildpackInfo{
					{name: "paketo-buildpacks/buildpack-1-1", version: "1.1"},
				})
				createBuilderInfoWithCleanup(ctx, "ignored-name2", "io.buildpacks.stacks.walrus", []buildpackInfo{
					{name: "paketo-buildpacks/buildpack-2-1", version: "2.1"},
				})
			})

			It("errors", func() {
				_, err := buildpackRepo.ListBuildpacks(context.Background(), authInfo, message)
				Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("BuilderInfo %q not found in namespace %q", builderName, rootNamespace))))
			})
		})

		When("the BuilderInfo resource with the configured BuilderName is not ready", func() {
			var builderInfo *korifiv1alpha1.BuilderInfo

			BeforeEach(func() {
				builderInfo = createBuilderInfoWithCleanup(ctx, builderName, "io.buildpacks.stacks.bionic", []buildpackInfo{
					{name: "paketo-buildpacks/buildpack-1-1", version: "1.1"},
					{name: "paketo-buildpacks/buildpack-2-1", version: "2.1"},
					{name: "paketo-buildpacks/buildpack-3-1", version: "3.1"},
				})
			})

			When("there is a ready condition with a message", func() {
				BeforeEach(func() {
					meta.SetStatusCondition(&builderInfo.Status.Conditions, metav1.Condition{
						Type:    korifiv1alpha1.StatusConditionReady,
						Status:  metav1.ConditionFalse,
						Reason:  "testing",
						Message: "this is a test",
					})
					Expect(k8sClient.Status().Update(ctx, builderInfo)).To(Succeed())
				})

				It("returns an error with the ready condition message", func() {
					_, err := buildpackRepo.ListBuildpacks(context.Background(), authInfo, message)
					Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("BuilderInfo %q not ready: this is a test", builderName))))
				})
			})

			When("there is a ready condition with an empty message", func() {
				BeforeEach(func() {
					meta.SetStatusCondition(&builderInfo.Status.Conditions, metav1.Condition{
						Type:    korifiv1alpha1.StatusConditionReady,
						Status:  metav1.ConditionFalse,
						Reason:  "testing",
						Message: "",
					})
					Expect(k8sClient.Status().Update(ctx, builderInfo)).To(Succeed())
				})

				It("returns an error with a generic message", func() {
					_, err := buildpackRepo.ListBuildpacks(context.Background(), authInfo, message)
					Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("BuilderInfo %q not ready: resource not reconciled", builderName))))
				})
			})
		})
	})
})

type buildpackInfo struct {
	name    string
	version string
}

func createBuilderInfoWithCleanup(ctx context.Context, name, stack string, buildpacks []buildpackInfo) *korifiv1alpha1.BuilderInfo {
	builderInfo := &korifiv1alpha1.BuilderInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rootNamespace,
		},
	}
	Expect(k8sClient.Create(ctx, builderInfo)).To(Succeed())
	DeferCleanup(func() {
		Expect(k8sClient.Delete(ctx, builderInfo)).To(Succeed())
	})

	creationTimestamp := metav1.Time{Time: time.Now().Add(-24 * time.Hour)}
	updatedTimestamp := metav1.Time{Time: time.Now().Add(-30 * time.Second)}

	builderInfo.Status.Stacks = []korifiv1alpha1.BuilderInfoStatusStack{
		{
			Name:              stack,
			CreationTimestamp: metav1.Time{Time: time.Now()},
			UpdatedTimestamp:  metav1.Time{Time: time.Now()},
		},
	}
	for _, b := range buildpacks {
		builderInfo.Status.Buildpacks = append(builderInfo.Status.Buildpacks, korifiv1alpha1.BuilderInfoStatusBuildpack{
			Name:              b.name,
			Version:           b.version,
			Stack:             stack,
			CreationTimestamp: creationTimestamp,
			UpdatedTimestamp:  updatedTimestamp,
		})
	}

	meta.SetStatusCondition(&builderInfo.Status.Conditions, metav1.Condition{
		Type:   korifiv1alpha1.StatusConditionReady,
		Status: metav1.ConditionTrue,
		Reason: "testing",
	})
	Expect(k8sClient.Status().Update(ctx, builderInfo)).To(Succeed())
	return builderInfo
}

var _ = DescribeTable("BuildpackSorter",
	func(p1, p2 BuildpackRecord, field string, match gomega_types.GomegaMatcher) {
		Expect(BuildpackComparator(field)(p1, p2)).To(match)
	},
	Entry("created_at",
		BuildpackRecord{CreatedAt: time.UnixMilli(1)},
		BuildpackRecord{CreatedAt: time.UnixMilli(2)},
		"created_at",
		BeNumerically("<", 0),
	),
	Entry("-created_at",
		BuildpackRecord{CreatedAt: time.UnixMilli(1)},
		BuildpackRecord{CreatedAt: time.UnixMilli(2)},
		"-created_at",
		BeNumerically(">", 0),
	),
	Entry("updated_at",
		BuildpackRecord{UpdatedAt: tools.PtrTo(time.UnixMilli(1))},
		BuildpackRecord{UpdatedAt: tools.PtrTo(time.UnixMilli(2))},
		"updated_at",
		BeNumerically("<", 0),
	),
	Entry("-updated_at",
		BuildpackRecord{UpdatedAt: tools.PtrTo(time.UnixMilli(1))},
		BuildpackRecord{UpdatedAt: tools.PtrTo(time.UnixMilli(2))},
		"-updated_at",
		BeNumerically(">", 0),
	),
	Entry("position",
		BuildpackRecord{Position: 1},
		BuildpackRecord{Position: 2},
		"position",
		BeNumerically("<", 0),
	),
	Entry("-position",
		BuildpackRecord{Position: 1},
		BuildpackRecord{Position: 2},
		"-position",
		BeNumerically(">", 0),
	),
)
