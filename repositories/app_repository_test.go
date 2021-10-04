package repositories_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "code.cloudfoundry.org/cf-k8s-api/repositories"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = SuiteDescribe("App Repository FetchApp", testFetchApp)
var _ = SuiteDescribe("App Repository FetchAppList", testFetchAppList)
var _ = SuiteDescribe("App Repository CreateApp", testCreateApp)
var _ = SuiteDescribe("App Repository Secret Create/Update", testEnvSecretCreate)
var _ = SuiteDescribe("App Repository FetchNamespace", func(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)
	when("space does not exist", func() {
		it("returns an unauthorized or not found err", func() {
			appRepo := AppRepo{}
			client, err := BuildCRClient(k8sConfig)

			_, err = appRepo.FetchNamespace(context.Background(), client, "some-guid")
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError("Resource not found or permission denied."))
		})
	})
})

func testFetchApp(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	var (
		testCtx context.Context
		appRepo *AppRepo
		client  client.Client

		namespace1 *corev1.Namespace
		namespace2 *corev1.Namespace
	)

	it.Before(func() {
		testCtx = context.Background()

		appRepo = new(AppRepo)
		var err error
		client, err = BuildCRClient(k8sConfig)
		g.Expect(err).ToNot(HaveOccurred())

		namespace1Name := generateGUID()
		namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
		g.Expect(k8sClient.Create(context.Background(), namespace1)).To(Succeed())

		namespace2Name := generateGUID()
		namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
		g.Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())
	})

	when("on the happy path", func() {
		var (
			app1GUID string
			app2GUID string
			cfApp1   *workloadsv1alpha1.CFApp
			cfApp2   *workloadsv1alpha1.CFApp
		)
		it.Before(func() {
			app1GUID = generateGUID()
			app2GUID = generateGUID()
			cfApp1 = &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      app1GUID,
					Namespace: namespace1.Name,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "test-app1",
					DesiredState: "STOPPED",
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), cfApp1)).To(Succeed())

			cfApp2 = &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      app2GUID,
					Namespace: namespace2.Name,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "test-app2",
					DesiredState: "STOPPED",
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{"java"},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), cfApp2)).To(Succeed())
		})

		it.After(func() {
			g.Expect(k8sClient.Delete(context.Background(), cfApp1)).To(Succeed())
			g.Expect(k8sClient.Delete(context.Background(), cfApp2)).To(Succeed())
		})

		it("can fetch the AppRecord CR we're looking for", func() {
			app, err := appRepo.FetchApp(testCtx, client, app2GUID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(app.GUID).To(Equal(app2GUID))
			g.Expect(app.Name).To(Equal("test-app2"))
			g.Expect(app.SpaceGUID).To(Equal(namespace2.Name))
			g.Expect(app.State).To(Equal(DesiredState("STOPPED")))

			expectedLifecycle := Lifecycle{
				Data: LifecycleData{
					Buildpacks: []string{"java"},
					Stack:      "",
				},
			}
			g.Expect(app.Lifecycle).To(Equal(expectedLifecycle))
		})
	})

	when("duplicate Apps exist across namespaces with the same GUIDs", func() {
		var (
			testAppGUID string
			cfApp1      *workloadsv1alpha1.CFApp
			cfApp2      *workloadsv1alpha1.CFApp
		)

		it.Before(func() {
			testAppGUID = generateGUID()

			cfApp1 = &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testAppGUID,
					Namespace: namespace1.Name,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "test-app1",
					DesiredState: "STOPPED",
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), cfApp1)).To(Succeed())

			cfApp2 = &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testAppGUID,
					Namespace: namespace2.Name,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "test-app2",
					DesiredState: "STOPPED",
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), cfApp2)).To(Succeed())
		})

		it.After(func() {
			g.Expect(k8sClient.Delete(context.Background(), cfApp1)).To(Succeed())
			g.Expect(k8sClient.Delete(context.Background(), cfApp2)).To(Succeed())
		})

		it("returns an error", func() {
			_, err := appRepo.FetchApp(testCtx, client, testAppGUID)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError("duplicate apps exist"))
		})
	})

	when("no Apps exist", func() {
		it("returns an error", func() {
			_, err := appRepo.FetchApp(testCtx, client, "i don't exist")
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError(NotFoundError{}))
		})
	})
}

