package repositories_test

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	. "code.cloudfoundry.org/cf-k8s-api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = SuiteDescribe("Build Repository FetchBuild", testFetchBuild)
var _ = SuiteDescribe("Build Repository CreateBuild", testCreateBuild)

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
		client, err = BuildCRClient(k8sConfig)
		g.Expect(err).ToNot(HaveOccurred())
	})

	it.After(func() {
		g.Expect(k8sClient.Delete(context.Background(), namespace1)).To(Succeed())
		g.Expect(k8sClient.Delete(context.Background(), namespace2)).To(Succeed())
	})

	when("on the happy path", func() {

		const (
			StagingConditionType   = "Staging"
			ReadyConditionType     = "Ready"
			SucceededConditionType = "Succeeded"
		)

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

		when("fetching a build", func() {
			var (
				buildRecord *BuildRecord
				fetchError  error
			)
			when("no status.Conditions are set", func() {
				it.Before(func() {
					returnedBuildRecord, err := buildRepo.FetchBuild(testCtx, client, build2GUID)
					buildRecord = &returnedBuildRecord
					fetchError = err
				})

				it("succeeds", func() {
					g.Expect(fetchError).NotTo(HaveOccurred())
				})

				it("returns a record with a matching GUID", func() {
					g.Expect(buildRecord.GUID).To(Equal(build2GUID))
				})

				it("returns a record with state \"STAGING\" and no StagingErrorMsg", func() {
					g.Expect(buildRecord.State).To(Equal("STAGING"))
					g.Expect(buildRecord.StagingErrorMsg).To(BeEmpty(), "record staging error message was supposed to be empty")
				})

				it("returns a record with no DropletGUID", func() {
					g.Expect(buildRecord.DropletGUID).To(BeEmpty())
				})

				it("returns a record with a CreatedAt field from the CR", func() {
					createdAt, err := time.Parse(time.RFC3339, buildRecord.CreatedAt)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
				})

				it("returns a record with a UpdatedAt field from the CR", func() {
					updatedAt, err := time.Parse(time.RFC3339, buildRecord.UpdatedAt)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
				})

				it("returns a record with a StagingMemoryMB field matching the CR", func() {
					g.Expect(buildRecord.StagingMemoryMB).To(Equal(build2.Spec.StagingMemoryMB))
				})

				it("returns a record with a StagingDiskMB field matching the CR", func() {
					g.Expect(buildRecord.StagingDiskMB).To(Equal(build2.Spec.StagingDiskMB))
				})

				it("returns a record with Lifecycle fields matching the CR", func() {
					g.Expect(buildRecord.Lifecycle.Type).To(Equal(string(build2.Spec.Lifecycle.Type)), "returned record lifecycle.type did not match CR")
					g.Expect(buildRecord.Lifecycle.Data.Buildpacks).To(Equal(build2.Spec.Lifecycle.Data.Buildpacks), "returned record lifecycle.data.buildpacks did not match CR")
					g.Expect(buildRecord.Lifecycle.Data.Stack).To(Equal(build2.Spec.Lifecycle.Data.Stack), "returned record lifecycle.data.stack did not match CR")
				})

				it("returns a record with a PackageGUID field matching the CR", func() {
					g.Expect(buildRecord.PackageGUID).To(Equal(build2.Spec.PackageRef.Name))
				})

				it("returns a record with an AppGUID field matching the CR", func() {
					g.Expect(buildRecord.AppGUID).To(Equal(build2.Spec.AppRef.Name))
				})
			})

			when("status.Conditions \"Staging\": False, \"Succeeded\": True, is set", func() {
				it.Before(func() {
					meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
						Type:    StagingConditionType,
						Status:  metav1.ConditionFalse,
						Reason:  "kpack",
						Message: "kpack",
					})
					meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
						Type:    SucceededConditionType,
						Status:  metav1.ConditionTrue,
						Reason:  "Unknown",
						Message: "Unknown",
					})
					// Update Build Status Conditions based on changes made to local copy
					g.Expect(k8sClient.Status().Update(testCtx, build2)).To(Succeed())
				})

				it("should eventually return a record with State: \"STAGED\", no StagingErrorMsg, and a DropletGUID that matches the BuildGUID", func() {
					ctx := context.Background()
					g.Eventually(func() string {

						returnedBuildRecord, err := buildRepo.FetchBuild(ctx, client, build2GUID)
						buildRecord = &returnedBuildRecord
						fetchError = err
						if err != nil {
							return ""
						}
						return buildRecord.State
					}, 10*time.Second, 250*time.Millisecond).Should(Equal("STAGED"), "the returned record State was not STAGED")
					g.Expect(buildRecord.DropletGUID).To(Equal(build2.Name), " the returned dropletGUID did not match the buildGUID")
					g.Expect(buildRecord.StagingErrorMsg).To(BeEmpty(), "record staging error message was supposed to be empty")
				})
			})

			when("status.Conditions \"Staging\": False, \"Succeeded\": False, is set", func() {
				const (
					StagingError        = "StagingError"
					StagingErrorMessage = "Staging failed for some reason"
				)

				it.Before(func() {
					meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
						Type:    StagingConditionType,
						Status:  metav1.ConditionFalse,
						Reason:  "kpack",
						Message: "kpack",
					})
					meta.SetStatusCondition(&build2.Status.Conditions, metav1.Condition{
						Type:    SucceededConditionType,
						Status:  metav1.ConditionFalse,
						Reason:  "StagingError",
						Message: StagingErrorMessage,
					})
					// Update Build Status Conditions based on changes made to local copy
					g.Expect(k8sClient.Status().Update(testCtx, build2)).To(Succeed())
				})

				it("should eventually return a record with State: \"FAILED\", no DropletGUID, and a Staging Error Message", func() {
					ctx := context.Background()
					g.Eventually(func() string {
						returnedBuildRecord, err := buildRepo.FetchBuild(ctx, client, build2GUID)
						buildRecord = &returnedBuildRecord
						fetchError = err
						if err != nil {
							return ""
						}
						return buildRecord.State
					}, 10*time.Second, 250*time.Millisecond).Should(Equal("FAILED"), "the returned record State was not FAILED")
					g.Expect(buildRecord.DropletGUID).To(BeEmpty())
					g.Expect(buildRecord.StagingErrorMsg).To(Equal(StagingError + ": " + StagingErrorMessage))
				})
			})
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

