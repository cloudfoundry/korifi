package repositories_test

import (
	"context"
	"testing"
	"time"

	. "code.cloudfoundry.org/cf-k8s-api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = SuiteDescribe("Build Repository FetchBuild", testFetchBuild)

func testFetchBuild(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		app1GUID      = "app-1-guid"
		app2GUID      = "app-2-guid"
		package1GUID  = "package-1-guid"
		package2GUID  = "package-2-guid"
		stagingMemory = 1024
		stagingDisk   = 2048
	)

	var (
		testCtx   context.Context
		buildRepo *BuildRepo
		client    client.Client

		namespace1 *corev1.Namespace
		namespace2 *corev1.Namespace
	)

	it.Before(func() {
		testCtx = context.Background()

		namespace1Name := generateGUID()
		namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
		g.Expect(k8sClient.Create(context.Background(), namespace1)).To(Succeed())

		namespace2Name := generateGUID()
		namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
		g.Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())

		buildRepo = new(BuildRepo)
		var err error
		client, err = BuildClient(k8sConfig)
		g.Expect(err).ToNot(HaveOccurred())
	})

	it.After(func() {
		g.Expect(k8sClient.Delete(context.Background(), namespace1)).To(Succeed())
		g.Expect(k8sClient.Delete(context.Background(), namespace2)).To(Succeed())
	})

	when("on the happy path", func() {

		var (
			build1GUID string
			build2GUID string
			build1     *workloadsv1alpha1.CFBuild
			build2     *workloadsv1alpha1.CFBuild
		)

		it.Before(func() {
			build1GUID = generateGUID()
			build2GUID = generateGUID()
			build1 = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      build1GUID,
					Namespace: namespace1.Name,
				},
				Spec: workloadsv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: package1GUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: app1GUID,
					},
					StagingMemoryMB: stagingMemory,
					StagingDiskMB:   stagingDisk,
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), build1)).To(Succeed())

			build2 = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      build2GUID,
					Namespace: namespace2.Name,
				},
				Spec: workloadsv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: package2GUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: app2GUID,
					},
					StagingMemoryMB: stagingMemory,
					StagingDiskMB:   stagingDisk,
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), build2)).To(Succeed())
		})

		it.After(func() {
			g.Expect(k8sClient.Delete(context.Background(), build1)).To(Succeed())
			g.Expect(k8sClient.Delete(context.Background(), build2)).To(Succeed())
		})

		it("can fetch the BuildRecord we're looking for", func() {
			record, err := buildRepo.FetchBuild(testCtx, client, build2GUID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(record.GUID).To(Equal(build2GUID))
			// TODO: fill me in!

			createdAt, err := time.Parse(time.RFC3339, record.CreatedAt)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(createdAt).To(BeTemporally("~", time.Now(), time.Second))

			updatedAt, err := time.Parse(time.RFC3339, record.CreatedAt)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updatedAt).To(BeTemporally("~", time.Now(), time.Second))
		})
	})

	when("duplicate Builds exist across namespaces with the same GUID", func() {
		var (
			buildGUID string
			build1    *workloadsv1alpha1.CFBuild
			build2    *workloadsv1alpha1.CFBuild
		)

		it.Before(func() {
			buildGUID = generateGUID()
			build1 = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildGUID,
					Namespace: namespace1.Name,
				},
				Spec: workloadsv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: package1GUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: app1GUID,
					},
					StagingMemoryMB: stagingMemory,
					StagingDiskMB:   stagingDisk,
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), build1)).To(Succeed())

			build2 = &workloadsv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Name:      buildGUID,
					Namespace: namespace2.Name,
				},
				Spec: workloadsv1alpha1.CFBuildSpec{
					PackageRef: corev1.LocalObjectReference{
						Name: package2GUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: app2GUID,
					},
					StagingMemoryMB: stagingMemory,
					StagingDiskMB:   stagingDisk,
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), build2)).To(Succeed())
		})

		it.After(func() {
			g.Expect(k8sClient.Delete(context.Background(), build1)).To(Succeed())
			g.Expect(k8sClient.Delete(context.Background(), build2)).To(Succeed())
		})

		it("returns an error", func() {
			_, err := buildRepo.FetchBuild(testCtx, client, buildGUID)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError("duplicate builds exist"))
		})
	})

	when("no builds exist", func() {
		it("returns an error", func() {
			_, err := buildRepo.FetchBuild(testCtx, client, "i don't exist")
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError(NotFoundError{}))
		})
	})
}