func testFetchAppList(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	var testCtx context.Context
	const namespace = "default"

	it.Before(func() {
		testCtx = context.Background()
	})

	when("multiple Apps exist", func() {
		var (
			app1GUID string
			app2GUID string
			cfApp1   *workloadsv1alpha1.CFApp
			cfApp2   *workloadsv1alpha1.CFApp
		)
		it.Before(func() {
			beforeCtx := context.Background()
			app1GUID = generateGUID()
			app2GUID = generateGUID()
			cfApp1 = &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      app1GUID,
					Namespace: namespace,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "test-app1",
					DesiredState: "STOPPED",
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(beforeCtx, cfApp1)).To(Succeed())

			cfApp2 = &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      app2GUID,
					Namespace: namespace,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "test-app2",
					DesiredState: "STOPPED",
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{"java"},
							Stack:      "",
						},
					},
				},
			}
			g.Expect(k8sClient.Create(beforeCtx, cfApp2)).To(Succeed())
		})

		it.After(func() {
			afterCtx := context.Background()
			g.Expect(k8sClient.Delete(afterCtx, cfApp1)).To(Succeed())
			g.Expect(k8sClient.Delete(afterCtx, cfApp2)).To(Succeed())
		})

		// TODO: Update this test annotation to reflect proper filtering by caller permissions when that is available
		it("returns all the AppRecord CRs", func() {
			appRepo := AppRepo{}
			client, err := BuildCRClient(k8sConfig)
			g.Expect(err).ToNot(HaveOccurred())

			appList, err := appRepo.FetchAppList(testCtx, client)
			g.Expect(err).NotTo(HaveOccurred())
			// Expect response list to contain 2 particular apps, unordered
			g.Expect(appList).To(HaveLen(2), "repository should return 2 app records")
			// Assert on equality for each expected appRecord? Could just hardcode checks for each app?

		})
	})

	when("no Apps exist", func() {
		// This test setup may fail if we run tests in parallel that create app records
		it("returns an error", func() {
			appRepo := AppRepo{}
			client, err := BuildCRClient(k8sConfig)
			g.Expect(err).ToNot(HaveOccurred())

			_, err = appRepo.FetchAppList(testCtx, client)
			g.Expect(err).NotTo(HaveOccurred())
		})
	})
}

func initializeAppCR(appName string, appGUID string, spaceGUID string) workloadsv1alpha1.CFApp {
	return workloadsv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appGUID,
			Namespace: spaceGUID,
		},
		Spec: workloadsv1alpha1.CFAppSpec{
			Name:         appName,
			DesiredState: "STOPPED",
			Lifecycle: workloadsv1alpha1.Lifecycle{
				Type: "buildpack",
				Data: workloadsv1alpha1.LifecycleData{
					Buildpacks: []string{},
					Stack:      "",
				},
			},
		},
	}
}

func initializeAppRecord(appName string, appGUID string, spaceGUID string) AppRecord {
	return AppRecord{
		Name:      appName,
		GUID:      appGUID,
		SpaceGUID: spaceGUID,
		State:     "STOPPED",
		Lifecycle: Lifecycle{
			Type: "buildpack",
			Data: LifecycleData{
				Buildpacks: []string{},
				Stack:      "cflinuxfs3",
			},
		},
	}
}

func generateGUID() string {
	newUUID, err := uuid.NewUUID()
	if err != nil {
		errorMessage := fmt.Sprintf("could not generate a UUID %v", err)
		panic(errorMessage)
	}
	return newUUID.String()
}

func cleanupApp(k8sClient client.Client, ctx context.Context, appGUID, appNamespace string) error {
	app := workloadsv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appGUID,
			Namespace: appNamespace,
		},
	}
	return k8sClient.Delete(ctx, &app)
}

