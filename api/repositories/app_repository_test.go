package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

const (
	CFAppRevisionKey   = "workloads.cloudfoundry.org/app-rev"
	CFAppRevisionValue = "1"
	CFAppStoppedState  = "STOPPED"
)

var _ = Describe("AppRepository", func() {
	var (
		testCtx context.Context
		appRepo *AppRepo
		org     *v1alpha2.SubnamespaceAnchor
		space   *v1alpha2.SubnamespaceAnchor
		cfApp   *workloadsv1alpha1.CFApp
	)

	BeforeEach(func() {
		testCtx = context.Background()

		appRepo = NewAppRepo(namespaceRetriever, userClientFactory, nsPerms)

		org = createOrgWithCleanup(testCtx, prefixedGUID("org"))
		space = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space1"))

		cfApp = createApp(space.Name)
	})

	Describe("GetApp", func() {
		var (
			appGUID string
			app     AppRecord
			getErr  error
		)

		BeforeEach(func() {
			appGUID = cfApp.Name
		})

		JustBeforeEach(func() {
			app, getErr = appRepo.GetApp(testCtx, authInfo, appGUID)
		})

		When("authorized in the space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), cfApp)).To(Succeed())
			})

			It("can fetch the AppRecord CR we're looking for", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(app.GUID).To(Equal(cfApp.Name))
				Expect(app.EtcdUID).To(Equal(cfApp.GetUID()))
				Expect(app.Revision).To(Equal(CFAppRevisionValue))
				Expect(app.Name).To(Equal(cfApp.Spec.Name))
				Expect(app.SpaceGUID).To(Equal(space.Name))
				Expect(app.State).To(Equal(DesiredState("STOPPED")))
				Expect(app.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
				Expect(app.Lifecycle).To(Equal(Lifecycle{
					Type: string(cfApp.Spec.Lifecycle.Type),
					Data: LifecycleData{
						Buildpacks: cfApp.Spec.Lifecycle.Data.Buildpacks,
						Stack:      cfApp.Spec.Lifecycle.Data.Stack,
					},
				}))
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("duplicate Apps exist across namespaces with the same GUIDs", func() {
			BeforeEach(func() {
				space2 := createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space2"))
				createAppWithGUID(space2.Name, appGUID)
			})

			It("returns an error", func() {
				Expect(getErr).To(HaveOccurred())
				Expect(getErr).To(MatchError("get-app duplicate records exist"))
			})
		})

		When("the app guid is not found", func() {
			BeforeEach(func() {
				appGUID = "does-not-exist"
			})

			It("returns an error", func() {
				Expect(getErr).To(HaveOccurred())
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("GetAppByNameAndSpace", func() {
		var (
			appRecord      AppRecord
			getErr         error
			querySpaceName string
		)

		BeforeEach(func() {
			querySpaceName = space.Name
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), cfApp)).To(Succeed())
		})

		JustBeforeEach(func() {
			appRecord, getErr = appRepo.GetAppByNameAndSpace(context.Background(), authInfo, cfApp.Spec.Name, querySpaceName)
		})

		When("the user is able to get apps in the space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceManagerRole.Name, querySpaceName)
			})

			It("returns the record", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(appRecord.Name).To(Equal(cfApp.Spec.Name))
				Expect(appRecord.GUID).To(Equal(cfApp.Name))
				Expect(appRecord.EtcdUID).To(Equal(cfApp.UID))
				Expect(appRecord.SpaceGUID).To(Equal(space.Name))
				Expect(appRecord.State).To(BeEquivalentTo(cfApp.Spec.DesiredState))
				Expect(appRecord.Lifecycle.Type).To(BeEquivalentTo(cfApp.Spec.Lifecycle.Type))
			})
		})

		When("the user is not authorized in the space at all", func() {
			It("returns a forbidden error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				Expect(getErr.(apierrors.ForbiddenError).ResourceType()).To(Equal(SpaceResourceType))
			})
		})

		When("the App doesn't exist in the Space (but is in another Space)", func() {
			BeforeEach(func() {
				space2 := createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space2"))
				querySpaceName = space2.Name
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, querySpaceName)
			})

			It("returns a NotFoundError", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("ListApps", func() {
		var (
			message ListAppsMessage
			appList []AppRecord
			cfApp2  *workloadsv1alpha1.CFApp
		)

		BeforeEach(func() {
			message = ListAppsMessage{}

			space2 := createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space2"))
			space3 := createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space3"))
			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space2.Name)

			cfApp2 = createApp(space2.Name)
			createApp(space3.Name)
		})

		JustBeforeEach(func() {
			var err error
			appList, err = appRepo.ListApps(testCtx, authInfo, message)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns all the AppRecord CRs where client has permission", func() {
			Expect(appList).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp.Name)}),
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp2.Name)}),
			))

			sortedByName := sort.SliceIsSorted(appList, func(i, j int) bool {
				return appList[i].Name < appList[j].Name
			})

			Expect(sortedByName).To(BeTrue(), fmt.Sprintf("AppList was not sorted by Name : App1 : %s , App2: %s", appList[0].Name, appList[1].Name))
		})

		When("there are apps in non-cf namespaces", func() {
			var nonCFApp *workloadsv1alpha1.CFApp

			BeforeEach(func() {
				nonCFNamespace := prefixedGUID("non-cf")
				Expect(k8sClient.Create(
					testCtx,
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nonCFNamespace}},
				)).To(Succeed())

				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, nonCFNamespace)
				nonCFApp = createApp(nonCFNamespace)
			})

			It("does not list them", func() {
				Expect(appList).NotTo(ContainElement(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(nonCFApp.Name)}),
				))
			})
		})

		Describe("filtering", func() {
			var cfApp12 *workloadsv1alpha1.CFApp

			BeforeEach(func() {
				cfApp12 = createApp(space.Name)
			})

			Describe("filtering by name", func() {
				When("no Apps exist that match the filter", func() {
					BeforeEach(func() {
						message = ListAppsMessage{Names: []string{"some-other-app"}}
					})

					It("returns an empty list of apps", func() {
						Expect(appList).To(BeEmpty())
					})
				})

				When("some Apps match the filter", func() {
					BeforeEach(func() {
						message = ListAppsMessage{Names: []string{cfApp2.Spec.Name, cfApp12.Spec.Name}}
					})

					It("returns the matching apps", func() {
						Expect(appList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp2.Name)}),
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp12.Name)}),
						))
					})
				})
			})

			Describe("filtering by guid", func() {
				When("no Apps exist that match the filter", func() {
					BeforeEach(func() {
						message = ListAppsMessage{Guids: []string{"some-other-app-guid"}}
					})

					It("returns an empty list of apps", func() {
						Expect(appList).To(BeEmpty())
					})
				})

				When("some Apps match the filter", func() {
					BeforeEach(func() {
						message = ListAppsMessage{Guids: []string{cfApp.Name, cfApp2.Name}}
					})

					It("returns the matching apps", func() {
						Expect(appList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp.Name)}),
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp2.Name)}),
						))
					})
				})
			})

			Describe("filtering by space", func() {
				When("no Apps exist that match the filter", func() {
					BeforeEach(func() {
						message = ListAppsMessage{SpaceGuids: []string{"some-other-space-guid"}}
					})

					It("returns an empty list of apps", func() {
						Expect(appList).To(BeEmpty())
					})
				})

				When("some Apps match the filter", func() {
					BeforeEach(func() {
						message = ListAppsMessage{SpaceGuids: []string{space.Name}}
					})

					It("returns the matching apps", func() {
						Expect(appList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp.Name)}),
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp12.Name)}),
						))
					})
				})
			})

			Describe("filtering by both name and space", func() {
				When("no Apps exist that match the union of the filters", func() {
					BeforeEach(func() {
						message = ListAppsMessage{Names: []string{cfApp.Spec.Name}, SpaceGuids: []string{"some-other-space-guid"}}
					})

					When("an App matches by Name but not by Space", func() {
						It("returns an empty list of apps", func() {
							Expect(appList).To(BeEmpty())
						})
					})

					When("an App matches by Space but not by Name", func() {
						BeforeEach(func() {
							message = ListAppsMessage{Names: []string{"fake-app-name"}, SpaceGuids: []string{space.Name}}
						})

						It("returns an empty list of apps", func() {
							Expect(appList).To(BeEmpty())
						})
					})
				})

				When("some Apps match the union of the filters", func() {
					BeforeEach(func() {
						message = ListAppsMessage{Names: []string{cfApp12.Spec.Name}, SpaceGuids: []string{space.Name}}
					})

					It("returns the matching apps", func() {
						Expect(appList).To(HaveLen(1))
						Expect(appList[0].GUID).To(Equal(cfApp12.Name))
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
			appCreateMessage CreateAppMessage
			spaceGUID        string
		)

		BeforeEach(func() {
			spaceGUID = generateGUID()

			Expect(k8sClient.Create(testCtx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: spaceGUID},
			})).To(Succeed())

			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, spaceGUID)

			appCreateMessage = initializeAppCreateMessage(testAppName, spaceGUID)
		})

		AfterEach(func() {
			err := k8sClient.Delete(testCtx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: spaceGUID},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a new app CR", func() {
			createdAppRecord, err := appRepo.CreateApp(testCtx, authInfo, appCreateMessage)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdAppRecord).NotTo(BeNil())

			cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: spaceGUID}
			createdCFApp := new(workloadsv1alpha1.CFApp)
			Eventually(func() error {
				return k8sClient.Get(context.Background(), cfAppLookupKey, createdCFApp)
			}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
		})

		It("returns an AppRecord with correct fields", func() {
			createdAppRecord, err := appRepo.CreateApp(context.Background(), authInfo, appCreateMessage)
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
				createdAppRecord, err := appRepo.CreateApp(testCtx, authInfo, appCreateMessage)
				Expect(err).NotTo(HaveOccurred())
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
				createdAppRecord, err := appRepo.CreateApp(testCtx, authInfo, appCreateMessage)
				Expect(err).NotTo(HaveOccurred())
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

	Describe("PatchAppEnvVars", func() {
		const (
			key0 = "KEY0"
			key1 = "KEY1"
			key2 = "KEY2"
		)

		var (
			testAppGUID              string
			testAppEnvSecretName     string
			originalEnvVars          map[string]string
			requestEnvVars           map[string]*string
			expectedEnvVars          map[string]string
			testAppEnvSecretPatchMsg PatchAppEnvVarsMessage
			secretRecord             AppEnvVarsRecord
			patchErr                 error
		)

		BeforeEach(func() {
			testAppGUID = generateGUID()
			testAppEnvSecretName = generateAppEnvSecretName(testAppGUID)

			originalEnvVars = map[string]string{
				key0: "VAL0",
				key1: "original-value",
			}

			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      GenerateEnvSecretName(testAppGUID),
					Namespace: space.Name,
				},
				StringData: originalEnvVars,
			}
			Expect(k8sClient.Create(testCtx, &secret)).To(Succeed())

			var value1 *string
			value2 := "VAL2"

			requestEnvVars = map[string]*string{
				key1: value1,
				key2: &value2,
			}
			testAppEnvSecretPatchMsg = PatchAppEnvVarsMessage{
				AppGUID:              testAppGUID,
				SpaceGUID:            space.Name,
				EnvironmentVariables: requestEnvVars,
			}

			expectedEnvVars = map[string]string{
				key0: originalEnvVars[key0],
				key2: *requestEnvVars[key2],
			}
		})

		JustBeforeEach(func() {
			secretRecord, patchErr = appRepo.PatchAppEnvVars(context.Background(), authInfo, testAppEnvSecretPatchMsg)
		})

		When("the user is authorized and an app exists with a secret", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns the updated secret record", func() {
				Expect(patchErr).NotTo(HaveOccurred())
				Expect(secretRecord.EnvironmentVariables).To(Equal(expectedEnvVars))
			})

			It("eventually patches the underlying secret", func() {
				cfAppSecretLookupKey := types.NamespacedName{Name: testAppEnvSecretName, Namespace: space.Name}

				var updatedSecret corev1.Secret
				Eventually(func() map[string][]byte {
					err := k8sClient.Get(context.Background(), cfAppSecretLookupKey, &updatedSecret)
					if err != nil {
						return map[string][]byte{}
					}
					return updatedSecret.Data
				}, timeCheckThreshold*time.Second).Should(HaveKey(key2))

				Expect(updatedSecret.Data).To(HaveLen(len(expectedEnvVars)))
				Expect(updatedSecret.Data).To(HaveKey(key0))
				Expect(string(updatedSecret.Data[key0])).To(Equal(expectedEnvVars[key0]))
				Expect(updatedSecret.Data).NotTo(HaveKey(key1))
				Expect(updatedSecret.Data).To(HaveKey(key2))
				Expect(string(updatedSecret.Data[key2])).To(Equal(expectedEnvVars[key2]))
			})
		})

		When("the user is not authorized", func() {
			It("return a forbidden error", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("CreateOrPatchAppEnvVars", func() {
		const (
			testAppName = "some-app-name"
			key1        = "KEY1"
			key2        = "KEY2"
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
			cfAppCR = createAppCR(testCtx, k8sClient, testAppName, testAppGUID, space.Name, CFAppStoppedState)

			testAppEnvSecretName = generateAppEnvSecretName(testAppGUID)
			requestEnvVars = map[string]string{
				key1: "VAL1",
				key2: "VAL2",
			}
			testAppEnvSecret = CreateOrPatchAppEnvVarsMessage{
				AppGUID:              testAppGUID,
				AppEtcdUID:           cfAppCR.GetUID(),
				SpaceGUID:            space.Name,
				EnvironmentVariables: requestEnvVars,
			}
		})

		JustBeforeEach(func() {
			returnedAppEnvVarsRecord, returnedErr = appRepo.CreateOrPatchAppEnvVars(context.Background(), authInfo, testAppEnvSecret)
		})

		When("the user is authorized", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
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

					returnedUpdatedAppEnvVarsRecord, returnedUpdatedErr := appRepo.CreateOrPatchAppEnvVars(testCtx, authInfo, testAppEnvSecret)
					Expect(returnedUpdatedErr).ToNot(HaveOccurred())
					Expect(returnedUpdatedAppEnvVarsRecord.AppGUID).To(Equal(testAppEnvSecret.AppGUID), "Expected App GUID to match after transform")
				})

				It("eventually creates a secret that matches the request record", func() {
					cfAppSecretLookupKey := types.NamespacedName{Name: testAppEnvSecretName, Namespace: space.Name}
					createdCFAppSecret := &corev1.Secret{}
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), cfAppSecretLookupKey, createdCFAppSecret)
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
							Namespace: space.Name,
							Labels: map[string]string{
								CFAppGUIDLabel: testAppGUID,
							},
						},
						StringData: originalEnvVars,
					}
					Expect(
						k8sClient.Create(context.Background(), &originalSecret),
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
					cfAppSecretLookupKey := types.NamespacedName{Name: testAppEnvSecretName, Namespace: space.Name}

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

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(returnedErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
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
			dropletCR *workloadsv1alpha1.CFBuild
		)

		BeforeEach(func() {
			beforeCtx := context.Background()
			appCR = createAppCR(beforeCtx, k8sClient, "some-app", appGUID, spaceGUID, CFAppStoppedState)
			dropletCR = createDropletCR(beforeCtx, k8sClient, dropletGUID, appGUID, spaceGUID)
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), appCR)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), dropletCR)).To(Succeed())
		})

		When("user has the space developer role", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, spaceGUID)
			})

			It("returns a CurrentDroplet record", func() {
				record, err := appRepo.SetCurrentDroplet(testCtx, authInfo, SetCurrentDropletMessage{
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
				_, err := appRepo.SetCurrentDroplet(testCtx, authInfo, SetCurrentDropletMessage{
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

			When("the app doesn't exist", func() {
				It("errors", func() {
					_, err := appRepo.SetCurrentDroplet(testCtx, authInfo, SetCurrentDropletMessage{
						AppGUID:     "no-such-app",
						DropletGUID: dropletGUID,
						SpaceGUID:   spaceGUID,
					})
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(ContainSubstring("not found")))
				})
			})
		})

		When("the user is not authorized", func() {
			It("errors", func() {
				_, err := appRepo.SetCurrentDroplet(testCtx, authInfo, SetCurrentDropletMessage{
					AppGUID:     appGUID,
					DropletGUID: dropletGUID,
					SpaceGUID:   spaceGUID,
				})
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
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
			appGUID           string
			returnedAppRecord *AppRecord
			returnedErr       error
			initialAppState   string
			desiredAppState   string
		)

		BeforeEach(func() {
			initialAppState = appStartedValue
			desiredAppState = appStartedValue
		})

		JustBeforeEach(func() {
			beforeCtx := context.Background()
			appGUID = generateGUID()
			_ = createAppCR(beforeCtx, k8sClient, appName, appGUID, spaceGUID, initialAppState)
			appRecord, err := appRepo.SetAppDesiredState(beforeCtx, authInfo, SetAppDesiredStateMessage{
				AppGUID:      appGUID,
				SpaceGUID:    spaceGUID,
				DesiredState: desiredAppState,
			})
			returnedAppRecord = &appRecord
			returnedErr = err
		})

		When("the user has permission to set the app state", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, spaceGUID)
			})

			When("starting an app", func() {
				BeforeEach(func() {
					initialAppState = appStoppedValue
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
				BeforeEach(func() {
					desiredAppState = appStoppedValue
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
					_, err := appRepo.SetAppDesiredState(context.Background(), authInfo, SetAppDesiredStateMessage{
						AppGUID:      "fake-app-guid",
						SpaceGUID:    spaceGUID,
						DesiredState: appStartedValue,
					})

					Expect(err).To(MatchError(ContainSubstring("\"fake-app-guid\" not found")))
				})
			})
		})

		When("not allowed to set the application state", func() {
			It("returns a forbidden error", func() {
				Expect(returnedErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("DeleteApp", func() {
		var appGUID string

		BeforeEach(func() {
			appGUID = generateGUID()
			_ = createAppCR(context.Background(), k8sClient, "some-app", appGUID, space.Name, CFAppStoppedState)
			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
		})

		When("on the happy path", func() {
			It("deletes the CFApp resource", func() {
				err := appRepo.DeleteApp(testCtx, authInfo, DeleteAppMessage{
					AppGUID:   appGUID,
					SpaceGUID: space.Name,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("the app doesn't exist", func() {
			It("errors", func() {
				err := appRepo.DeleteApp(testCtx, authInfo, DeleteAppMessage{
					AppGUID:   "no-such-app",
					SpaceGUID: space.Name,
				})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("not found")))
			})
		})
	})

	Describe("GetAppEnv", func() {
		var (
			envVars      map[string]string
			secretName   string
			appGUID      string
			appEnv       map[string]string
			getAppEnvErr error
		)

		BeforeEach(func() {
			appGUID = cfApp.Name
			secretName = "the-env-secret"

			envVars = map[string]string{
				"RAILS_ENV": "production",
				"LUNCHTIME": "12:00",
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: space.Name,
				},
				StringData: envVars,
			}

			Expect(
				k8sClient.Create(context.Background(), secret),
			).To(Succeed())

			appRepo = NewAppRepo(namespaceRetriever, userClientFactory, nsPerms)
		})

		JustBeforeEach(func() {
			ogCFApp := cfApp.DeepCopy()
			cfApp.Spec.EnvSecretName = secretName
			Expect(
				k8sClient.Patch(context.Background(), cfApp, client.MergeFrom(ogCFApp)),
			).To(Succeed())

			appEnv, getAppEnvErr = appRepo.GetAppEnv(testCtx, authInfo, appGUID)
		})

		When("the user can read secrets in the space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns the env vars stored on the secret", func() {
				Expect(getAppEnvErr).NotTo(HaveOccurred())
				Expect(appEnv).To(Equal(envVars))
			})

			When("the EnvSecret doesn't exist", func() {
				BeforeEach(func() {
					secretName = "doIReallyExist"
				})

				It("errors", func() {
					Expect(getAppEnvErr).To(MatchError(ContainSubstring("Secret")))
				})
			})
		})

		When("EnvSecretName is blank", func() {
			BeforeEach(func() {
				secretName = ""
			})

			It("returns an empty map", func() {
				Expect(appEnv).To(BeEmpty())
			})
		})

		When("the user doesn't have permission to get secrets in the space", func() {
			It("errors", func() {
				Expect(getAppEnvErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the app does not exist", func() {
			BeforeEach(func() {
				appGUID = "i don't exist"
			})
			It("returns an error", func() {
				_, err := appRepo.GetAppEnv(testCtx, authInfo, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
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
			Annotations: map[string]string{
				CFAppRevisionKey: CFAppRevisionValue,
			},
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
