package repositories_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	. "code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CFAppRevisionKey   = "korifi.cloudfoundry.org/app-rev"
	CFAppRevisionValue = "1"
)

var _ = Describe("AppRepository", func() {
	var (
		testCtx context.Context
		appRepo *AppRepo
		cfOrg   *korifiv1alpha1.CFOrg
		cfSpace *korifiv1alpha1.CFSpace
		cfApp   *korifiv1alpha1.CFApp
	)

	BeforeEach(func() {
		testCtx = context.Background()

		appRepo = NewAppRepo(namespaceRetriever, userClientFactory, nsPerms, conditions.NewConditionAwaiter[*korifiv1alpha1.CFApp, korifiv1alpha1.CFAppList](2*time.Second))

		cfOrg = createOrgWithCleanup(testCtx, prefixedGUID("org"))
		cfSpace = createSpaceWithCleanup(testCtx, cfOrg.Name, prefixedGUID("space1"))

		cfApp = createApp(cfSpace.Name)
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
				createRoleBinding(testCtx, userName, orgUserRole.Name, cfOrg.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("can fetch the AppRecord CR we're looking for", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(app.GUID).To(Equal(cfApp.Name))
				Expect(app.EtcdUID).To(Equal(cfApp.GetUID()))
				Expect(app.Revision).To(Equal(CFAppRevisionValue))
				Expect(app.Name).To(Equal(cfApp.Spec.DisplayName))
				Expect(app.SpaceGUID).To(Equal(cfSpace.Name))
				Expect(app.State).To(Equal(DesiredState("STOPPED")))
				Expect(app.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
				Expect(app.Lifecycle).To(Equal(Lifecycle{
					Type: string(cfApp.Spec.Lifecycle.Type),
					Data: LifecycleData{
						Buildpacks: cfApp.Spec.Lifecycle.Data.Buildpacks,
						Stack:      cfApp.Spec.Lifecycle.Data.Stack,
					},
				}))
				Expect(app.IsStaged).To(BeFalse())
			})

			When("the app has staged condition true", func() {
				BeforeEach(func() {
					cfApp.Status.Conditions = []metav1.Condition{{
						Type:               workloads.StatusConditionStaged,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
						Reason:             "staged",
						Message:            "staged",
					}}
					Expect(k8sClient.Status().Update(testCtx, cfApp)).To(Succeed())
					Eventually(func(g Gomega) {
						app := korifiv1alpha1.CFApp{}
						g.Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(cfApp), &app)).To(Succeed())
						g.Expect(app.Status.Conditions).NotTo(BeEmpty())
					}).Should(Succeed())
				})

				It("sets IsStaged to true", func() {
					Expect(getErr).ToNot(HaveOccurred())
					Expect(app.IsStaged).To(BeTrue())
				})
			})

			When("the app has staged condition false", func() {
				BeforeEach(func() {
					meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
						Type:    workloads.StatusConditionStaged,
						Status:  metav1.ConditionFalse,
						Reason:  "appStaged",
						Message: "",
					})
					Expect(k8sClient.Status().Update(testCtx, cfApp)).To(Succeed())
					Eventually(func(g Gomega) {
						app := korifiv1alpha1.CFApp{}
						g.Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(cfApp), &app)).To(Succeed())
						g.Expect(meta.IsStatusConditionFalse(app.Status.Conditions, workloads.StatusConditionStaged)).To(BeTrue())
					}).Should(Succeed())
				})

				It("sets IsStaged to false", func() {
					Expect(getErr).ToNot(HaveOccurred())
					Expect(app.IsStaged).To(BeFalse())
				})
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("duplicate Apps exist across namespaces with the same GUIDs", func() {
			BeforeEach(func() {
				space2 := createSpaceWithCleanup(testCtx, cfOrg.Name, prefixedGUID("space2"))
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
			querySpaceName = cfSpace.Name
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
				Expect(appRecord.SpaceGUID).To(Equal(cfSpace.Name))
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
				space2 := createSpaceWithCleanup(testCtx, cfOrg.Name, prefixedGUID("space2"))
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
			cfApp2  *korifiv1alpha1.CFApp
		)

		BeforeEach(func() {
			message = ListAppsMessage{}

			space2 := createSpaceWithCleanup(testCtx, cfOrg.Name, prefixedGUID("space2"))
			space3 := createSpaceWithCleanup(testCtx, cfOrg.Name, prefixedGUID("space3"))
			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
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
			var nonCFApp *korifiv1alpha1.CFApp

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
			var cfApp12 *korifiv1alpha1.CFApp

			BeforeEach(func() {
				cfApp12 = createApp(cfSpace.Name)
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
						message = ListAppsMessage{SpaceGuids: []string{cfSpace.Name}}
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
							message = ListAppsMessage{Names: []string{"fake-app-name"}, SpaceGuids: []string{cfSpace.Name}}
						})

						It("returns an empty list of apps", func() {
							Expect(appList).To(BeEmpty())
						})
					})
				})

				When("some Apps match the union of the filters", func() {
					BeforeEach(func() {
						message = ListAppsMessage{Names: []string{cfApp12.Spec.DisplayName}, SpaceGuids: []string{cfSpace.Name}}
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
			createErr        error
		)

		BeforeEach(func() {
			appCreateMessage = initializeAppCreateMessage(testAppName, cfSpace.Name)
		})

		JustBeforeEach(func() {
			createdAppRecord, createErr = appRepo.CreateApp(testCtx, authInfo, appCreateMessage)
		})

		When("authorized in the space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, orgUserRole.Name, cfOrg.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("creates a new app CR", func() {
				Expect(createErr).NotTo(HaveOccurred())
				cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: cfSpace.Name}
				createdCFApp := new(korifiv1alpha1.CFApp)
				Expect(k8sClient.Get(testCtx, cfAppLookupKey, createdCFApp)).To(Succeed())
			})

			It("returns an AppRecord with correct fields", func() {
				Expect(createErr).NotTo(HaveOccurred())
				Expect(createdAppRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"))
				Expect(createdAppRecord.SpaceGUID).To(Equal(cfSpace.Name))
				Expect(createdAppRecord.Name).To(Equal(testAppName))
				Expect(createdAppRecord.Lifecycle.Data.Buildpacks).To(BeEmpty())

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
					Expect(createErr).NotTo(HaveOccurred())
					cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: cfSpace.Name}
					createdCFApp := new(korifiv1alpha1.CFApp)
					Expect(k8sClient.Get(testCtx, cfAppLookupKey, createdCFApp)).To(Succeed())
					Expect(createdCFApp.Spec.EnvSecretName).NotTo(BeEmpty())

					secretLookupKey := types.NamespacedName{Name: createdCFApp.Spec.EnvSecretName, Namespace: cfSpace.Name}
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
					Expect(createErr).NotTo(HaveOccurred())
					cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: cfSpace.Name}
					createdCFApp := new(korifiv1alpha1.CFApp)
					Expect(k8sClient.Get(testCtx, cfAppLookupKey, createdCFApp)).To(Succeed())
					Expect(createdCFApp.Spec.EnvSecretName).NotTo(BeEmpty())

					secretLookupKey := types.NamespacedName{Name: createdCFApp.Spec.EnvSecretName, Namespace: cfSpace.Name}
					createdSecret := new(corev1.Secret)
					Expect(k8sClient.Get(testCtx, secretLookupKey, createdSecret)).To(Succeed())
					Expect(createdSecret.Data).To(MatchAllKeys(Keys{
						"FOO": BeEquivalentTo("foo"),
						"BAR": BeEquivalentTo("bar"),
					}))
				})
			})

			When("buildpacks are given", func() {
				var buildpacks []string

				BeforeEach(func() {
					buildpacks = []string{"buildpack-1", "buildpack-2"}
					appCreateMessage.Lifecycle.Data.Buildpacks = buildpacks
				})

				It("creates a CFApp with the buildpacks set", func() {
					Expect(createErr).NotTo(HaveOccurred())
					cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: cfSpace.Name}
					createdCFApp := new(korifiv1alpha1.CFApp)
					Expect(k8sClient.Get(testCtx, cfAppLookupKey, createdCFApp)).To(Succeed())
					Expect(createdAppRecord.Lifecycle.Data.Buildpacks).To(Equal(buildpacks))
				})

				It("returns an AppRecord with the buildpacks set", func() {
					Expect(createdAppRecord.Lifecycle.Data.Buildpacks).To(Equal(buildpacks))
				})
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("PatchApp", func() {
		var (
			patchedAppRecord AppRecord
			patchErr         error

			appPatchMessage PatchAppMessage
		)

		BeforeEach(func() {
			appPatchMessage = initializeAppPatchMessage(cfApp.Spec.DisplayName, cfApp.Name, cfSpace.Name)
		})

		JustBeforeEach(func() {
			patchedAppRecord, patchErr = appRepo.PatchApp(testCtx, authInfo, appPatchMessage)
		})

		When("authorized in the space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, orgUserRole.Name, cfOrg.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("can patch the AppRecord CR we're looking for", func() {
				Expect(patchErr).NotTo(HaveOccurred())

				Expect(patchedAppRecord.GUID).To(Equal(cfApp.Name))
				Expect(patchedAppRecord.Name).To(Equal(cfApp.Spec.DisplayName))
				Expect(patchedAppRecord.SpaceGUID).To(Equal(cfSpace.Name))
				Expect(patchedAppRecord.State).To(Equal(DesiredState("STOPPED")))
				Expect(patchedAppRecord.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
				Expect(patchedAppRecord.Lifecycle).To(Equal(Lifecycle{
					Type: string(cfApp.Spec.Lifecycle.Type),
					Data: LifecycleData{
						Buildpacks: []string{"some-buildpack"},
						Stack:      "cflinuxfs3",
					},
				}))
				Expect(patchedAppRecord.IsStaged).To(BeFalse())
			})

			When("no environment variables are given", func() {
				BeforeEach(func() {
					appPatchMessage.EnvironmentVariables = nil
				})

				It("creates an empty secret and sets the environment variable secret ref on the App", func() {
					Expect(patchErr).NotTo(HaveOccurred())

					cfAppLookupKey := types.NamespacedName{Name: patchedAppRecord.GUID, Namespace: cfSpace.Name}
					patchedCFApp := new(korifiv1alpha1.CFApp)
					Expect(k8sClient.Get(testCtx, cfAppLookupKey, patchedCFApp)).To(Succeed())
					Expect(patchedCFApp.Spec.EnvSecretName).NotTo(BeEmpty())

					secretLookupKey := types.NamespacedName{Name: patchedCFApp.Spec.EnvSecretName, Namespace: cfSpace.Name}
					createdSecret := new(corev1.Secret)
					Expect(k8sClient.Get(testCtx, secretLookupKey, createdSecret)).To(Succeed())
					Expect(createdSecret.Data).To(BeEmpty())
				})
			})

			When("environment variables are given", func() {
				BeforeEach(func() {
					appPatchMessage.EnvironmentVariables = map[string]string{
						"FOO": "foo",
						"BAR": "bar",
					}
				})

				It("creates an secret for the environment variables and sets the ref on the App", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					cfAppLookupKey := types.NamespacedName{Name: patchedAppRecord.GUID, Namespace: cfSpace.Name}
					patchedCFApp := new(korifiv1alpha1.CFApp)
					Expect(k8sClient.Get(testCtx, cfAppLookupKey, patchedCFApp)).To(Succeed())
					Expect(patchedCFApp.Spec.EnvSecretName).NotTo(BeEmpty())

					secretLookupKey := types.NamespacedName{Name: patchedCFApp.Spec.EnvSecretName, Namespace: cfSpace.Name}
					createdSecret := new(corev1.Secret)
					Expect(k8sClient.Get(testCtx, secretLookupKey, createdSecret)).To(Succeed())
					Expect(createdSecret.Data).To(MatchAllKeys(Keys{
						"FOO": BeEquivalentTo("foo"),
						"BAR": BeEquivalentTo("bar"),
					}))
				})
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user is a Space Manager (i.e. can view apps but not modify them)", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, orgUserRole.Name, cfOrg.Name)
				createRoleBinding(testCtx, userName, spaceManagerRole.Name, cfSpace.Name)
			})

			It("returns a forbidden error", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
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
					Namespace: cfSpace.Name,
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
				SpaceGUID:            cfSpace.Name,
				EnvironmentVariables: newEnvVars,
			}

			secretRecord, patchErr = appRepo.PatchAppEnvVars(testCtx, authInfo, patchEnvMsg)
		})

		When("the user is authorized and an app exists with a secret", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
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
				cfAppSecretLookupKey := types.NamespacedName{Name: envSecretName, Namespace: cfSpace.Name}

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
			key1 = "KEY1"
			key2 = "KEY2"
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
				SpaceGUID:            cfSpace.Name,
				EnvironmentVariables: env,
			}
		})

		JustBeforeEach(func() {
			returnedAppEnvVarsRecord, returnedErr = appRepo.CreateOrPatchAppEnvVars(testCtx, authInfo, envSecret)
		})

		When("the user is authorized", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
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
					cfAppSecretLookupKey := types.NamespacedName{Name: envSecretName, Namespace: cfSpace.Name}
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
							Namespace: cfSpace.Name,
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
					cfAppSecretLookupKey := types.NamespacedName{Name: envSecretName, Namespace: cfSpace.Name}

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

	Describe("PatchAppMetadata", func() {
		var (
			appGUID                       string
			patchErr                      error
			appRecord                     AppRecord
			labelsPatch, annotationsPatch map[string]*string
		)

		BeforeEach(func() {
			appGUID = cfApp.Name
			labelsPatch = nil
			annotationsPatch = nil
		})

		JustBeforeEach(func() {
			patchMsg := PatchAppMetadataMessage{
				AppGUID:   appGUID,
				SpaceGUID: cfSpace.Name,
				MetadataPatch: MetadataPatch{
					Annotations: annotationsPatch,
					Labels:      labelsPatch,
				},
			}

			appRecord, patchErr = appRepo.PatchAppMetadata(testCtx, authInfo, patchMsg)
		})

		When("the user is authorized and an app exists", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			When("the app doesn't have labels or annotations", func() {
				BeforeEach(func() {
					labelsPatch = map[string]*string{
						"key-one": pointerTo("value-one"),
						"key-two": pointerTo("value-two"),
					}
					annotationsPatch = map[string]*string{
						"key-one": pointerTo("value-one"),
						"key-two": pointerTo("value-two"),
					}
					Expect(k8s.PatchResource(ctx, k8sClient, cfApp, func() {
						cfApp.Labels = nil
						cfApp.Annotations = nil
					})).To(Succeed())
				})

				It("returns the updated org record", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					Expect(appRecord.GUID).To(Equal(appGUID))
					Expect(appRecord.SpaceGUID).To(Equal(cfSpace.Name))
					Expect(appRecord.Labels).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
					Expect(appRecord.Annotations).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
				})

				It("sets the k8s CFSpace resource", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					updatedCFApp := new(korifiv1alpha1.CFApp)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), updatedCFApp)).To(Succeed())
					Expect(updatedCFApp.Labels).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
					Expect(updatedCFApp.Annotations).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
				})
			})

			When("the app already has labels and annotations", func() {
				BeforeEach(func() {
					labelsPatch = map[string]*string{
						"key-one":        pointerTo("value-one-updated"),
						"key-two":        pointerTo("value-two"),
						"before-key-two": nil,
					}
					annotationsPatch = map[string]*string{
						"key-one":        pointerTo("value-one-updated"),
						"key-two":        pointerTo("value-two"),
						"before-key-two": nil,
					}
					Expect(k8s.PatchResource(ctx, k8sClient, cfApp, func() {
						cfApp.Labels = map[string]string{
							"before-key-one": "value-one",
							"before-key-two": "value-two",
							"key-one":        "value-one",
						}
						cfApp.Annotations = map[string]string{
							"before-key-one": "value-one",
							"before-key-two": "value-two",
							"key-one":        "value-one",
						}
					})).To(Succeed())
				})

				It("returns the updated app record", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					Expect(appRecord.GUID).To(Equal(cfApp.Name))
					Expect(appRecord.SpaceGUID).To(Equal(cfApp.Namespace))
					Expect(appRecord.State).To(BeEquivalentTo(cfApp.Spec.DesiredState))
					Expect(appRecord.Labels).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
					Expect(appRecord.Annotations).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
				})

				It("sets the k8s cfapp resource", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					updatedCFApp := new(korifiv1alpha1.CFApp)
					Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfApp), updatedCFApp)).To(Succeed())
					Expect(updatedCFApp.Labels).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
					Expect(updatedCFApp.Annotations).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
				})
			})

			When("an annotation is invalid", func() {
				BeforeEach(func() {
					annotationsPatch = map[string]*string{
						"-bad-annotation": pointerTo("stuff"),
					}
				})

				It("returns an UnprocessableEntityError", func() {
					var unprocessableEntityError apierrors.UnprocessableEntityError
					Expect(errors.As(patchErr, &unprocessableEntityError)).To(BeTrue())
					Expect(unprocessableEntityError.Detail()).To(SatisfyAll(
						ContainSubstring("metadata.annotations is invalid"),
						ContainSubstring(`"-bad-annotation"`),
						ContainSubstring("alphanumeric"),
					))
				})
			})

			When("a label is invalid", func() {
				BeforeEach(func() {
					labelsPatch = map[string]*string{
						"-bad-label": pointerTo("stuff"),
					}
				})

				It("returns an UnprocessableEntityError", func() {
					var unprocessableEntityError apierrors.UnprocessableEntityError
					Expect(errors.As(patchErr, &unprocessableEntityError)).To(BeTrue())
					Expect(unprocessableEntityError.Detail()).To(SatisfyAll(
						ContainSubstring("metadata.labels is invalid"),
						ContainSubstring(`"-bad-label"`),
						ContainSubstring("alphanumeric"),
					))
				})
			})
		})

		When("the user is authorized but the app does not exist", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
				appGUID = "invalidAppName"
			})

			It("fails to get the app", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

		When("the user is not authorized", func() {
			It("return a forbidden error", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("SetCurrentDroplet", func() {
		var (
			dropletGUID string
			appGUID     string

			currentDropletRecord CurrentDropletRecord
			setDropletErr        error
			simAppController     func()
			simControllerSync    sync.WaitGroup
		)

		BeforeEach(func() {
			dropletGUID = generateGUID()
			appGUID = cfApp.Name
			createDropletCR(testCtx, k8sClient, dropletGUID, cfApp.Name, cfSpace.Name)

			simControllerSync.Add(1)
			simAppController = func() {
				defer GinkgoRecover()
				defer simControllerSync.Done()

				Eventually(func(g Gomega) {
					theApp := &korifiv1alpha1.CFApp{}
					g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfApp), theApp)).To(Succeed())
					g.Expect(theApp.Spec.CurrentDropletRef.Name).NotTo(BeEmpty())

					theAppCopy := theApp.DeepCopy()
					theAppCopy.Status = korifiv1alpha1.CFAppStatus{
						Conditions: []metav1.Condition{{
							Type:               workloads.StatusConditionStaged,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: metav1.Now(),
							Reason:             "staged",
							Message:            "staged",
						}},
						ObservedDesiredState: "STOPPED",
					}
					g.Expect(k8sClient.Status().Patch(context.Background(), theAppCopy, client.MergeFrom(theApp))).To(Succeed())
				}).Should(Succeed())
			}
		})

		AfterEach(func() {
			simControllerSync.Wait()
		})

		JustBeforeEach(func() {
			go simAppController()

			currentDropletRecord, setDropletErr = appRepo.SetCurrentDroplet(testCtx, authInfo, SetCurrentDropletMessage{
				AppGUID:     appGUID,
				DropletGUID: dropletGUID,
				SpaceGUID:   cfSpace.Name,
			})
		})

		When("user has the space developer role", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("returns a CurrentDroplet record", func() {
				Expect(setDropletErr).NotTo(HaveOccurred())
				Expect(currentDropletRecord).To(Equal(CurrentDropletRecord{
					AppGUID:     cfApp.Name,
					DropletGUID: dropletGUID,
				}))
			})

			It("sets the spec.current_droplet_ref.name to the Droplet GUID", func() {
				lookupKey := client.ObjectKeyFromObject(cfApp)
				updatedApp := new(korifiv1alpha1.CFApp)
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

			When("the app does not get the staged condition", func() {
				BeforeEach(func() {
					simAppController = func() {
						defer GinkgoRecover()
						defer simControllerSync.Done()
					}
				})

				It("returns an error", func() {
					Expect(setDropletErr).To(MatchError(ContainSubstring("did not get the Staged condition")))
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
			_ = createAppCR(testCtx, k8sClient, appName, appGUID, cfSpace.Name, initialAppState)
			appRecord, err := appRepo.SetAppDesiredState(testCtx, authInfo, SetAppDesiredStateMessage{
				AppGUID:      appGUID,
				SpaceGUID:    cfSpace.Name,
				DesiredState: desiredAppState,
			})
			returnedAppRecord = &appRecord
			returnedErr = err
		})

		When("the user has permission to set the app state", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
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
					Expect(returnedAppRecord.SpaceGUID).To(Equal(cfSpace.Name))
					Expect(returnedAppRecord.State).To(Equal(DesiredState("STARTED")))
				})

				It("changes the desired state of the App", func() {
					cfAppLookupKey := types.NamespacedName{Name: appGUID, Namespace: cfSpace.Name}
					updatedCFApp := new(korifiv1alpha1.CFApp)
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
					Expect(returnedAppRecord.SpaceGUID).To(Equal(cfSpace.Name))
					Expect(returnedAppRecord.State).To(Equal(DesiredState("STOPPED")))
				})

				It("changes the desired state of the App", func() {
					cfAppLookupKey := types.NamespacedName{Name: appGUID, Namespace: cfSpace.Name}
					updatedCFApp := new(korifiv1alpha1.CFApp)
					Expect(k8sClient.Get(testCtx, cfAppLookupKey, updatedCFApp)).To(Succeed())
					Expect(string(updatedCFApp.Spec.DesiredState)).To(Equal(appStoppedValue))
				})
			})

			When("the app doesn't exist", func() {
				It("returns an error", func() {
					_, err := appRepo.SetAppDesiredState(testCtx, authInfo, SetAppDesiredStateMessage{
						AppGUID:      "fake-app-guid",
						SpaceGUID:    cfSpace.Name,
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
			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
		})

		JustBeforeEach(func() {
			deleteAppErr = appRepo.DeleteApp(testCtx, authInfo, DeleteAppMessage{
				AppGUID:   appGUID,
				SpaceGUID: cfSpace.Name,
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
			appEnvRecord AppEnvRecord
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
					Namespace: cfSpace.Name,
				},
				StringData: envVars,
			}

			Expect(k8sClient.Create(testCtx, secret)).To(Succeed())
		})

		JustBeforeEach(func() {
			appEnvRecord, getAppEnvErr = appRepo.GetAppEnv(testCtx, authInfo, appGUID)
		})

		When("the user can read secrets in the space", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, k8sClient, cfApp, func() {
					cfApp.Spec.EnvSecretName = secretName
				})).To(Succeed())

				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("returns the env vars stored on the secret", func() {
				Expect(getAppEnvErr).NotTo(HaveOccurred())
				Expect(appEnvRecord.AppGUID).To(Equal(cfApp.Name))
				Expect(appEnvRecord.SpaceGUID).To(Equal(cfApp.Namespace))
				Expect(appEnvRecord.EnvironmentVariables).To(Equal(envVars))
				Expect(appEnvRecord.SystemEnv).To(BeEmpty())
			})

			When("the app has a service-binding secret", func() {
				var (
					vcapServiceSecretDataByte map[string][]byte
					vcapServiceSecretData     map[string]string
					vcapServiceDataPresenter  *env.VcapServicesPresenter
					err                       error
				)

				BeforeEach(func() {
					vcapServicesSecretName := prefixedGUID("vcap-secret")
					vcapServiceSecretDataByte, err = generateVcapServiceSecretDataByte()
					Expect(err).NotTo(HaveOccurred())
					vcapServiceSecretData = asMapOfStrings(vcapServiceSecretDataByte)
					vcapServiceDataPresenter = new(env.VcapServicesPresenter)
					err = json.Unmarshal(vcapServiceSecretDataByte["VCAP_SERVICES"], vcapServiceDataPresenter)
					Expect(err).NotTo(HaveOccurred())

					vcapSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      vcapServicesSecretName,
							Namespace: cfSpace.Name,
						},
						StringData: vcapServiceSecretData,
					}
					Expect(k8sClient.Create(testCtx, vcapSecret)).To(Succeed())

					ogCFApp := cfApp.DeepCopy()
					cfApp.Status.VCAPServicesSecretName = vcapServicesSecretName
					Expect(k8sClient.Status().Patch(testCtx, cfApp, client.MergeFrom(ogCFApp))).To(Succeed())
				})

				It("returns the env vars stored on the secret", func() {
					Expect(getAppEnvErr).NotTo(HaveOccurred())
					Expect(appEnvRecord.EnvironmentVariables).To(Equal(envVars))

					Expect(appEnvRecord.SystemEnv).NotTo(BeEmpty())
					Expect(appEnvRecord.SystemEnv["VCAP_SERVICES"]).To(Equal(vcapServiceDataPresenter))
				})
			})

			When("the app has a service-binding secret with empty VCAP_SERVICES data", func() {
				BeforeEach(func() {
					vcapServicesSecretName := prefixedGUID("vcap-secret")
					vcapSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      vcapServicesSecretName,
							Namespace: cfSpace.Name,
						},
						StringData: map[string]string{
							"VCAP_SERVICES": "{}",
						},
					}
					Expect(k8sClient.Create(testCtx, vcapSecret)).To(Succeed())

					ogCFApp := cfApp.DeepCopy()
					cfApp.Status.VCAPServicesSecretName = vcapServicesSecretName
					Expect(k8sClient.Status().Patch(testCtx, cfApp, client.MergeFrom(ogCFApp))).To(Succeed())
				})

				It("return an empty record for system env variables", func() {
					Expect(getAppEnvErr).NotTo(HaveOccurred())
					Expect(appEnvRecord.SystemEnv).To(BeEmpty())
				})
			})

			When("the app has a service-binding secret with missing VCAP_SERVICES data", func() {
				BeforeEach(func() {
					vcapServicesSecretName := prefixedGUID("vcap-secret")
					vcapSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      vcapServicesSecretName,
							Namespace: cfSpace.Name,
						},
					}
					Expect(k8sClient.Create(testCtx, vcapSecret)).To(Succeed())

					ogCFApp := cfApp.DeepCopy()
					cfApp.Status.VCAPServicesSecretName = vcapServicesSecretName
					Expect(k8sClient.Status().Patch(testCtx, cfApp, client.MergeFrom(ogCFApp))).To(Succeed())
				})

				It("return an empty record for system env variables", func() {
					Expect(getAppEnvErr).NotTo(HaveOccurred())
					Expect(appEnvRecord.SystemEnv).To(BeEmpty())
				})
			})

			When("the EnvSecret doesn't exist", func() {
				BeforeEach(func() {
					secretName = "doIReallyExist"
					Expect(k8s.PatchResource(ctx, k8sClient, cfApp, func() {
						cfApp.Spec.EnvSecretName = secretName
					})).To(Succeed())
				})

				It("errors", func() {
					Expect(getAppEnvErr).To(MatchError(ContainSubstring("Secret")))
				})
			})

			When("the VCAPService secret doesn't exist", func() {
				BeforeEach(func() {
					vcapServicesSecretName := "doIReallyExist"

					ogCFApp := cfApp.DeepCopy()
					cfApp.Status.VCAPServicesSecretName = vcapServicesSecretName
					Expect(k8sClient.Status().Patch(testCtx, cfApp, client.MergeFrom(ogCFApp))).To(Succeed())
				})

				It("errors", func() {
					Expect(getAppEnvErr).To(MatchError(ContainSubstring("Secret")))
				})
			})
		})

		When("EnvSecretName is blank", func() {
			BeforeEach(func() {
				secretName = ""
				Expect(k8s.PatchResource(ctx, k8sClient, cfApp, func() {
					cfApp.Spec.EnvSecretName = secretName
				})).To(Succeed())
			})

			It("returns an empty map", func() {
				Expect(appEnvRecord.EnvironmentVariables).To(BeEmpty())
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
				Expect(getAppEnvErr).To(HaveOccurred())
				Expect(getAppEnvErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})
})

func createApp(space string) *korifiv1alpha1.CFApp {
	return createAppWithGUID(space, generateGUID())
}

func createAppWithGUID(space, guid string) *korifiv1alpha1.CFApp {
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: space,
			Annotations: map[string]string{
				CFAppRevisionKey: CFAppRevisionValue,
			},
		},
		Spec: korifiv1alpha1.CFAppSpec{
			DisplayName:  generateGUID(),
			DesiredState: "STOPPED",
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
				Data: korifiv1alpha1.LifecycleData{
					Buildpacks: []string{"java"},
				},
			},
			CurrentDropletRef: corev1.LocalObjectReference{
				Name: generateGUID(),
			},
			EnvSecretName: GenerateEnvSecretName(guid),
		},
	}
	Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

	cfApp.Status.Conditions = []metav1.Condition{}
	cfApp.Status.ObservedDesiredState = "STOPPED"
	Expect(k8sClient.Status().Update(context.Background(), cfApp)).To(Succeed())

	return cfApp
}

func generateVcapServiceSecretDataByte() (map[string][]byte, error) {
	serviceDetails := env.ServiceDetails{
		Label:        "user-provided",
		Name:         "myupsi",
		Tags:         nil,
		InstanceGUID: "9779c01b-4b03-4a72-93c2-aae2ad4c75b2",
		InstanceName: "myupsi",
		BindingGUID:  "73f68d28-4602-47a3-8110-74ca991d5032",
		BindingName:  nil,
		Credentials: map[string]string{
			"foo": "bar",
		},
		SyslogDrainURL: nil,
		VolumeMounts:   nil,
	}

	vcapServicesData, err := json.Marshal(env.VcapServicesPresenter{
		UserProvided: []env.ServiceDetails{
			serviceDetails,
		},
	})
	if err != nil {
		return nil, err
	}

	secretData := map[string][]byte{}
	secretData["VCAP_SERVICES"] = vcapServicesData

	return secretData, nil
}

func asMapOfStrings(data map[string][]byte) map[string]string {
	result := map[string]string{}

	for k, v := range data {
		result[k] = string(v)
	}

	return result
}
