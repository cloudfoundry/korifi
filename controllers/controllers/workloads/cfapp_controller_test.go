package workloads_test

import (
	"context"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFAppReconciler Integration Tests", func() {
	var (
		cfSpace      *korifiv1alpha1.CFSpace
		cfDomainGUID string
	)

	BeforeEach(func() {
		cfSpace = createSpace(cfOrg)
		cfDomainGUID = PrefixedGUID("test-domain")
		cfDomain := &korifiv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfSpace.Status.GUID,
				Name:      cfDomainGUID,
			},
			Spec: korifiv1alpha1.CFDomainSpec{
				Name: "a" + uuid.NewString() + ".com",
			},
		}
		Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())
	})

	When("a new CFApp resource is created", func() {
		var (
			cfAppGUID      string
			cfApp          *korifiv1alpha1.CFApp
			serviceBinding *korifiv1alpha1.CFServiceBinding
		)

		BeforeEach(func() {
			ctx = context.Background()
			cfAppGUID = PrefixedGUID("create-app")

			cfApp = &korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: cfSpace.Status.GUID,
					Annotations: map[string]string{
						korifiv1alpha1.CFAppRevisionKey: "42",
					},
				},
				Spec: korifiv1alpha1.CFAppSpec{
					DisplayName:  "test-app",
					DesiredState: "STOPPED",
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}

			serviceInstanceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      PrefixedGUID("service-instance-secret"),
					Namespace: cfSpace.Status.GUID,
				},
				StringData: map[string]string{
					"foo": "bar",
				},
			}
			Expect(k8sClient.Create(context.Background(), serviceInstanceSecret)).To(Succeed())

			serviceInstance := &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      PrefixedGUID("app-service-instance"),
					Namespace: cfSpace.Status.GUID,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					Type:       "user-provided",
					SecretName: serviceInstanceSecret.Name,
				},
			}
			Expect(k8sClient.Create(ctx, serviceInstance)).To(Succeed())

			serviceBinding = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      PrefixedGUID("app-service-binding"),
					Namespace: cfSpace.Status.GUID,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					AppRef: corev1.LocalObjectReference{
						Name: cfAppGUID,
					},
					Service: corev1.ObjectReference{
						Namespace: cfSpace.Status.GUID,
						Name:      serviceInstance.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, serviceBinding)).To(Succeed())
			Expect(k8s.Patch(ctx, k8sClient, serviceBinding, func() {
				serviceBinding.Status.Conditions = []metav1.Condition{}
				serviceBinding.Status.Binding = corev1.LocalObjectReference{
					Name: serviceInstanceSecret.Name,
				}
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())
		})

		It("sets the last-stop-app-rev annotation to the value of the app-rev annotation", func() {
			Eventually(func(g Gomega) {
				createdCFApp, err := getApp(cfSpace.Status.GUID, cfAppGUID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(createdCFApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]).To(Equal("42"))
			}).Should(Succeed())
		})

		When("status.lastStopAppRev is not empty", func() {
			BeforeEach(func() {
				cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey] = "2"
			})

			It("doesn't set it", func() {
				Consistently(func(g Gomega) {
					createdCFApp, err := getApp(cfSpace.Status.GUID, cfAppGUID)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(createdCFApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]).To(Equal("2"))
				}, "1s").Should(Succeed())
			})
		})

		It("sets status.vcapApplicationSecretName and creates the corresponding secret", func() {
			var createdSecret corev1.Secret

			Eventually(func(g Gomega) {
				createdCFApp, err := getApp(cfSpace.Status.GUID, cfAppGUID)
				g.Expect(err).NotTo(HaveOccurred())

				vcapApplicationSecretName := createdCFApp.Status.VCAPApplicationSecretName
				g.Expect(vcapApplicationSecretName).NotTo(BeEmpty())

				vcapApplicationSecretLookupKey := types.NamespacedName{Name: vcapApplicationSecretName, Namespace: cfSpace.Status.GUID}
				g.Expect(k8sClient.Get(ctx, vcapApplicationSecretLookupKey, &createdSecret)).To(Succeed())
			}).Should(Succeed())

			Expect(createdSecret.Data).To(HaveKeyWithValue("VCAP_APPLICATION", ContainSubstring("application_id")))
			Expect(createdSecret.OwnerReferences).To(HaveLen(1))
			Expect(createdSecret.OwnerReferences[0].Name).To(Equal(cfApp.Name))
		})

		getVCAPServicesSecret := func() *corev1.Secret {
			secret := new(corev1.Secret)
			Eventually(func(g Gomega) {
				createdCFApp, err := getApp(cfSpace.Status.GUID, cfAppGUID)
				g.Expect(err).NotTo(HaveOccurred())

				vcapServicesSecretName := createdCFApp.Status.VCAPServicesSecretName
				g.Expect(vcapServicesSecretName).NotTo(BeEmpty())

				vcapServicesSecretLookupKey := types.NamespacedName{Name: vcapServicesSecretName, Namespace: cfSpace.Status.GUID}
				g.Expect(k8sClient.Get(ctx, vcapServicesSecretLookupKey, secret)).To(Succeed())
				g.Expect(secret.Data).To(HaveKeyWithValue("VCAP_SERVICES", ContainSubstring("user-provided")))
			}).Should(Succeed())

			return secret
		}

		It("sets status.vcapServicesSecretName and creates the corresponding secret", func() {
			createdSecret := getVCAPServicesSecret()
			Expect(createdSecret.Data).To(HaveKeyWithValue("VCAP_SERVICES", ContainSubstring("user-provided")))
			Expect(createdSecret.OwnerReferences).To(HaveLen(1))
			Expect(createdSecret.OwnerReferences[0].Name).To(Equal(cfApp.Name))
		})

		It("sets its status conditions", func() {
			Eventually(func(g Gomega) {
				createdCFApp, err := getApp(cfSpace.Status.GUID, cfAppGUID)
				g.Expect(err).NotTo(HaveOccurred())
				readyStatusCondition := meta.FindStatusCondition(createdCFApp.Status.Conditions, shared.StatusConditionReady)
				g.Expect(readyStatusCondition).NotTo(BeNil())
				g.Expect(readyStatusCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyStatusCondition.Reason).To(Equal("DropletNotAssigned"))
				g.Expect(readyStatusCondition.ObservedGeneration).To(Equal(createdCFApp.Generation))
			}).Should(Succeed())
		})

		It("sets the ObservedGeneration status field", func() {
			Eventually(func(g Gomega) {
				createdCFApp, err := getApp(cfSpace.Status.GUID, cfAppGUID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(createdCFApp.Status.ObservedGeneration).To(Equal(createdCFApp.Generation))
			}).Should(Succeed())
		})

		When("the service binding is deleted", func() {
			var vcapServicesSecret *corev1.Secret

			JustBeforeEach(func() {
				vcapServicesSecret = getVCAPServicesSecret()
				Expect(k8sClient.Delete(ctx, serviceBinding)).To(Succeed())
			})

			It("updates the VCAP_SERVICES secret", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(vcapServicesSecret), vcapServicesSecret)).To(Succeed())
					g.Expect(vcapServicesSecret.Data).To(HaveKeyWithValue("VCAP_SERVICES", Equal([]byte("{}"))))
				}).Should(Succeed())
			})
		})
	})

	When("the app references a non-existing droplet", func() {
		var (
			cfAppGUID string
			cfApp     *korifiv1alpha1.CFApp
		)

		BeforeEach(func() {
			cfAppGUID = GenerateGUID()

			cfApp = BuildCFAppCRObject(cfAppGUID, cfSpace.Status.GUID)
			cfApp.Spec.CurrentDropletRef.Name = "droplet-that-does-not-exist"
			Expect(
				k8sClient.Create(context.Background(), cfApp),
			).To(Succeed())
		})

		It("sets the ready condition to false", func() {
			Eventually(func(g Gomega) {
				createdCFApp, err := getApp(cfSpace.Status.GUID, cfAppGUID)
				g.Expect(err).NotTo(HaveOccurred())
				readyStatusCondition := meta.FindStatusCondition(createdCFApp.Status.Conditions, shared.StatusConditionReady)
				g.Expect(readyStatusCondition).NotTo(BeNil())
				g.Expect(readyStatusCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyStatusCondition.Reason).To(Equal("CannotResolveCurrentDropletRef"))
				g.Expect(readyStatusCondition.ObservedGeneration).To(Equal(createdCFApp.Generation))
			}).Should(Succeed())
		})
	})

	When("a CFApp resource exists with a valid currentDropletRef set", func() {
		const (
			processTypeWeb           = "web"
			processTypeWebCommand    = "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"
			processTypeWorker        = "worker"
			processTypeWorkerCommand = "bundle exec rackup config.ru"
			port8080                 = 8080
			port9000                 = 9000
		)

		var (
			cfAppGUID           string
			cfBuildGUID         string
			cfPackageGUID       string
			cfApp               *korifiv1alpha1.CFApp
			cfPackage           *korifiv1alpha1.CFPackage
			cfBuild             *korifiv1alpha1.CFBuild
			dropletProcessTypes map[string]string
		)

		BeforeEach(func() {
			cfAppGUID = GenerateGUID()
			cfPackageGUID = GenerateGUID()
			cfBuildGUID = GenerateGUID()

			cfApp = BuildCFAppCRObject(cfAppGUID, cfSpace.Status.GUID)
			Expect(
				k8sClient.Create(context.Background(), cfApp),
			).To(Succeed())

			cfPackage = BuildCFPackageCRObject(cfPackageGUID, cfSpace.Status.GUID, cfAppGUID, "ref")
			Expect(
				k8sClient.Create(context.Background(), cfPackage),
			).To(Succeed())

			cfBuild = BuildCFBuildObject(cfBuildGUID, cfSpace.Status.GUID, cfPackageGUID, cfAppGUID)
			dropletProcessTypes = map[string]string{
				processTypeWeb:    processTypeWebCommand,
				processTypeWorker: processTypeWorkerCommand,
			}
		})

		JustBeforeEach(func() {
			buildDropletStatus := BuildCFBuildDropletStatusObject(dropletProcessTypes, []int32{port8080, port9000})
			cfBuild = createBuildWithDroplet(context.Background(), k8sClient, cfBuild, buildDropletStatus)

			patchAppWithDroplet(context.Background(), k8sClient, cfAppGUID, cfSpace.Status.GUID, cfBuildGUID)
		})

		When("CFProcesses do not exist for the app", func() {
			It("eventually creates CFProcess for each process listed on the droplet", func() {
				testCtx := context.Background()
				droplet := cfBuild.Status.Droplet
				processTypes := droplet.ProcessTypes
				for _, process := range processTypes {
					cfProcessList := korifiv1alpha1.CFProcessList{}
					Eventually(func() []korifiv1alpha1.CFProcess {
						Expect(
							k8sClient.List(testCtx, &cfProcessList, &client.ListOptions{
								LabelSelector: labelSelectorForAppAndProcess(cfAppGUID, process.Type),
								Namespace:     cfApp.Namespace,
							}),
						).To(Succeed())
						return cfProcessList.Items
					}).Should(HaveLen(1), "expected CFProcess to eventually be created")
					createdCFProcess := cfProcessList.Items[0]
					Expect(createdCFProcess.Spec.DetectedCommand).To(Equal(process.Command), "cfprocess command does not match with droplet command")
					Expect(createdCFProcess.Spec.AppRef.Name).To(Equal(cfAppGUID), "cfprocess app ref does not match app-guid")
					Expect(createdCFProcess.Spec.Ports).To(Equal(droplet.Ports), "cfprocess ports does not match ports on droplet")

					Expect(createdCFProcess.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
						{
							APIVersion:         "korifi.cloudfoundry.org/v1alpha1",
							Kind:               "CFApp",
							Name:               cfApp.Name,
							UID:                cfApp.GetUID(),
							Controller:         tools.PtrTo(true),
							BlockOwnerDeletion: tools.PtrTo(true),
						},
					}))
				}
			})
		})

		When("CFProcesses exist for the app", func() {
			var (
				cfProcessForTypeWebGUID string
				cfProcessForTypeWeb     *korifiv1alpha1.CFProcess
			)

			BeforeEach(func() {
				beforeCtx := context.Background()
				cfProcessForTypeWebGUID = GenerateGUID()
				cfProcessForTypeWeb = BuildCFProcessCRObject(cfProcessForTypeWebGUID, cfSpace.Status.GUID, cfAppGUID, processTypeWeb, processTypeWebCommand, "")
				cfProcessForTypeWeb.Spec.Command = ""
				Expect(k8sClient.Create(beforeCtx, cfProcessForTypeWeb)).To(Succeed())
			})

			When("a process from processTypes does not exist", func() {
				It("creates it", func() {
					Expect(findProcessWithType(cfApp, processTypeWorker)).NotTo(BeNil())
				})
			})

			It("sets the detectedCommand on the web process to the droplet value for the web process if empty", func() {
				Eventually(func(g Gomega) {
					proc := findProcessWithType(cfApp, processTypeWeb)
					g.Expect(proc.Spec.DetectedCommand).To(Equal(processTypeWebCommand))
				}).Should(Succeed())
			})

			When("the command on the web process is not empty", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(context.Background(), k8sClient, cfProcessForTypeWeb, func() {
						cfProcessForTypeWeb.Spec.Command = "something else"
					})).To(Succeed())
				})

				It("should save the detectedCommand but not change the command", func() {
					Eventually(func(g Gomega) {
						proc := findProcessWithType(cfApp, processTypeWeb)
						g.Expect(proc.Spec.DetectedCommand).To(Equal(processTypeWebCommand))
					}).Should(Succeed())
					Consistently(func(g Gomega) {
						proc := findProcessWithType(cfApp, processTypeWeb)
						g.Expect(proc.Spec.Command).To(Equal("something else"))
					}).Should(Succeed())
				})
			})
		})

		When("the build/droplet doesn't include a `web` process", func() {
			BeforeEach(func() {
				dropletProcessTypes = map[string]string{
					processTypeWorker: processTypeWorkerCommand,
				}
			})

			When("no `web` CFProcess exists for the app", func() {
				It("adds a `web` process without a start command", func() {
					var webProcessesList korifiv1alpha1.CFProcessList
					Eventually(func() []korifiv1alpha1.CFProcess {
						Expect(
							k8sClient.List(context.Background(), &webProcessesList, &client.ListOptions{
								LabelSelector: labelSelectorForAppAndProcess(cfAppGUID, processTypeWeb),
								Namespace:     cfApp.Namespace,
							}),
						).To(Succeed())
						return webProcessesList.Items
					}).Should(HaveLen(1))

					webProcess := webProcessesList.Items[0]
					Expect(webProcess.Spec.AppRef.Name).To(Equal(cfAppGUID))
					Expect(webProcess.Spec.Command).To(Equal(""))
					Expect(webProcess.Spec.DetectedCommand).To(Equal(""))
				})
			})

			When("the `web` CFProcess already exists", func() {
				var existingWebProcess *korifiv1alpha1.CFProcess

				BeforeEach(func() {
					existingWebProcess = BuildCFProcessCRObject(GenerateGUID(), cfSpace.Status.GUID, cfAppGUID, processTypeWeb, processTypeWebCommand, "")
					Expect(
						k8sClient.Create(context.Background(), existingWebProcess),
					).To(Succeed())
				})

				It("doesn't alter the existing `web` process", func() {
					var webProcessesList korifiv1alpha1.CFProcessList
					Consistently(func() []korifiv1alpha1.CFProcess {
						Expect(
							k8sClient.List(context.Background(), &webProcessesList, &client.ListOptions{
								LabelSelector: labelSelectorForAppAndProcess(cfAppGUID, processTypeWeb),
								Namespace:     cfApp.Namespace,
							}),
						).To(Succeed())
						return webProcessesList.Items
					}, 5*time.Second).Should(HaveLen(1))
					webProcess := webProcessesList.Items[0]
					Expect(webProcess.Name).To(Equal(existingWebProcess.Name))
					Expect(webProcess.Spec).To(Equal(existingWebProcess.Spec))
				})
			})
		})

		When("the droplet has no ports set", func() {
			var (
				otherBuildGUID string
				otherCFBuild   *korifiv1alpha1.CFBuild
			)

			BeforeEach(func() {
				beforeCtx := context.Background()

				otherBuildGUID = GenerateGUID()
				otherCFBuild = BuildCFBuildObject(otherBuildGUID, cfSpace.Status.GUID, cfPackageGUID, cfAppGUID)
				dropletProcessTypeMap := map[string]string{
					processTypeWeb: processTypeWebCommand,
				}
				dropletPorts := []int32{}
				buildDropletStatus := BuildCFBuildDropletStatusObject(dropletProcessTypeMap, dropletPorts)
				otherCFBuild = createBuildWithDroplet(beforeCtx, k8sClient, otherCFBuild, buildDropletStatus)
			})

			It("eventually creates CFProcess with empty ports and a healthCheck type of \"process\"", func() {
				testCtx := context.Background()
				droplet := cfBuild.Status.Droplet
				processTypes := droplet.ProcessTypes
				for _, process := range processTypes {
					cfProcessList := korifiv1alpha1.CFProcessList{}
					Eventually(func() []korifiv1alpha1.CFProcess {
						Expect(
							k8sClient.List(testCtx, &cfProcessList, &client.ListOptions{
								LabelSelector: labelSelectorForAppAndProcess(cfAppGUID, process.Type),
								Namespace:     cfApp.Namespace,
							}),
						).To(Succeed())
						return cfProcessList.Items
					}).Should(HaveLen(1), "expected CFProcess to eventually be created")
					createdCFProcess := cfProcessList.Items[0]
					Expect(createdCFProcess.Spec.Ports).To(Equal(droplet.Ports), "cfprocess ports does not match ports on droplet")
				}
			})
		})

		It("sets the ready condition to true", func() {
			Eventually(func(g Gomega) {
				createdCFApp, err := getApp(cfSpace.Status.GUID, cfAppGUID)
				g.Expect(err).NotTo(HaveOccurred())
				readyStatusCondition := meta.FindStatusCondition(createdCFApp.Status.Conditions, shared.StatusConditionReady)
				g.Expect(readyStatusCondition).NotTo(BeNil())
				g.Expect(readyStatusCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(readyStatusCondition.Reason).To(Equal("DropletAssigned"))
				g.Expect(readyStatusCondition.ObservedGeneration).To(Equal(createdCFApp.Generation))
			}).Should(Succeed())
		})

		When("the droplet disappears", func() {
			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					createdCFApp, err := getApp(cfSpace.Status.GUID, cfAppGUID)
					g.Expect(err).NotTo(HaveOccurred())
					readyStatusCondition := meta.FindStatusCondition(createdCFApp.Status.Conditions, shared.StatusConditionReady)
					g.Expect(readyStatusCondition).NotTo(BeNil())
					g.Expect(readyStatusCondition.Status).To(Equal(metav1.ConditionTrue))
					g.Expect(readyStatusCondition.ObservedGeneration).To(Equal(createdCFApp.Generation))
				}).Should(Succeed())
				Expect(k8sClient.Delete(context.Background(), cfBuild)).To(Succeed())
			})

			It("sets the ready condition to false", func() {
				Eventually(func(g Gomega) {
					createdCFApp, err := getApp(cfSpace.Status.GUID, cfAppGUID)
					g.Expect(err).NotTo(HaveOccurred())
					readyStatusCondition := meta.FindStatusCondition(createdCFApp.Status.Conditions, shared.StatusConditionReady)
					g.Expect(readyStatusCondition).NotTo(BeNil())
					g.Expect(readyStatusCondition.Status).To(Equal(metav1.ConditionFalse))
					g.Expect(readyStatusCondition.Reason).To(Equal("CannotResolveCurrentDropletRef"))
					g.Expect(readyStatusCondition.ObservedGeneration).To(Equal(createdCFApp.Generation))
				}).Should(Succeed())
			})
		})
	})

	When("a CFApp resource is deleted", func() {
		var (
			cfAppGUID   string
			cfRouteGUID string
			cfApp       *korifiv1alpha1.CFApp
			cfRoute     *korifiv1alpha1.CFRoute
		)

		BeforeEach(func() {
			cfAppGUID = GenerateGUID()
			cfApp = BuildCFAppCRObject(cfAppGUID, cfSpace.Status.GUID)
			Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

			cfRouteGUID = GenerateGUID()
			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfRouteGUID,
					Namespace: cfSpace.Status.GUID,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "test-route-host",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      cfDomainGUID,
						Namespace: cfSpace.Status.GUID,
					},
					Destinations: []korifiv1alpha1.Destination{
						{
							GUID: "destination-1-guid",
							Port: 0,
							AppRef: corev1.LocalObjectReference{
								Name: cfAppGUID,
							},
							ProcessType: "web",
							Protocol:    "http1",
						},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), cfRoute)).To(Succeed())
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Delete(context.Background(), cfApp)).To(Succeed())
		})

		It("eventually deletes the CFApp", func() {
			Eventually(func() bool {
				var createdCFApp korifiv1alpha1.CFApp
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfAppGUID, Namespace: cfSpace.Status.GUID}, &createdCFApp)
				return apierrors.IsNotFound(err)
			}).Should(BeTrue(), "timed out waiting for app to be deleted")
		})

		It("writes a log message", func() {
			Eventually(logOutput).Should(gbytes.Say("finalizer removed"))
		})

		It("eventually deletes the destination on the CFRoute", func() {
			Eventually(func(g Gomega) {
				var createdCFRoute korifiv1alpha1.CFRoute
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfRouteGUID, Namespace: cfSpace.Status.GUID}, &createdCFRoute)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(createdCFRoute.Spec.Destinations).To(BeEmpty())
			}).Should(Succeed())
		})

		When("the app is referenced by service bindings", func() {
			BeforeEach(func() {
				cfServiceBinding := korifiv1alpha1.CFServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      PrefixedGUID("service-binding"),
						Namespace: cfSpace.Status.GUID,
					},
					Spec: korifiv1alpha1.CFServiceBindingSpec{
						AppRef: corev1.LocalObjectReference{
							Name: cfAppGUID,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), &cfServiceBinding)).To(Succeed())
			})

			It("deletes the referencing service bindings", func() {
				Eventually(func(g Gomega) {
					sbList := korifiv1alpha1.CFServiceBindingList{}
					g.Expect(k8sClient.List(context.Background(), &sbList, client.InNamespace(cfSpace.Status.GUID))).To(Succeed())
					g.Expect(sbList.Items).To(BeEmpty())
				}).Should(Succeed())
			})
		})
	})
})