func testCreateApp(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		defaultNamespace = "default"
	)

	when("creating an App record and", func() {
		const (
			testAppName = "test-app-name"
		)
		var (
			testAppGUID    string
			emptyAppRecord = AppRecord{}
			testCtx        context.Context
		)
		it.Before(func() {
			testAppGUID = generateGUID()
			testCtx = context.Background()
		})

		when("app does not already exist", func() {
			var (
				appRepo   AppRepo
				client    client.Client
				appRecord AppRecord
			)

			it.Before(func() {
				appRepo = AppRepo{}

				var err error
				client, err = BuildCRClient(k8sConfig)
				g.Expect(err).NotTo(HaveOccurred())

				appRecord = initializeAppRecord(testAppName, testAppGUID, defaultNamespace)
			})

			it("should create a new app CR successfully", func() {
				createdAppRecord, err := appRepo.CreateApp(testCtx, client, appRecord)
				g.Expect(err).To(BeNil())
				g.Expect(createdAppRecord).NotTo(BeNil())

				cfAppLookupKey := types.NamespacedName{Name: testAppGUID, Namespace: defaultNamespace}
				createdCFApp := new(workloadsv1alpha1.CFApp)
				g.Eventually(func() string {
					err := k8sClient.Get(context.Background(), cfAppLookupKey, createdCFApp)
					if err != nil {
						return ""
					}
					return createdCFApp.Name
				}, 10*time.Second, 250*time.Millisecond).Should(Equal(testAppGUID))
				g.Expect(cleanupApp(k8sClient, testCtx, testAppGUID, defaultNamespace)).To(Succeed())
			})

			when("an app is created with the repository", func() {
				var (
					beforeCreationTime time.Time
					createdAppRecord   AppRecord
				)

				it.Before(func() {
					beforeCtx := context.Background()
					beforeCreationTime = time.Now().UTC().AddDate(0, 0, -1)

					var err error
					createdAppRecord, err = appRepo.CreateApp(beforeCtx, client, appRecord)
					g.Expect(err).To(BeNil())
				})

				it.After(func() {
					afterCtx := context.Background()
					g.Expect(cleanupApp(k8sClient, afterCtx, testAppGUID, defaultNamespace)).To(Succeed())
				})

				it("should return a non-empty AppRecord", func() {
					g.Expect(createdAppRecord).NotTo(Equal(emptyAppRecord))
				})

				it("should return an AppRecord with matching GUID, spaceGUID, and name", func() {
					g.Expect(createdAppRecord.GUID).To(Equal(testAppGUID), "App GUID in record did not match input")
					g.Expect(createdAppRecord.SpaceGUID).To(Equal(defaultNamespace), "App SpaceGUID in record did not match input")
					g.Expect(createdAppRecord.Name).To(Equal(testAppName), "App Name in record did not match input")
				})

				it("should return an AppRecord with CreatedAt and UpdatedAt fields that make sense", func() {
					afterTestTime := time.Now().UTC().AddDate(0, 0, 1)
					recordCreatedTime, err := time.Parse(TimestampFormat, createdAppRecord.CreatedAt)
					g.Expect(err).To(BeNil(), "There was an error converting the createAppRecord CreatedTime to string")
					recordUpdatedTime, err := time.Parse(TimestampFormat, createdAppRecord.UpdatedAt)
					g.Expect(err).To(BeNil(), "There was an error converting the createAppRecord UpdatedTime to string")

					g.Expect(recordCreatedTime.After(beforeCreationTime)).To(BeTrue(), "app record creation time was not after the expected creation time")
					g.Expect(recordCreatedTime.Before(afterTestTime)).To(BeTrue(), "app record creation time was not before the expected testing time")

					g.Expect(recordUpdatedTime.After(beforeCreationTime)).To(BeTrue(), "app record updated time was not after the expected creation time")
					g.Expect(recordUpdatedTime.Before(afterTestTime)).To(BeTrue(), "app record updated time was not before the expected testing time")
				})

			})
		})

		when("the app already exists", func() {
			var (
				appCR   workloadsv1alpha1.CFApp
				appRepo AppRepo
				client  client.Client
			)

			it.Before(func() {
				beforeCtx := context.Background()
				appCR = initializeAppCR(testAppName, testAppGUID, defaultNamespace)

				g.Expect(k8sClient.Create(beforeCtx, &appCR)).To(Succeed())

				appRepo = AppRepo{}
				client, _ = BuildCRClient(k8sConfig)
			})

			it.After(func() {
				afterCtx := context.Background()
				g.Expect(k8sClient.Delete(afterCtx, &appCR)).To(Succeed())
			})

			it("should error when trying to create the same app again", func() {
				appRecord := initializeAppRecord(testAppName, testAppGUID, defaultNamespace)
				createdAppRecord, err := appRepo.CreateApp(testCtx, client, appRecord)
				g.Expect(err).NotTo(BeNil())
				g.Expect(createdAppRecord).To(Equal(emptyAppRecord))
			})
		})
	})
}

func generateAppEnvSecretName(appGUID string) string {
	return appGUID + "-env"
}

