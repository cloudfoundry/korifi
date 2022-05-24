package repositories_test

import (
	"context"
	"fmt"
	"sort"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	. "code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CFAppRevisionKey   = "korifi.cloudfoundry.org/app-rev"
	CFAppRevisionValue = "1"
	CFAppStoppedState  = "STOPPED"
)

var _ = Describe("AppRepository", func() {
	var (
		testCtx context.Context
		appRepo *AppRepo
		org     *v1alpha1.CFOrg
		space   *v1alpha1.CFSpace
		cfApp   *v1alpha1.CFApp
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
				createRoleBinding(testCtx, userName, orgUserRole.Name, org.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("can fetch the AppRecord CR we're looking for", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(app.GUID).To(Equal(cfApp.Name))
				Expect(app.EtcdUID).To(Equal(cfApp.GetUID()))
				Expect(app.Revision).To(Equal(CFAppRevisionValue))
				Expect(app.Name).To(Equal(cfApp.Spec.DisplayName))
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

		JustBeforeEach(func() {
			appRecord, getErr = appRepo.GetAppByNameAndSpace(testCtx, authInfo, cfApp.Spec.DisplayName, querySpaceName)
		})

		When("the user is able to get apps in the space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceManagerRole.Name, querySpaceName)
			})

			It("returns the record", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(appRecord.Name).To(Equal(cfApp.Spec.DisplayName))
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
			cfApp2  *v1alpha1.CFApp
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
			var nonCFApp *v1alpha1.CFApp

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
			var cfApp12 *v1alpha1.CFApp

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
						message = ListAppsMessage{Names: []string{cfApp2.Spec.DisplayName, cfApp12.Spec.DisplayName}}
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
						message = ListAppsMessage{Names: []string{cfApp.Spec.DisplayName}, SpaceGuids: []string{"some-other-space-guid"}}
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
						message = ListAppsMessage{Names: []string{cfApp12.Spec.DisplayName}, SpaceGuids: []string{space.Name}}
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
			createdAppRecord AppRecord
		)

		BeforeEach(func() {
			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)

			appCreateMessage = initializeAppCreateMessage(testAppName, space.Name)
		})

		JustBeforeEach(func() {
			var err error
			createdAppRecord, err = appRepo.CreateApp(testCtx, authInfo, appCreateMessage)
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a new app CR", func() {
			cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: space.Name}
			createdCFApp := new(v1alpha1.CFApp)
			Expect(k8sClient.Get(testCtx, cfAppLookupKey, createdCFApp)).To(Succeed())
		})

		It("returns an AppRecord with correct fields", func() {
			Expect(createdAppRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "record GUID was not a 36 character guid")
			Expect(createdAppRecord.SpaceGUID).To(Equal(space.Name), "App SpaceGUID in record did not match input")
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
				cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: space.Name}
				createdCFApp := new(v1alpha1.CFApp)
				Expect(k8sClient.Get(testCtx, cfAppLookupKey, createdCFApp)).To(Succeed())

				Expect(createdCFApp.Spec.EnvSecretName).NotTo(BeEmpty())

				secretLookupKey := types.NamespacedName{Name: createdCFApp.Spec.EnvSecretName, Namespace: space.Name}
				createdSecret := new(corev1.Secret)
				Expect(k8sClient.Get(testCtx, secretLookupKey, createdSecret)).To(Succeed())
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
				cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: space.Name}
				createdCFApp := new(v1alpha1.CFApp)
				Expect(k8sClient.Get(testCtx, cfAppLookupKey, createdCFApp)).To(Succeed())
				Expect(createdCFApp.Spec.EnvSecretName).NotTo(BeEmpty())

				secretLookupKey := types.NamespacedName{Name: createdCFApp.Spec.EnvSecretName, Namespace: space.Name}
				createdSecret := new(corev1.Secret)
				Expect(k8sClient.Get(testCtx, secretLookupKey, createdSecret)).To(Succeed())
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
			envSecretName string
			secretRecord  AppEnvVarsRecord
			patchErr      error
		)

		BeforeEach(func() {
			envSecretName = generateAppEnvSecretName(cfApp.Name)

			envVars := map[string]string{
				key0: "VAL0",
				key1: "original-value",
			}
			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      GenerateEnvSecretName(cfApp.Name),
					Namespace: space.Name,
				},
				StringData: envVars,
			}
			Expect(k8sClient.Create(testCtx, &secret)).To(Succeed())
		})

		JustBeforeEach(func() {
			var value1 *string
			value2 := "VAL2"

			newEnvVars := map[string]*string{
				key1: value1,
				key2: &value2,
			}
			patchEnvMsg := PatchAppEnvVarsMessage{
				AppGUID:              cfApp.Name,
				SpaceGUID:            space.Name,
				EnvironmentVariables: newEnvVars,
			}

			secretRecord, patchErr = appRepo.PatchAppEnvVars(testCtx, authInfo, patchEnvMsg)
		})

		When("the user is authorized and an app exists with a secret", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns the updated secret record", func() {
				Expect(patchErr).NotTo(HaveOccurred())
				Expect(secretRecord.EnvironmentVariables).To(SatisfyAll(
					HaveLen(2),
					HaveKeyWithValue(key0, "VAL0"),
					HaveKeyWithValue(key2, "VAL2"),
				))
			})

			It("patches the underlying secret", func() {
				cfAppSecretLookupKey := types.NamespacedName{Name: envSecretName, Namespace: space.Name}

				var updatedSecret corev1.Secret
				err := k8sClient.Get(testCtx, cfAppSecretLookupKey, &updatedSecret)
				Expect(err).NotTo(HaveOccurred())

				Expect(asMapOfStrings(updatedSecret.Data)).To(SatisfyAll(
					HaveLen(2),
					HaveKeyWithValue(key0, "VAL0"),
					HaveKeyWithValue(key2, "VAL2"),
				))
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
			envSecretName            string
			envSecret                CreateOrPatchAppEnvVarsMessage
			returnedAppEnvVarsRecord AppEnvVarsRecord
			returnedErr              error
		)

		BeforeEach(func() {
			envSecretName = generateAppEnvSecretName(cfApp.Name)
			env := map[string]string{
				key1: "VAL1",
				key2: "VAL2",
			}
			envSecret = CreateOrPatchAppEnvVarsMessage{
				AppGUID:              cfApp.Name,
				AppEtcdUID:           cfApp.GetUID(),
				SpaceGUID:            space.Name,
				EnvironmentVariables: env,
			}
		})

		JustBeforeEach(func() {
			returnedAppEnvVarsRecord, returnedErr = appRepo.CreateOrPatchAppEnvVars(testCtx, authInfo, envSecret)
		})

		When("the user is authorized", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			When("the secret doesn't already exist", func() {
				It("returns a record matching the input and no error", func() {
					Expect(returnedAppEnvVarsRecord.AppGUID).To(Equal(envSecret.AppGUID))
					Expect(returnedAppEnvVarsRecord.SpaceGUID).To(Equal(envSecret.SpaceGUID))
					Expect(returnedAppEnvVarsRecord.EnvironmentVariables).To(HaveLen(len(envSecret.EnvironmentVariables)))
					Expect(returnedErr).To(BeNil())
				})

				It("returns a record with the created Secret's name", func() {
					Expect(returnedAppEnvVarsRecord.Name).ToNot(BeEmpty())
				})

				It("the App record GUID returned should equal the App GUID provided", func() {
					// Used a strings.Trim to remove characters, which cause the behavior in Issue #103
					envSecret.AppGUID = "estringtrimmedguid"

					returnedUpdatedAppEnvVarsRecord, returnedUpdatedErr := appRepo.CreateOrPatchAppEnvVars(testCtx, authInfo, envSecret)
					Expect(returnedUpdatedErr).ToNot(HaveOccurred())
					Expect(returnedUpdatedAppEnvVarsRecord.AppGUID).To(Equal(envSecret.AppGUID), "Expected App GUID to match after transform")
				})

				It("creates a secret that matches the request record", func() {
					cfAppSecretLookupKey := types.NamespacedName{Name: envSecretName, Namespace: space.Name}
					createdCFAppSecret := &corev1.Secret{}
					Expect(k8sClient.Get(testCtx, cfAppSecretLookupKey, createdCFAppSecret)).To(Succeed())

					// Secret has an owner reference that points to the App CR
					Expect(createdCFAppSecret.OwnerReferences)
					Expect(createdCFAppSecret.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
						{
							APIVersion: "korifi.cloudfoundry.org/v1alpha1",
							Kind:       "CFApp",
							Name:       cfApp.Name,
							UID:        cfApp.GetUID(),
						},
					}))

					Expect(createdCFAppSecret.Name).To(Equal(envSecretName))
					Expect(createdCFAppSecret.Labels).To(HaveKeyWithValue(CFAppGUIDLabel, cfApp.Name))
					Expect(createdCFAppSecret.Data).To(HaveLen(len(envSecret.EnvironmentVariables)))
				})
			})

			When("the secret does exist", func() {
				const (
					key0 = "KEY0"
				)
				var originalEnvVars map[string]string
				BeforeEach(func() {
					originalEnvVars = map[string]string{
						key0: "VAL0",
						key1: "original-value", // This variable will change after the manifest is applied
					}
					originalSecret := corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      envSecretName,
							Namespace: space.Name,
							Labels: map[string]string{
								CFAppGUIDLabel: cfApp.Name,
							},
						},
						StringData: originalEnvVars,
					}
					Expect(
						k8sClient.Create(testCtx, &originalSecret),
					).To(Succeed())
				})

				It("returns a record matching the input and no error", func() {
					Expect(returnedErr).NotTo(HaveOccurred())
					Expect(returnedAppEnvVarsRecord.AppGUID).To(Equal(envSecret.AppGUID))
					Expect(returnedAppEnvVarsRecord.Name).ToNot(BeEmpty())
					Expect(returnedAppEnvVarsRecord.SpaceGUID).To(Equal(envSecret.SpaceGUID))
					Expect(returnedAppEnvVarsRecord.EnvironmentVariables).To(SatisfyAll(
						HaveLen(3),
						HaveKeyWithValue(key0, "VAL0"),
						HaveKeyWithValue(key1, "VAL1"),
						HaveKeyWithValue(key2, "VAL2"),
					))
				})

				It("creates a secret that matches the request record", func() {
					cfAppSecretLookupKey := types.NamespacedName{Name: envSecretName, Namespace: space.Name}

					var updatedSecret corev1.Secret
					err := k8sClient.Get(testCtx, cfAppSecretLookupKey, &updatedSecret)
					Expect(err).NotTo(HaveOccurred())

					Expect(updatedSecret.Name).To(Equal(envSecretName))
					Expect(updatedSecret.Labels).To(HaveKeyWithValue(CFAppGUIDLabel, cfApp.Name))
					Expect(asMapOfStrings(updatedSecret.Data)).To(SatisfyAll(
						HaveLen(3),
						HaveKeyWithValue(key0, "VAL0"),
						HaveKeyWithValue(key1, "VAL1"),
						HaveKeyWithValue(key2, "VAL2"),
					))
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
		var (
			dropletGUID string
			appGUID     string

			currentDropletRecord CurrentDropletRecord
			setDropletErr        error
		)

		BeforeEach(func() {
			dropletGUID = generateGUID()
			appGUID = cfApp.Name
			createDropletCR(testCtx, k8sClient, dropletGUID, cfApp.Name, space.Name)
		})

		JustBeforeEach(func() {
			currentDropletRecord, setDropletErr = appRepo.SetCurrentDroplet(testCtx, authInfo, SetCurrentDropletMessage{
				AppGUID:     appGUID,
				DropletGUID: dropletGUID,
				SpaceGUID:   space.Name,
			})
		})

		When("user has the space developer role", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns a CurrentDroplet record", func() {
				Expect(setDropletErr).NotTo(HaveOccurred())
				Expect(currentDropletRecord).To(Equal(CurrentDropletRecord{
					AppGUID:     cfApp.Name,
					DropletGUID: dropletGUID,
				}))
			})

			It("sets the spec.current_droplet_ref.name to the Droplet GUID", func() {
				lookupKey := types.NamespacedName{Name: cfApp.Name, Namespace: space.Name}
				updatedApp := new(v1alpha1.CFApp)
				Expect(k8sClient.Get(testCtx, lookupKey, updatedApp)).To(Succeed())
				Expect(updatedApp.Spec.CurrentDropletRef.Name).To(Equal(dropletGUID))
			})

			When("the app doesn't exist", func() {
				BeforeEach(func() {
					appGUID = "no-such-app"
				})

				It("errors", func() {
					Expect(setDropletErr).To(MatchError(ContainSubstring("not found")))
				})
			})
		})

		When("the user is not authorized", func() {
			It("errors", func() {
				Expect(setDropletErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("SetDesiredState", func() {
		const (
			appName         = "some-app"
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
			appGUID = generateGUID()
			_ = createAppCR(testCtx, k8sClient, appName, appGUID, space.Name, initialAppState)
			appRecord, err := appRepo.SetAppDesiredState(testCtx, authInfo, SetAppDesiredStateMessage{
				AppGUID:      appGUID,
				SpaceGUID:    space.Name,
				DesiredState: desiredAppState,
			})
			returnedAppRecord = &appRecord
			returnedErr = err
		})

		When("the user has permission to set the app state", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
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
					Expect(returnedAppRecord.SpaceGUID).To(Equal(space.Name))
					Expect(returnedAppRecord.State).To(Equal(DesiredState("STARTED")))
				})

				It("changes the desired state of the App", func() {
					cfAppLookupKey := types.NamespacedName{Name: appGUID, Namespace: space.Name}
					updatedCFApp := new(v1alpha1.CFApp)
					Expect(k8sClient.Get(testCtx, cfAppLookupKey, updatedCFApp)).To(Succeed())
					Expect(string(updatedCFApp.Spec.DesiredState)).To(Equal(appStartedValue))
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
					Expect(returnedAppRecord.SpaceGUID).To(Equal(space.Name))
					Expect(returnedAppRecord.State).To(Equal(DesiredState("STOPPED")))
				})

				It("changes the desired state of the App", func() {
					cfAppLookupKey := types.NamespacedName{Name: appGUID, Namespace: space.Name}
					updatedCFApp := new(v1alpha1.CFApp)
					Expect(k8sClient.Get(testCtx, cfAppLookupKey, updatedCFApp)).To(Succeed())
					Expect(string(updatedCFApp.Spec.DesiredState)).To(Equal(appStoppedValue))
				})
			})

			When("the app doesn't exist", func() {
				It("returns an error", func() {
					_, err := appRepo.SetAppDesiredState(testCtx, authInfo, SetAppDesiredStateMessage{
						AppGUID:      "fake-app-guid",
						SpaceGUID:    space.Name,
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
		var (
			appGUID      string
			deleteAppErr error
		)

		BeforeEach(func() {
			appGUID = cfApp.Name
			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
		})

		JustBeforeEach(func() {
			deleteAppErr = appRepo.DeleteApp(testCtx, authInfo, DeleteAppMessage{
				AppGUID:   appGUID,
				SpaceGUID: space.Name,
			})
		})

		It("deletes the CFApp resource", func() {
			Expect(deleteAppErr).NotTo(HaveOccurred())
			_, err := appRepo.GetApp(testCtx, authInfo, appGUID)
			Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
		})

		When("the app doesn't exist", func() {
			BeforeEach(func() {
				appGUID = "no-such-app"
			})

			It("errors", func() {
				Expect(deleteAppErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
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
				k8sClient.Create(testCtx, secret),
			).To(Succeed())

			appRepo = NewAppRepo(namespaceRetriever, userClientFactory, nsPerms)
		})

		JustBeforeEach(func() {
			ogCFApp := cfApp.DeepCopy()
			cfApp.Spec.EnvSecretName = secretName
			Expect(
				k8sClient.Patch(testCtx, cfApp, client.MergeFrom(ogCFApp)),
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

func createApp(space string) *v1alpha1.CFApp {
	return createAppWithGUID(space, generateGUID())
}

func createAppWithGUID(space, guid string) *v1alpha1.CFApp {
	cfApp := &v1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: space,
			Annotations: map[string]string{
				CFAppRevisionKey: CFAppRevisionValue,
			},
		},
		Spec: v1alpha1.CFAppSpec{
			DisplayName:  generateGUID(),
			DesiredState: "STOPPED",
			Lifecycle: v1alpha1.Lifecycle{
				Type: "buildpack",
				Data: v1alpha1.LifecycleData{
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

func asMapOfStrings(data map[string][]byte) map[string]string {
	result := map[string]string{}

	for k, v := range data {
		result[k] = string(v)
	}

	return result
}