func getApp(nsGUID, appGUID string) (*korifiv1alpha1.CFApp, error) {
	cfAppLookupKey := types.NamespacedName{Name: appGUID, Namespace: nsGUID}
	createdCFApp := &korifiv1alpha1.CFApp{}
	err := k8sClient.Get(context.Background(), cfAppLookupKey, createdCFApp)
	return createdCFApp, err
}

func findProcessWithType(cfApp *korifiv1alpha1.CFApp, processType string) *korifiv1alpha1.CFProcess {
	cfProcessList := &korifiv1alpha1.CFProcessList{}

	Eventually(func(g Gomega) {
		g.Expect(k8sClient.List(ctx, cfProcessList, &client.ListOptions{
			LabelSelector: labelSelectorForAppAndProcess(cfApp.Name, processType),
			Namespace:     cfApp.Namespace,
		})).To(Succeed())
		g.Expect(cfProcessList.Items).To(HaveLen(1))
	}).Should(Succeed())

	return &cfProcessList.Items[0]
}

func labelSelectorForAppAndProcess(appGUID, processType string) labels.Selector {
	labelSelectorMap := labels.Set{
		CFAppLabelKey:         appGUID,
		CFProcessTypeLabelKey: processType,
	}
	selector, selectorValidationErr := labelSelectorMap.AsValidatedSelector()
	Expect(selectorValidationErr).NotTo(HaveOccurred())
	return selector
}