func testEnvSecretCreate(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		defaultNamespace = "default"
	)

	when("an envSecret is created for a CFApp with the Repo", func() {
		const (
			testAppName = "test-app-name"
		)
		var (
			appRepo                  AppRepo
			client                   client.Client
			testAppGUID              string
			testAppEnvSecretName     string
			testAppEnvSecret         AppEnvVarsRecord
			returnedAppEnvVarsRecord AppEnvVarsRecord
			returnedErr              error
		)
		it.Before(func() {
			beforeCtx := context.Background()
			appRepo = AppRepo{}
			client, _ = BuildCRClient(k8sConfig)
			testAppGUID = generateGUID()
			testAppEnvSecretName = generateAppEnvSecretName(testAppGUID)
			testAppEnvSecret = AppEnvVarsRecord{
				AppGUID:              testAppGUID,
				SpaceGUID:            defaultNamespace,
				EnvironmentVariables: map[string]string{"foo": "foo", "bar": "bar"},
			}

			returnedAppEnvVarsRecord, returnedErr = appRepo.CreateAppEnvironmentVariables(beforeCtx, client, testAppEnvSecret)

		})

		it.After(func() {
			afterCtx := context.Background()
			lookupSecretK8sResource := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testAppEnvSecretName,
					Namespace: defaultNamespace,
				},
			}
			g.Expect(client.Delete(afterCtx, &lookupSecretK8sResource)).To(Succeed(), "Could not clean up the created App Env Secret")
		})

		it("returns a record matching the input and no error", func() {
			g.Expect(returnedAppEnvVarsRecord.AppGUID).To(Equal(testAppEnvSecret.AppGUID))
			g.Expect(returnedAppEnvVarsRecord.SpaceGUID).To(Equal(testAppEnvSecret.SpaceGUID))
			g.Expect(len(returnedAppEnvVarsRecord.EnvironmentVariables)).To(Equal(len(testAppEnvSecret.EnvironmentVariables)))
			g.Expect(returnedErr).To(BeNil())
		})

		it("returns a record with the created Secret's name", func() {
			g.Expect(returnedAppEnvVarsRecord.Name).ToNot(BeEmpty())
		})

		when("examining the created Secret in the k8s api", func() {
			var (
				createdCFAppSecret corev1.Secret
			)
			it.Before(func() {
				beforeCtx := context.Background()
				cfAppSecretLookupKey := types.NamespacedName{Name: testAppEnvSecretName, Namespace: defaultNamespace}
				createdCFAppSecret = corev1.Secret{}
				g.Eventually(func() bool {
					err := client.Get(beforeCtx, cfAppSecretLookupKey, &createdCFAppSecret)
					if err != nil {
						return false
					}
					return true
				}, 10*time.Second, 250*time.Millisecond).Should(BeTrue(), "could not find the secret created by the repo")
			})
			it("is not empty", func() {
				g.Expect(createdCFAppSecret).ToNot(Equal(corev1.Secret{}))
			})
			it("has a Name that is derived from the CFApp", func() {
				g.Expect(createdCFAppSecret.Name).To(Equal(testAppEnvSecretName))
			})
			it("has a label that matches the CFApp GUID", func() {
				labelValue, exists := createdCFAppSecret.Labels[CFAppGUIDLabel]
				g.Expect(exists).To(BeTrue(), "label for envSecret AppGUID not found")
				g.Expect(labelValue).To(Equal(testAppGUID))
			})
			it("contains string data that matches the input record length", func() {
				g.Expect(len(createdCFAppSecret.Data)).To(Equal(len(testAppEnvSecret.EnvironmentVariables)))
			})
		})

		it("returns an error if the secret already exists", func() {
			testCtx := context.Background()
			testAppEnvSecret.EnvironmentVariables = map[string]string{"foo": "foo", "bar": "bar"}

			returnedUpdatedAppEnvVarsRecord, returnedUpdatedErr := appRepo.CreateAppEnvironmentVariables(testCtx, client, testAppEnvSecret)
			g.Expect(returnedUpdatedErr).ToNot(BeNil())
			g.Expect(returnedUpdatedAppEnvVarsRecord).To(Equal(AppEnvVarsRecord{}))
		})

		it("the App record GUID returned should equal the App GUID provided", func() {
			testCtx := context.Background()
			// Used a strings.Trim to remove characters, which cause the behavior in Issue #103
			testAppEnvSecret.AppGUID = "estringtrimmedguid"

			returnedUpdatedAppEnvVarsRecord, returnedUpdatedErr := appRepo.CreateAppEnvironmentVariables(testCtx, client, testAppEnvSecret)
			g.Expect(returnedUpdatedErr).ToNot(HaveOccurred())
			g.Expect(returnedUpdatedAppEnvVarsRecord.AppGUID).To(Equal(testAppEnvSecret.AppGUID), "Expected App GUID to match after transform")
		})

	})
}