func testCreateBuild(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		appGUID     = "the-app-guid"
		packageGUID = "the-package-guid"

		buildStagingState = "STAGING"

		buildLifecycleType = "buildpack"
		buildStack         = "cflinuxfs3"

		stagingMemory = 1024
		stagingDisk   = 2048
	)

	var (
		buildRepo              *BuildRepo
		client                 client.Client
		buildCreateLabels      map[string]string
		buildCreateAnnotations map[string]string
		buildCreateMsg         BuildCreateMessage
		spaceGUID              string
	)

	it.Before(func() {
		buildRepo = new(BuildRepo)

		var err error
		client, err = BuildCRClient(k8sConfig)
		g.Expect(err).NotTo(HaveOccurred())

		beforeCtx := context.Background()
		spaceGUID = generateGUID()
		g.Expect(
			k8sClient.Create(beforeCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}}),
		).To(Succeed())

		buildCreateLabels = nil
		buildCreateAnnotations = nil
		buildCreateMsg = BuildCreateMessage{
			AppGUID:         appGUID,
			PackageGUID:     packageGUID,
			SpaceGUID:       spaceGUID,
			StagingMemoryMB: stagingMemory,
			StagingDiskMB:   stagingDisk,
			Lifecycle: Lifecycle{
				Type: buildLifecycleType,
				Data: LifecycleData{
					Buildpacks: []string{},
					Stack:      buildStack,
				},
			},
			Labels:      buildCreateLabels,
			Annotations: buildCreateAnnotations,
		}

	})

	when("creating a Build", func() {

		var (
			buildCreateRecord BuildRecord
			buildCreateErr    error
		)

		it.Before(func() {
			ctx := context.Background()
			buildCreateRecord, buildCreateErr = buildRepo.CreateBuild(ctx, client, buildCreateMsg)
		})

		it.After(func() {
			afterCtx := context.Background()
			cleanupBuild(afterCtx, client, buildCreateRecord.GUID, spaceGUID)
		})

		it("does not return an error", func() {
			g.Expect(buildCreateErr).NotTo(HaveOccurred())
		})

		when("examining the returned record", func() {
			it("is not empty", func() {
				g.Expect(buildCreateRecord).ToNot(Equal(BuildCreateMessage{}))
			})
			it("contains a GUID", func() {
				g.Expect(buildCreateRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "record GUID was not a 36 character guid")
			})
			it("has a State of \"STAGING\"", func() {
				g.Expect(buildCreateRecord.State).To(Equal(buildStagingState))
			})
			it("has a CreatedAt that makes sense", func() {
				createdAt, err := time.Parse(time.RFC3339, buildCreateRecord.CreatedAt)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
			})
			it("has a UpdatedAt that makes sense", func() {
				createdAt, err := time.Parse(time.RFC3339, buildCreateRecord.UpdatedAt)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
			})
			it("has an empty StagingErrorMsg", func() {
				g.Expect(buildCreateRecord.StagingErrorMsg).To(BeEmpty())
			})
			it("has StagingMemoryMB that matches the CreateMessage", func() {
				g.Expect(buildCreateRecord.StagingMemoryMB).To(Equal(stagingMemory))
			})
			it("has StagingDiskMB that matches the CreateMessage", func() {
				g.Expect(buildCreateRecord.StagingDiskMB).To(Equal(stagingDisk))
			})
			it("has Lifecycle fields that match the CreateMessage", func() {
				g.Expect(buildCreateRecord.Lifecycle.Type).To(Equal(buildLifecycleType))
				g.Expect(buildCreateRecord.Lifecycle.Data.Stack).To(Equal(buildStack))
			})
			it("has a PackageGUID that matches the CreateMessage", func() {
				g.Expect(buildCreateRecord.PackageGUID).To(Equal(packageGUID))
			})
			it("has no DropletGUID", func() {
				g.Expect(buildCreateRecord.DropletGUID).To(BeEmpty())
			})
			it("has an AppGUID that matches the CreateMessage", func() {
				g.Expect(buildCreateRecord.AppGUID).To(Equal(appGUID))
			})
			it("has Labels that match the CreateMessage", func() {
				g.Expect(buildCreateRecord.Labels).To(Equal(buildCreateLabels))
			})
			it("has Annotations that match the CreateMessage", func() {
				g.Expect(buildCreateRecord.Annotations).To(Equal(buildCreateAnnotations))
			})
		})

		it("should eventually create a new Build CR", func() {
			cfBuildLookupKey := types.NamespacedName{Name: buildCreateRecord.GUID, Namespace: spaceGUID}
			createdCFBuild := new(workloadsv1alpha1.CFBuild)
			g.Eventually(func() bool {
				err := k8sClient.Get(context.Background(), cfBuildLookupKey, createdCFBuild)
				return err == nil
			}, 5*time.Second, 250*time.Millisecond).Should(BeTrue(), "A CFBuild CR was not eventually created")
		})

	})
}

func cleanupBuild(ctx context.Context, k8sClient client.Client, buildGUID, namespace string) error {
	cfBuild := workloadsv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildGUID,
			Namespace: namespace,
		},
	}
	return k8sClient.Delete(ctx, &cfBuild)
}
