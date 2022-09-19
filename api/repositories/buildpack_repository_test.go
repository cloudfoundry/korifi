package repositories_test

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	. "code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BuildpackRepository", func() {
	var buildpackRepo *BuildpackRepository

	BeforeEach(func() {
		buildpackRepo = NewBuildpackRepository(buildReconciler, userClientFactory, rootNamespace)
	})

	Describe("ListBuildpacks", func() {
		When("the configured BuildReconciler exists", func() {
			BeforeEach(func() {
				createBuilderInfoWithCleanup(ctx, buildReconciler, "io.buildpacks.stacks.bionic", []buildpackInfo{
					{name: "paketo-buildpacks/buildpack-1-1", version: "1.1"},
					{name: "paketo-buildpacks/buildpack-2-1", version: "2.1"},
					{name: "paketo-buildpacks/buildpack-3-1", version: "3.1"},
				})
			})

			It("lists the buildpacks in order", func() {
				buildpackRecords, err := buildpackRepo.ListBuildpacks(context.Background(), authInfo)
				Expect(err).NotTo(HaveOccurred())
				Expect(buildpackRecords).To(ConsistOf(
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
		})

		When("there no BuildReconcilers exist", func() {
			It("errors", func() {
				_, err := buildpackRepo.ListBuildpacks(context.Background(), authInfo)
				Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("no BuilderInfo %q resource found in %q namespace", buildReconciler, rootNamespace))))
			})
		})

		When("configured BuildReconciler is not found", func() {
			BeforeEach(func() {
				createBuilderInfoWithCleanup(ctx, "ignored-name1", "io.buildpacks.stacks.bionic", []buildpackInfo{
					{name: "paketo-buildpacks/buildpack-1-1", version: "1.1"},
				})
				createBuilderInfoWithCleanup(ctx, "ignored-name2", "io.buildpacks.stacks.walrus", []buildpackInfo{
					{name: "paketo-buildpacks/buildpack-2-1", version: "2.1"},
				})
			})

			It("errors", func() {
				_, err := buildpackRepo.ListBuildpacks(context.Background(), authInfo)
				Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("no BuilderInfo %q resource found in %q namespace", buildReconciler, rootNamespace))))
			})
		})
	})
})

type buildpackInfo struct {
	name    string
	version string
}

func createBuilderInfoWithCleanup(ctx context.Context, name, stack string, buildpacks []buildpackInfo) *v1alpha1.BuilderInfo {
	builderInfo := &v1alpha1.BuilderInfo{
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

	builderInfo.Status.Stacks = []v1alpha1.BuilderInfoStatusStack{
		{
			Name:              stack,
			CreationTimestamp: metav1.Time{Time: time.Now()},
			UpdatedTimestamp:  metav1.Time{Time: time.Now()},
		},
	}
	for _, b := range buildpacks {
		builderInfo.Status.Buildpacks = append(builderInfo.Status.Buildpacks, v1alpha1.BuilderInfoStatusBuildpack{
			Name:              b.name,
			Version:           b.version,
			Stack:             stack,
			CreationTimestamp: creationTimestamp,
			UpdatedTimestamp:  updatedTimestamp,
		})
	}

	meta.SetStatusCondition(&builderInfo.Status.Conditions, metav1.Condition{
		Type:   "Ready",
		Status: metav1.ConditionTrue,
		Reason: "testing",
	})
	Expect(k8sClient.Status().Update(ctx, builderInfo)).To(Succeed())
	return builderInfo
}
