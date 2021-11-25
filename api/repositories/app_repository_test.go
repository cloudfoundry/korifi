package repositories_test

import (
	"context"
	"errors"
	"time"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("AppRepository", func() {
	var (
		testCtx                context.Context
		appRepo                *AppRepo
		testClient             client.Client
		rootNamespace          string
		org                    *v1alpha2.SubnamespaceAnchor
		space1, space2, space3 *v1alpha2.SubnamespaceAnchor
		cfApp1, cfApp2, cfApp3 *workloadsv1alpha1.CFApp
	)

	BeforeEach(func() {
		testCtx = context.Background()

		var err error
		testClient, err = BuildPrivilegedClient(k8sConfig, "")
		appRepo = NewAppRepo(testClient)
		Expect(err).ToNot(HaveOccurred())

		rootNamespace = generateGUID()
		rootNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}}
		Expect(k8sClient.Create(testCtx, rootNs)).To(Succeed())

		org = createOrgAnchorAndNamespace(testCtx, rootNamespace, generateGUID())
		space1 = createSpaceAnchorAndNamespace(testCtx, org.Name, generateGUID())
		space2 = createSpaceAnchorAndNamespace(testCtx, org.Name, generateGUID())
		space3 = createSpaceAnchorAndNamespace(testCtx, org.Name, generateGUID())
	})

	Describe("FetchApp", func() {
		When("on the happy path", func() {
			BeforeEach(func() {
				cfApp1 = createApp(space1.Name)
				cfApp2 = createApp(space2.Name)
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), cfApp1)).To(Succeed())
				Expect(k8sClient.Delete(context.Background(), cfApp2)).To(Succeed())
			})

			It("can fetch the AppRecord CR we're looking for", func() {
				app, err := appRepo.FetchApp(testCtx, testClient, cfApp2.Name)
				Expect(err).NotTo(HaveOccurred())

				Expect(app.GUID).To(Equal(cfApp2.Name))
				Expect(app.EtcdUID).To(Equal(cfApp2.GetUID()))
				Expect(app.Name).To(Equal(cfApp2.Spec.Name))
				Expect(app.SpaceGUID).To(Equal(space2.Name))
				Expect(app.State).To(Equal(DesiredState("STOPPED")))
				Expect(app.DropletGUID).To(Equal(cfApp2.Spec.CurrentDropletRef.Name))
				Expect(app.Lifecycle).To(Equal(Lifecycle{
					Type: string(cfApp2.Spec.Lifecycle.Type),
					Data: LifecycleData{
						Buildpacks: cfApp2.Spec.Lifecycle.Data.Buildpacks,
						Stack:      cfApp2.Spec.Lifecycle.Data.Stack,
					},
				}))
			})
		})

		When("duplicate Apps exist across namespaces with the same GUIDs", func() {
			BeforeEach(func() {
				cfApp1 = createAppWithGUID(space1.Name, "test-guid")
				cfApp2 = createAppWithGUID(space2.Name, "test-guid")
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), cfApp1)).To(Succeed())
				Expect(k8sClient.Delete(context.Background(), cfApp2)).To(Succeed())
			})

			It("returns an error", func() {
				_, err := appRepo.FetchApp(testCtx, testClient, "test-guid")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("duplicate apps exist"))
			})
		})

		When("no Apps exist", func() {
			It("returns an error", func() {
				_, err := appRepo.FetchApp(testCtx, testClient, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(NotFoundError{ResourceType: "App"}))
			})
		})
	})

	Describe("FetchAppByNameAndSpace", func() {
		BeforeEach(func() {
			cfApp1 = createApp(space1.Name)
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), cfApp1)).To(Succeed())
		})

		When("the App exists in the Space", func() {
			It("returns the record", func() {
				appRecord, err := appRepo.FetchAppByNameAndSpace(context.Background(), testClient, cfApp1.Spec.Name, space1.Name)
				Expect(err).NotTo(HaveOccurred())

				Expect(appRecord.Name).To(Equal(cfApp1.Spec.Name))
				Expect(appRecord.GUID).To(Equal(cfApp1.Name))
				Expect(appRecord.EtcdUID).To(Equal(cfApp1.UID))
				Expect(appRecord.SpaceGUID).To(Equal(space1.Name))
				Expect(appRecord.State).To(BeEquivalentTo(cfApp1.Spec.DesiredState))
				Expect(appRecord.Lifecycle.Type).To(BeEquivalentTo(cfApp1.Spec.Lifecycle.Type))
				// Expect(appRecord.CreatedAt).To(Equal()) "",
			})

			When("the App doesn't exist in the Space (but is in another Space)", func() {
				It("returns a NotFoundError", func() {
					_, err := appRepo.FetchAppByNameAndSpace(context.Background(), testClient, cfApp1.Spec.Name, space2.Name)
					Expect(err).To(MatchError(NotFoundError{ResourceType: "App"}))
				})
			})
		})
	})

	Describe("FetchAppList", Serial, func() {
		var (
			message                   AppListMessage
			userName                  string
			spaceDeveloperClusterRole *rbacv1.ClusterRole
		)

		BeforeEach(func() {
			userName = generateGUID()
			message = AppListMessage{}

			var cfAppList workloadsv1alpha1.CFAppList
			Expect(
				testClient.List(context.Background(), &cfAppList),
			).To(Succeed())

			for _, app := range cfAppList.Items {
				Expect(
					testClient.Delete(context.Background(), &app),
				).To(Succeed())
			}

			cert, key := obtainClientCert(userName)
			authHeader := "clientCert " + encodeCertAndKey(cert, key)

			var err error
			testClient, err = BuildUserClient(k8sConfig, authHeader)
			Expect(err).NotTo(HaveOccurred())

			spaceDeveloperClusterRole = createSpaceDeveloperClusterRole(testCtx)
			createRoleBinding(testCtx, userName, spaceDeveloperClusterRole.Name, space1.Name)
			createRoleBinding(testCtx, userName, spaceDeveloperClusterRole.Name, space2.Name)
		})

		Describe("when filters are NOT provided", func() {
			When("no Apps exist", func() {
				It("returns an empty list of apps", func() {
					appList, err := appRepo.FetchAppList(testCtx, testClient, message)
					Expect(err).NotTo(HaveOccurred())
					Expect(appList).To(BeEmpty())
				})
			})

			When("multiple Apps exist", func() {
				BeforeEach(func() {
					cfApp1 = createApp(space1.Name)
					cfApp2 = createApp(space2.Name)
					createApp(space3.Name)
				})

				It("returns all the AppRecord CRs where client has permission", func() {
					appList, err := appRepo.FetchAppList(testCtx, testClient, message)
					Expect(err).NotTo(HaveOccurred())
					Expect(appList).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp1.Name)}),
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp2.Name)}),
					))
				})
			})
		})

		Describe("when filters are provided", func() {
			When("a name filter is provided", func() {
				When("no Apps exist that match the filter", func() {
					BeforeEach(func() {
						createApp(space1.Name)
						createApp(space2.Name)
					})

					It("returns an empty list of apps", func() {
						message = AppListMessage{Names: []string{"some-other-app"}}
						appList, err := appRepo.FetchAppList(testCtx, testClient, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(BeEmpty())
					})
				})

				When("some Apps match the filter", func() {
					BeforeEach(func() {
						createApp(space1.Name)
						cfApp2 = createApp(space2.Name)
						cfApp3 = createApp(space1.Name)
					})

					It("returns the matching apps", func() {
						message = AppListMessage{Names: []string{cfApp2.Spec.Name, cfApp3.Spec.Name}}
						appList, err := appRepo.FetchAppList(testCtx, testClient, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp2.Name)}),
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp3.Name)}),
						))
					})
				})
			})

			When("a space filter is provided", func() {
				When("no Apps exist that match the filter", func() {
					BeforeEach(func() {
						createApp(space1.Name)
						createApp(space1.Name)
					})

					It("returns an empty list of apps", func() {
						message = AppListMessage{SpaceGuids: []string{"some-other-space-guid"}}
						appList, err := appRepo.FetchAppList(testCtx, testClient, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(BeEmpty())
					})
				})

				When("some Apps match the filter", func() {
					BeforeEach(func() {
						createApp(space1.Name)
						cfApp2 = createApp(space2.Name)
						cfApp3 = createApp(space2.Name)
					})

					It("returns the matching apps", func() {
						message = AppListMessage{SpaceGuids: []string{space2.Name}}
						appList, err := appRepo.FetchAppList(testCtx, testClient, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp2.Name)}),
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp3.Name)}),
						))
					})
				})
			})

			When("both name and space filters are provided", func() {
				When("no Apps exist that match the union of the filters", func() {
					BeforeEach(func() {
						cfApp1 = createApp(space1.Name)
						cfApp2 = createApp(space1.Name)
					})

					When("an App matches by Name but not by Space", func() {
						It("returns an empty list of apps", func() {
							message = AppListMessage{Names: []string{cfApp1.Spec.Name}, SpaceGuids: []string{"some-other-space-guid"}}
							appList, err := appRepo.FetchAppList(testCtx, testClient, message)
							Expect(err).NotTo(HaveOccurred())
							Expect(appList).To(BeEmpty())
						})
					})

					When("an App matches by Space but not by Name", func() {
						It("returns an empty list of apps", func() {
							message = AppListMessage{Names: []string{"fake-app-name"}, SpaceGuids: []string{space1.Name}}
							appList, err := appRepo.FetchAppList(testCtx, testClient, message)
							Expect(err).NotTo(HaveOccurred())
							Expect(appList).To(BeEmpty())
						})
					})
				})

				When("some Apps match the union of the filters", func() {
					BeforeEach(func() {
						cfApp1 = createApp(space1.Name)
						cfApp2 = createApp(space2.Name)
						cfApp3 = createApp(space2.Name)
					})

					It("returns the matching apps", func() {
						message = AppListMessage{Names: []string{cfApp2.Spec.Name}, SpaceGuids: []string{space2.Name}}
						appList, err := appRepo.FetchAppList(testCtx, testClient, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(HaveLen(1))

						Expect(appList[0].GUID).To(Equal(cfApp2.Name))
					})
				})
			})
		})
	})

	Describe("CreateApp", func() {
		const (
			testAppName = "test-app-name"
		)
		var (
			appCreateMessage AppCreateMessage
			spaceGUID        string
		)

		BeforeEach(func() {
			spaceGUID = generateGUID()

			Expect(k8sClient.Create(testCtx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: spaceGUID},
			})).To(Succeed())

			appCreateMessage = initializeAppCreateMessage(testAppName, spaceGUID)
		})

		AfterEach(func() {
			err := k8sClient.Delete(testCtx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: spaceGUID},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a new app CR", func() {
			createdAppRecord, err := appRepo.CreateApp(testCtx, testClient, appCreateMessage)
			Expect(err).To(BeNil())
			Expect(createdAppRecord).NotTo(BeNil())

			cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: spaceGUID}
			createdCFApp := new(workloadsv1alpha1.CFApp)
			Eventually(func() error {
				return k8sClient.Get(context.Background(), cfAppLookupKey, createdCFApp)
			}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
		})

		It("returns an AppRecord with correct fields", func() {
			createdAppRecord, err := appRepo.CreateApp(context.Background(), testClient, appCreateMessage)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdAppRecord).NotTo(Equal(AppRecord{}))
			Expect(createdAppRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "record GUID was not a 36 character guid")
			Expect(createdAppRecord.SpaceGUID).To(Equal(spaceGUID), "App SpaceGUID in record did not match input")
			Expect(createdAppRecord.Name).To(Equal(testAppName), "App Name in record did not match input")

			recordCreatedTime, err := time.Parse(TimestampFormat, createdAppRecord.CreatedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(recordCreatedTime).To(BeTemporally("~", time.Now(), 2*time.Second))

			recordUpdatedTime, err := time.Parse(TimestampFormat, createdAppRecord.UpdatedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(recordUpdatedTime).To(BeTemporally("~", time.Now(), 2*time.Second))
		})

		When("no environment variables are given", func() {
			BeforeEach(func() {
				appCreateMessage.EnvironmentVariables = nil
			})

			It("creates an empty secret and sets the environment variable secret ref on the App", func() {
				createdAppRecord, err := appRepo.CreateApp(testCtx, testClient, appCreateMessage)
				Expect(err).To(BeNil())
				Expect(createdAppRecord).NotTo(BeNil())

				cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: spaceGUID}
				createdCFApp := new(workloadsv1alpha1.CFApp)
				Eventually(func() error {
					return k8sClient.Get(context.Background(), cfAppLookupKey, createdCFApp)
				}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

				Expect(createdCFApp.Spec.EnvSecretName).NotTo(BeEmpty())

				secretLookupKey := types.NamespacedName{Name: createdCFApp.Spec.EnvSecretName, Namespace: spaceGUID}
				createdSecret := new(corev1.Secret)
				Eventually(func() error {
					return k8sClient.Get(context.Background(), secretLookupKey, createdSecret)
				}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

				Expect(createdSecret.Data).To(BeEmpty())
			})
		})

		When("environment variables are given", func() {
			BeforeEach(func() {
				appCreateMessage.EnvironmentVariables = map[string]string{
					"FOO": "foo",
					"BAR": "bar",
				}
			})

			It("creates an secret for the environment variables and sets the ref on the App", func() {
				createdAppRecord, err := appRepo.CreateApp(testCtx, testClient, appCreateMessage)
				Expect(err).To(BeNil())
				Expect(createdAppRecord).NotTo(BeNil())

				cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: spaceGUID}
				createdCFApp := new(workloadsv1alpha1.CFApp)
				Eventually(func() error {
					return k8sClient.Get(context.Background(), cfAppLookupKey, createdCFApp)
				}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

				Expect(createdCFApp.Spec.EnvSecretName).NotTo(BeEmpty())

				secretLookupKey := types.NamespacedName{Name: createdCFApp.Spec.EnvSecretName, Namespace: spaceGUID}
				createdSecret := new(corev1.Secret)
				Eventually(func() error {
					return k8sClient.Get(context.Background(), secretLookupKey, createdSecret)
				}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

				Expect(createdSecret.Data).To(MatchAllKeys(Keys{
					"FOO": BeEquivalentTo("foo"),
					"BAR": BeEquivalentTo("bar"),
				}))
			})
		})
	})

	Describe("CreateOrPatchAppEnvVars", func() {
		const (
			testAppName      = "some-app-name"
			defaultNamespace = "default"
			key1             = "KEY1"
			key2             = "KEY2"
		)

		var (
			testAppGUID              string
			cfAppCR                  *workloadsv1alpha1.CFApp
			testAppEnvSecretName     string
			requestEnvVars           map[string]string
			testAppEnvSecret         CreateOrPatchAppEnvVarsMessage
			returnedAppEnvVarsRecord AppEnvVarsRecord
			returnedErr              error
		)

		BeforeEach(func() {
			testAppGUID = generateGUID()
			cfAppCR = initializeAppCR(testAppName, testAppGUID, defaultNamespace)
			Expect(
				k8sClient.Create(context.Background(), cfAppCR),
			).To(Succeed())

			testAppEnvSecretName = generateAppEnvSecretName(testAppGUID)
			requestEnvVars = map[string]string{
				key1: "VAL1",
				key2: "VAL2",
			}
			testAppEnvSecret = CreateOrPatchAppEnvVarsMessage{
				AppGUID:              testAppGUID,
				AppEtcdUID:           cfAppCR.GetUID(),
				SpaceGUID:            defaultNamespace,
				EnvironmentVariables: requestEnvVars,
			}
		})

		JustBeforeEach(func() {
			returnedAppEnvVarsRecord, returnedErr = appRepo.CreateOrPatchAppEnvVars(context.Background(), testClient, testAppEnvSecret)
		})

		AfterEach(func() {
			lookupSecretK8sResource := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testAppEnvSecretName,
					Namespace: defaultNamespace,
				},
			}
			Expect(
				testClient.Delete(context.Background(), &lookupSecretK8sResource),
			).To(Succeed(), "Could not clean up the created App Env Secret")
		})

		When("the secret doesn't already exist", func() {
			It("returns a record matching the input and no error", func() {
				Expect(returnedAppEnvVarsRecord.AppGUID).To(Equal(testAppEnvSecret.AppGUID))
				Expect(returnedAppEnvVarsRecord.SpaceGUID).To(Equal(testAppEnvSecret.SpaceGUID))
				Expect(returnedAppEnvVarsRecord.EnvironmentVariables).To(HaveLen(len(testAppEnvSecret.EnvironmentVariables)))
				Expect(returnedErr).To(BeNil())
			})

			It("returns a record with the created Secret's name", func() {
				Expect(returnedAppEnvVarsRecord.Name).ToNot(BeEmpty())
			})

			It("the App record GUID returned should equal the App GUID provided", func() {
				// Used a strings.Trim to remove characters, which cause the behavior in Issue #103
				testAppEnvSecret.AppGUID = "estringtrimmedguid"

				returnedUpdatedAppEnvVarsRecord, returnedUpdatedErr := appRepo.CreateOrPatchAppEnvVars(testCtx, testClient, testAppEnvSecret)
				Expect(returnedUpdatedErr).ToNot(HaveOccurred())
				Expect(returnedUpdatedAppEnvVarsRecord.AppGUID).To(Equal(testAppEnvSecret.AppGUID), "Expected App GUID to match after transform")
			})

			It("eventually creates a secret that matches the request record", func() {
				cfAppSecretLookupKey := types.NamespacedName{Name: testAppEnvSecretName, Namespace: defaultNamespace}
				createdCFAppSecret := &corev1.Secret{}
				Eventually(func() bool {
					err := testClient.Get(context.Background(), cfAppSecretLookupKey, createdCFAppSecret)
					return err == nil
				}, timeCheckThreshold*time.Second, 250*time.Millisecond).Should(BeTrue(), "could not find the secret created by the repo")

				// Secret has an owner reference that points to the App CR
				Expect(createdCFAppSecret.OwnerReferences)
				Expect(createdCFAppSecret.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
					{
						APIVersion: "workloads.cloudfoundry.org/v1alpha1",
						Kind:       "CFApp",
						Name:       cfAppCR.Name,
						UID:        cfAppCR.GetUID(),
					},
				}))

				Expect(createdCFAppSecret).ToNot(BeZero())
				Expect(createdCFAppSecret.Name).To(Equal(testAppEnvSecretName))
				Expect(createdCFAppSecret.Labels).To(HaveKeyWithValue(CFAppGUIDLabel, testAppGUID))
				Expect(createdCFAppSecret.Data).To(HaveLen(len(testAppEnvSecret.EnvironmentVariables)))
			})
		})

		When("the secret does exist", func() {
			const (
				key0              = "KEY0"
				expectedMapLength = 3
			)
			var originalEnvVars map[string]string
			BeforeEach(func() {
				originalEnvVars = map[string]string{
					key0: "VAL0",
					key1: "original-value", // This variable will change after the manifest is applied
				}
				originalSecret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testAppEnvSecretName,
						Namespace: defaultNamespace,
						Labels: map[string]string{
							CFAppGUIDLabel: testAppGUID,
						},
					},
					StringData: originalEnvVars,
				}
				Expect(
					testClient.Create(context.Background(), &originalSecret),
				).To(Succeed())
			})

			It("returns a record matching the input and no error", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
				Expect(returnedAppEnvVarsRecord.AppGUID).To(Equal(testAppEnvSecret.AppGUID))
				Expect(returnedAppEnvVarsRecord.Name).ToNot(BeEmpty())
				Expect(returnedAppEnvVarsRecord.SpaceGUID).To(Equal(testAppEnvSecret.SpaceGUID))
				Expect(returnedAppEnvVarsRecord.EnvironmentVariables).To(Equal(map[string]string{
					key0: originalEnvVars[key0],
					key1: requestEnvVars[key1],
					key2: requestEnvVars[key2],
				}))
			})

			It("eventually creates a secret that matches the request record", func() {
				cfAppSecretLookupKey := types.NamespacedName{Name: testAppEnvSecretName, Namespace: defaultNamespace}

				var updatedSecret corev1.Secret
				Eventually(func() error {
					err := k8sClient.Get(context.Background(), cfAppSecretLookupKey, &updatedSecret)
					if err == nil && len(updatedSecret.Data) != expectedMapLength {
						return errors.New("the data entries in the secret were not updated")
					}
					return err
				}, timeCheckThreshold*time.Second).Should(Succeed())

				Expect(updatedSecret).ToNot(BeZero())
				Expect(updatedSecret.Name).To(Equal(testAppEnvSecretName))
				Expect(updatedSecret.Labels).To(HaveKeyWithValue(CFAppGUIDLabel, testAppGUID))
				Expect(updatedSecret.Data).To(HaveLen(expectedMapLength))
				Expect(updatedSecret.Data).To(HaveKey(key1))
				Expect(string(updatedSecret.Data[key1])).To(Equal(requestEnvVars[key1]))
				Expect(updatedSecret.Data).To(HaveKey(key2))
				Expect(string(updatedSecret.Data[key2])).To(Equal(requestEnvVars[key2]))
				Expect(updatedSecret.Data).To(HaveKey(key0))
				Expect(string(updatedSecret.Data[key0])).To(Equal(originalEnvVars[key0]))
			})
		})
	})

	Describe("GetNamespace", func() {
		When("space does not exist", func() {
			It("returns an unauthorized or not found err", func() {
				_, err := appRepo.FetchNamespace(context.Background(), testClient, "some-guid")
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
				record, err := appRepo.SetCurrentDroplet(testCtx, testClient, SetCurrentDropletMessage{
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
				_, err := appRepo.SetCurrentDroplet(testCtx, testClient, SetCurrentDropletMessage{
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
				_, err := appRepo.SetCurrentDroplet(testCtx, testClient, SetCurrentDropletMessage{
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
			appStoppedValue = "STOPPED"
		)

		var (
			appGUID string
			appCR   *workloadsv1alpha1.CFApp
		)

		When("starting an app", func() {
			var (
				returnedAppRecord *AppRecord
				returnedErr       error
			)

			BeforeEach(func() {
				appGUID = generateGUID()
				appCR = initializeAppCR(appName, appGUID, spaceGUID)
				appCR.Spec.DesiredState = appStoppedValue

				Expect(k8sClient.Create(context.Background(), appCR)).To(Succeed())

				appRecord, err := appRepo.SetAppDesiredState(context.Background(), testClient, SetAppDesiredStateMessage{
					AppGUID:      appGUID,
					SpaceGUID:    spaceGUID,
					DesiredState: appStartedValue,
				})
				returnedAppRecord = &appRecord
				returnedErr = err
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), appCR)).To(Succeed())
			})

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

		When("stopping an app", func() {
			var (
				returnedAppRecord *AppRecord
				returnedErr       error
			)

			BeforeEach(func() {
				appGUID = generateGUID()
				appCR = initializeAppCR(appName, appGUID, spaceGUID)
				appCR.Spec.DesiredState = appStartedValue

				Expect(k8sClient.Create(context.Background(), appCR)).To(Succeed())

				appRecord, err := appRepo.SetAppDesiredState(context.Background(), testClient, SetAppDesiredStateMessage{
					AppGUID:      appGUID,
					SpaceGUID:    spaceGUID,
					DesiredState: appStoppedValue,
				})
				returnedAppRecord = &appRecord
				returnedErr = err
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), appCR)).To(Succeed())
			})

			It("doesn't return an error", func() {
				Expect(returnedErr).ToNot(HaveOccurred())
			})

			It("returns the updated app record", func() {
				Expect(returnedAppRecord.GUID).To(Equal(appGUID))
				Expect(returnedAppRecord.Name).To(Equal(appName))
				Expect(returnedAppRecord.SpaceGUID).To(Equal(spaceGUID))
				Expect(returnedAppRecord.State).To(Equal(DesiredState("STOPPED")))
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
				}, 10*time.Second, 250*time.Millisecond).Should(Equal(appStoppedValue))
			})
		})

		When("the app doesn't exist", func() {
			It("returns an error", func() {
				appGUID = "fake-app-guid"

				_, err := appRepo.SetAppDesiredState(context.Background(), testClient, SetAppDesiredStateMessage{
					AppGUID:      appGUID,
					SpaceGUID:    spaceGUID,
					DesiredState: appStartedValue,
				})

				Expect(err).To(MatchError(ContainSubstring("\"fake-app-guid\" not found")))
			})
		})
	})
})

func createApp(space string) *workloadsv1alpha1.CFApp {
	return createAppWithGUID(space, generateGUID())
}

func createAppWithGUID(space, guid string) *workloadsv1alpha1.CFApp {
	cfApp := &workloadsv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: space,
		},
		Spec: workloadsv1alpha1.CFAppSpec{
			Name:         generateGUID(),
			DesiredState: "STOPPED",
			Lifecycle: workloadsv1alpha1.Lifecycle{
				Type: "buildpack",
				Data: workloadsv1alpha1.LifecycleData{
					Buildpacks: []string{"java"},
				},
			},
			CurrentDropletRef: corev1.LocalObjectReference{
				Name: generateGUID(),
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

	return cfApp
}
