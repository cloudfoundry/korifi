package repositories_test

import (
	"context"
	"time"

	. "code.cloudfoundry.org/cf-k8s-api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AppRepository", func() {
	var (
		testCtx context.Context
		appRepo *AppRepo
		client  client.Client
	)

	BeforeEach(func() {
		testCtx = context.Background()

		appRepo = new(AppRepo)
		var err error
		client, err = BuildCRClient(k8sConfig)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("GetApp", func() {
		var (
			namespace1 *corev1.Namespace
			namespace2 *corev1.Namespace
		)

		BeforeEach(func() {
			namespace1Name := generateGUID()
			namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
			Expect(k8sClient.Create(context.Background(), namespace1)).To(Succeed())

			namespace2Name := generateGUID()
			namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
			Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())
		})

		When("on the happy path", func() {
			const (
				app2DropletGUID = "app2-droplet-guid"
			)
			var (
				app1GUID string
				app2GUID string
				cfApp1   *workloadsv1alpha1.CFApp
				cfApp2   *workloadsv1alpha1.CFApp
			)

			BeforeEach(func() {
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
				Expect(k8sClient.Create(context.Background(), cfApp1)).To(Succeed())

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
						CurrentDropletRef: corev1.LocalObjectReference{
							Name: app2DropletGUID,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), cfApp2)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), cfApp1)).To(Succeed())
				Expect(k8sClient.Delete(context.Background(), cfApp2)).To(Succeed())
			})

			It("can fetch the AppRecord CR we're looking for", func() {
				app, err := appRepo.FetchApp(testCtx, client, app2GUID)
				Expect(err).NotTo(HaveOccurred())
				Expect(app.GUID).To(Equal(app2GUID))
				Expect(app.Name).To(Equal("test-app2"))
				Expect(app.SpaceGUID).To(Equal(namespace2.Name))
				Expect(app.State).To(Equal(DesiredState("STOPPED")))

				By("returning a record with the App's DropletGUID when it is set", func() {
					Expect(app.DropletGUID).To(Equal(app2DropletGUID))
				})

				expectedLifecycle := Lifecycle{
					Data: LifecycleData{
						Buildpacks: []string{"java"},
						Stack:      "",
					},
				}
				Expect(app.Lifecycle).To(Equal(expectedLifecycle))
			})

		})

		When("duplicate Apps exist across namespaces with the same GUIDs", func() {
			var (
				testAppGUID string
				cfApp1      *workloadsv1alpha1.CFApp
				cfApp2      *workloadsv1alpha1.CFApp
			)

			BeforeEach(func() {
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
				Expect(k8sClient.Create(context.Background(), cfApp1)).To(Succeed())

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
				Expect(k8sClient.Create(context.Background(), cfApp2)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), cfApp1)).To(Succeed())
				Expect(k8sClient.Delete(context.Background(), cfApp2)).To(Succeed())
			})

			It("returns an error", func() {
				_, err := appRepo.FetchApp(testCtx, client, testAppGUID)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("duplicate apps exist"))
			})
		})

		When("no Apps exist", func() {
			It("returns an error", func() {
				_, err := appRepo.FetchApp(testCtx, client, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(NotFoundError{}))
			})
		})
	})

	Describe("FetchAppList", func() {
		const namespace = "default"

		When("multiple Apps exist", func() {
			var (
				app1GUID string
				app2GUID string
				cfApp1   *workloadsv1alpha1.CFApp
				cfApp2   *workloadsv1alpha1.CFApp
			)

			BeforeEach(func() {
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
				Expect(k8sClient.Create(beforeCtx, cfApp1)).To(Succeed())

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
				Expect(k8sClient.Create(beforeCtx, cfApp2)).To(Succeed())
			})

			AfterEach(func() {
				afterCtx := context.Background()
				Expect(k8sClient.Delete(afterCtx, cfApp1)).To(Succeed())
				Expect(k8sClient.Delete(afterCtx, cfApp2)).To(Succeed())
			})

			// TODO: Update this test annotation to reflect proper filtering by caller permissions when that is available
			It("returns all the AppRecord CRs", func() {
				appList, err := appRepo.FetchAppList(testCtx, client)
				Expect(err).NotTo(HaveOccurred())
				Expect(appList).To(HaveLen(2), "repository should return 2 app records")
				// TODO: Assert on equality for each expected appRecord? Could just hardcode checks for each app?
			})
		})

		When("no Apps exist", func() {
			It("returns an error", func() {
				_, err := appRepo.FetchAppList(testCtx, client)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("CreateApp", func() {
		const (
			defaultNamespace = "default"
		)

		When("creating an App record and", func() {
			const (
				testAppName = "test-app-name"
			)

			var (
				testAppGUID    string
				emptyAppRecord = AppRecord{}
			)

			BeforeEach(func() {
				testAppGUID = generateGUID()
				testCtx = context.Background()
			})

			When("app does not already exist", func() {
				var (
					appRecord AppRecord
				)

				BeforeEach(func() {
					appRecord = initializeAppRecord(testAppName, testAppGUID, defaultNamespace)
				})

				It("should create a new app CR successfully", func() {
					createdAppRecord, err := appRepo.CreateApp(testCtx, client, appRecord)
					Expect(err).To(BeNil())
					Expect(createdAppRecord).NotTo(BeNil())

					cfAppLookupKey := types.NamespacedName{Name: testAppGUID, Namespace: defaultNamespace}
					createdCFApp := new(workloadsv1alpha1.CFApp)
					Eventually(func() string {
						err := k8sClient.Get(context.Background(), cfAppLookupKey, createdCFApp)
						if err != nil {
							return ""
						}
						return createdCFApp.Name
					}, 10*time.Second, 250*time.Millisecond).Should(Equal(testAppGUID))
					Expect(cleanupApp(k8sClient, testCtx, testAppGUID, defaultNamespace)).To(Succeed())
				})

				When("an app is created with the repository", func() {
					var (
						beforeCreationTime time.Time
						createdAppRecord   AppRecord
					)

					BeforeEach(func() {
						beforeCreationTime = time.Now().UTC().AddDate(0, 0, -1)

						var err error
						createdAppRecord, err = appRepo.CreateApp(context.Background(), client, appRecord)
						Expect(err).To(BeNil())
					})

					AfterEach(func() {
						Expect(
							cleanupApp(k8sClient, context.Background(), testAppGUID, defaultNamespace),
						).To(Succeed())
					})

					It("should return a non-empty AppRecord", func() {
						Expect(createdAppRecord).NotTo(Equal(emptyAppRecord))
					})

					It("should return an AppRecord with matching GUID, spaceGUID, and name", func() {
						Expect(createdAppRecord.GUID).To(Equal(testAppGUID), "App GUID in record did not match input")
						Expect(createdAppRecord.SpaceGUID).To(Equal(defaultNamespace), "App SpaceGUID in record did not match input")
						Expect(createdAppRecord.Name).To(Equal(testAppName), "App Name in record did not match input")
					})

					It("should return an AppRecord with CreatedAt and UpdatedAt fields that make sense", func() {
						afterTestTime := time.Now().UTC().AddDate(0, 0, 1)
						recordCreatedTime, err := time.Parse(TimestampFormat, createdAppRecord.CreatedAt)
						Expect(err).To(BeNil(), "There was an error converting the createAppRecord CreatedTime to string")
						recordUpdatedTime, err := time.Parse(TimestampFormat, createdAppRecord.UpdatedAt)
						Expect(err).To(BeNil(), "There was an error converting the createAppRecord UpdatedTime to string")

						Expect(recordCreatedTime.After(beforeCreationTime)).To(BeTrue(), "app record creation time was not after the expected creation time")
						Expect(recordCreatedTime.Before(afterTestTime)).To(BeTrue(), "app record creation time was not before the expected testing time")

						Expect(recordUpdatedTime.After(beforeCreationTime)).To(BeTrue(), "app record updated time was not after the expected creation time")
						Expect(recordUpdatedTime.Before(afterTestTime)).To(BeTrue(), "app record updated time was not before the expected testing time")
					})

				})
			})

			When("the app already exists", func() {
				var (
					appCR *workloadsv1alpha1.CFApp
				)

				BeforeEach(func() {
					appCR = initializeAppCR(testAppName, testAppGUID, defaultNamespace)
					Expect(k8sClient.Create(context.Background(), appCR)).To(Succeed())
				})

				AfterEach(func() {
					Expect(k8sClient.Delete(context.Background(), appCR)).To(Succeed())
				})

				It("should error when trying to create the same app again", func() {
					appRecord := initializeAppRecord(testAppName, testAppGUID, defaultNamespace)
					createdAppRecord, err := appRepo.CreateApp(testCtx, client, appRecord)
					Expect(err).NotTo(BeNil())
					Expect(createdAppRecord).To(Equal(emptyAppRecord))
				})
			})
		})
	})

	Describe("CreateAppEnvSecret", func() {
		const (
			defaultNamespace = "default"
		)

		When("an envSecret is created for a CFApp with the Repo", func() {
			var (
				testAppGUID              string
				testAppEnvSecretName     string
				testAppEnvSecret         AppEnvVarsRecord
				returnedAppEnvVarsRecord AppEnvVarsRecord
				returnedErr              error
			)

			BeforeEach(func() {
				testAppGUID = generateGUID()
				testAppEnvSecretName = generateAppEnvSecretName(testAppGUID)
				testAppEnvSecret = AppEnvVarsRecord{
					AppGUID:              testAppGUID,
					SpaceGUID:            defaultNamespace,
					EnvironmentVariables: map[string]string{"foo": "foo", "bar": "bar"},
				}

				returnedAppEnvVarsRecord, returnedErr = appRepo.CreateAppEnvironmentVariables(context.Background(), client, testAppEnvSecret)
			})

			AfterEach(func() {
				lookupSecretK8sResource := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testAppEnvSecretName,
						Namespace: defaultNamespace,
					},
				}
				Expect(
					client.Delete(context.Background(), &lookupSecretK8sResource),
				).To(Succeed(), "Could not clean up the created App Env Secret")
			})

			It("returns a record matching the input and no error", func() {
				Expect(returnedAppEnvVarsRecord.AppGUID).To(Equal(testAppEnvSecret.AppGUID))
				Expect(returnedAppEnvVarsRecord.SpaceGUID).To(Equal(testAppEnvSecret.SpaceGUID))
				Expect(len(returnedAppEnvVarsRecord.EnvironmentVariables)).To(Equal(len(testAppEnvSecret.EnvironmentVariables)))
				Expect(returnedErr).To(BeNil())
			})

			It("returns a record with the created Secret's name", func() {
				Expect(returnedAppEnvVarsRecord.Name).ToNot(BeEmpty())
			})

			It("the App record GUID returned should equal the App GUID provided", func() {
				// Used a strings.Trim to remove characters, which cause the behavior in Issue #103
				testAppEnvSecret.AppGUID = "estringtrimmedguid"

				returnedUpdatedAppEnvVarsRecord, returnedUpdatedErr := appRepo.CreateAppEnvironmentVariables(testCtx, client, testAppEnvSecret)
				Expect(returnedUpdatedErr).ToNot(HaveOccurred())
				Expect(returnedUpdatedAppEnvVarsRecord.AppGUID).To(Equal(testAppEnvSecret.AppGUID), "Expected App GUID to match after transform")
			})

			When("examining the created Secret in the k8s api", func() {
				var (
					createdCFAppSecret corev1.Secret
				)

				BeforeEach(func() {
					cfAppSecretLookupKey := types.NamespacedName{Name: testAppEnvSecretName, Namespace: defaultNamespace}
					createdCFAppSecret = corev1.Secret{}
					Eventually(func() bool {
						err := client.Get(context.Background(), cfAppSecretLookupKey, &createdCFAppSecret)
						if err != nil {
							return false
						}
						return true
					}, 10*time.Second, 250*time.Millisecond).Should(BeTrue(), "could not find the secret created by the repo")
				})

				It("is not empty", func() {
					Expect(createdCFAppSecret).ToNot(Equal(corev1.Secret{}))
				})

				It("has a Name that is derived from the CFApp", func() {
					Expect(createdCFAppSecret.Name).To(Equal(testAppEnvSecretName))
				})

				It("has a label that matches the CFApp GUID", func() {
					labelValue, exists := createdCFAppSecret.Labels[CFAppGUIDLabel]
					Expect(exists).To(BeTrue(), "label for envSecret AppGUID not found")
					Expect(labelValue).To(Equal(testAppGUID))
				})

				It("contains string data that matches the input record length", func() {
					Expect(len(createdCFAppSecret.Data)).To(Equal(len(testAppEnvSecret.EnvironmentVariables)))
				})
			})

			When("the secret already exists", func() {
				BeforeEach(func() {
					testAppEnvSecret.EnvironmentVariables = map[string]string{"foo": "foo", "bar": "bar"}
				})

				It("returns an error if the secret already exists", func() {
					_, err := appRepo.CreateAppEnvironmentVariables(testCtx, client, testAppEnvSecret)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(ContainSubstring("already exists")))
				})
			})
		})
	})

	Describe("GetNamespace", func() {
		When("space does not exist", func() {
			It("returns an unauthorized or not found err", func() {
				_, err := appRepo.FetchNamespace(context.Background(), client, "some-guid")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("Resource not found or permission denied."))
			})
		})
	})

	Describe("SetCurrentDroplet", func() {
		const (
			appGUID     = "the-app-guid"
			dropletGUID = "the-droplet-guid"
			spaceGUID   = "default"
		)

		var (
			appCR     *workloadsv1alpha1.CFApp
			dropletCR workloadsv1alpha1.CFBuild
		)

		BeforeEach(func() {
			appCR = initializeAppCR("some-app", appGUID, spaceGUID)
			dropletCR = initializeDropletCR(dropletGUID, appGUID, spaceGUID)

			Expect(k8sClient.Create(context.Background(), appCR)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), &dropletCR)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), appCR)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), &dropletCR)).To(Succeed())
		})

		When("on the happy path", func() {
			It("returns a CurrentDroplet record", func() {
				record, err := appRepo.SetCurrentDroplet(testCtx, client, SetCurrentDropletMessage{
					AppGUID:     appGUID,
					DropletGUID: dropletGUID,
					SpaceGUID:   spaceGUID,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(record).To(Equal(CurrentDropletRecord{
					AppGUID:     appGUID,
					DropletGUID: dropletGUID,
				}))
			})

			It("sets the spec.current_droplet_ref.name to the Droplet GUID", func() {
				_, err := appRepo.SetCurrentDroplet(testCtx, client, SetCurrentDropletMessage{
					AppGUID:     appGUID,
					DropletGUID: dropletGUID,
					SpaceGUID:   spaceGUID,
				})
				Expect(err).NotTo(HaveOccurred())

				lookupKey := types.NamespacedName{Name: appGUID, Namespace: spaceGUID}
				updatedApp := new(workloadsv1alpha1.CFApp)
				Eventually(func() error {
					return k8sClient.Get(context.Background(), lookupKey, updatedApp)
				}, 10*time.Second, 250*time.Millisecond).ShouldNot(HaveOccurred())
				Expect(updatedApp.Spec.CurrentDropletRef.Name).To(Equal(dropletGUID))
			})
		})

		When("the app doesn't exist", func() {
			It("errors", func() {
				_, err := appRepo.SetCurrentDroplet(testCtx, client, SetCurrentDropletMessage{
					AppGUID:     "no-such-app",
					DropletGUID: dropletGUID,
					SpaceGUID:   spaceGUID,
				})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("not found")))
			})
		})
	})

	Describe("SetDesiredState", func() {
		const (
			appName         = "some-app"
			spaceGUID       = "default"
			appStartedValue = "STARTED"
		)

		var (
			appGUID string
			appCR   *workloadsv1alpha1.CFApp

			returnedAppRecord *AppRecord
			returnedErr       error
		)

		BeforeEach(func() {
			appGUID = generateGUID()
			appCR = initializeAppCR(appName, appGUID, spaceGUID)

			Expect(k8sClient.Create(context.Background(), appCR)).To(Succeed())
		})

		AfterEach(func() {
			k8sClient.Delete(context.Background(), appCR)
		})

		JustBeforeEach(func() {
			appRecord, err := appRepo.SetAppDesiredState(context.Background(), client, SetAppDesiredStateMessage{
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
				Value:     appStartedValue,
			})
			returnedAppRecord = &appRecord
			returnedErr = err
		})

		When("on the happy path", func() {

			It("doesn't return an error", func() {
				Expect(returnedErr).ToNot(HaveOccurred())
			})

			It("returns the updated app record", func() {
				Expect(returnedAppRecord.GUID).To(Equal(appGUID))
				Expect(returnedAppRecord.Name).To(Equal(appName))
				Expect(returnedAppRecord.SpaceGUID).To(Equal(spaceGUID))
				Expect(returnedAppRecord.State).To(Equal(DesiredState("STARTED")))
			})

			It("eventually changes the desired state of the App", func() {
				cfAppLookupKey := types.NamespacedName{Name: appGUID, Namespace: spaceGUID}
				updatedCFApp := new(workloadsv1alpha1.CFApp)
				Eventually(func() string {
					err := k8sClient.Get(context.Background(), cfAppLookupKey, updatedCFApp)
					if err != nil {
						return ""
					}
					return string(updatedCFApp.Spec.DesiredState)
				}, 10*time.Second, 250*time.Millisecond).Should(Equal(appStartedValue))

			})
		})

		When("the app doesn't exist", func() {
			BeforeEach(func() {
				appGUID = "fake-app-guid"
			})
			It("returns an error", func() {
				Expect(returnedErr).To(HaveOccurred())
			})
		})
	})

})
