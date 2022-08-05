package integration_test

import (
	"context"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		namespaceGUID string
		ns            *corev1.Namespace
	)

	BeforeEach(func() {
		namespaceGUID = GenerateGUID()
		ns = createNamespace(context.Background(), k8sClient, namespaceGUID)
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), ns)).To(Succeed())
	})

	When("a new CFApp resource is created", func() {
		var (
			ctx       context.Context
			cfAppGUID string
			cfApp     *korifiv1alpha1.CFApp
		)

		BeforeEach(func() {
			ctx = context.Background()
			cfAppGUID = PrefixedGUID("create-app")

			cfApp = &korifiv1alpha1.CFApp{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFApp",
					APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespaceGUID,
				},
				Spec: korifiv1alpha1.CFAppSpec{
					DisplayName:  "test-app",
					DesiredState: "STOPPED",
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(
				k8sClient.Create(ctx, cfApp),
			).To(Succeed())
		})

		It("eventually sets status.vcapServicesSecretName and creates the corresponding secret", func() {
			Eventually(func() string {
				createdCFApp, err := getApp(namespaceGUID, cfAppGUID)
				if err != nil {
					return ""
				}
				return string(createdCFApp.Status.VCAPServicesSecretName)
			}).Should(Not(BeEmpty()))

			vcapServicesSecretLookupKey := types.NamespacedName{Name: cfAppGUID + "-vcap-services", Namespace: namespaceGUID}
			createdSecret := new(corev1.Secret)
			Expect(k8sClient.Get(ctx, vcapServicesSecretLookupKey, createdSecret)).To(Succeed())
			Expect(createdSecret.Data).To(HaveKeyWithValue("VCAP_SERVICES", []byte("{}")))
			Expect(createdSecret.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
				{
					APIVersion: "korifi.cloudfoundry.org/v1alpha1",
					Kind:       "CFApp",
					Name:       cfApp.Name,
					UID:        cfApp.GetUID(),
				},
			}))
		})

		It("sets its status.conditions", func() {
			Eventually(func(g Gomega) {
				createdCFApp, err := getApp(namespaceGUID, cfAppGUID)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(createdCFApp.Status.ObservedDesiredState).To(Equal(cfApp.Spec.DesiredState))
				g.Expect(meta.IsStatusConditionTrue(createdCFApp.Status.Conditions, workloads.StatusConditionStaged)).To(BeFalse())
				g.Expect(meta.IsStatusConditionTrue(createdCFApp.Status.Conditions, workloads.StatusConditionRunning)).To(BeFalse())
			}).Should(Succeed())
		})
	})

	When("the app references a non-existing droplet", func() {
		var (
			cfAppGUID string
			cfApp     *korifiv1alpha1.CFApp
		)

		BeforeEach(func() {
			cfAppGUID = GenerateGUID()

			cfApp = BuildCFAppCRObject(cfAppGUID, namespaceGUID)
			cfApp.Spec.CurrentDropletRef.Name = "droplet-that-does-not-exist"
			Expect(
				k8sClient.Create(context.Background(), cfApp),
			).To(Succeed())
		})

		It("sets the staged condition to false", func() {
			Consistently(func(g Gomega) {
				createdCFApp, err := getApp(namespaceGUID, cfAppGUID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(meta.IsStatusConditionTrue(createdCFApp.Status.Conditions, workloads.StatusConditionStaged)).To(BeFalse())
			}, "1s").Should(Succeed())
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

			cfApp = BuildCFAppCRObject(cfAppGUID, namespaceGUID)
			Expect(
				k8sClient.Create(context.Background(), cfApp),
			).To(Succeed())

			cfPackage = BuildCFPackageCRObject(cfPackageGUID, namespaceGUID, cfAppGUID)
			Expect(
				k8sClient.Create(context.Background(), cfPackage),
			).To(Succeed())

			cfBuild = BuildCFBuildObject(cfBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
			dropletProcessTypes = map[string]string{
				processTypeWeb:    processTypeWebCommand,
				processTypeWorker: processTypeWorkerCommand,
			}
		})

		JustBeforeEach(func() {
			buildDropletStatus := BuildCFBuildDropletStatusObject(dropletProcessTypes, []int32{port8080, port9000})
			cfBuild = createBuildWithDroplet(context.Background(), k8sClient, cfBuild, buildDropletStatus)

			patchAppWithDroplet(context.Background(), k8sClient, cfAppGUID, namespaceGUID, cfBuildGUID)
		})

		labelSelectorForAppAndProcess := func(appGUID, processType string) labels.Selector {
			labelSelectorMap := labels.Set{
				CFAppLabelKey:         appGUID,
				CFProcessTypeLabelKey: processType,
			}
			selector, selectorValidationErr := labelSelectorMap.AsValidatedSelector()
			Expect(selectorValidationErr).NotTo(HaveOccurred())
			return selector
		}

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
					Expect(createdCFProcess.Spec.Command).To(Equal(process.Command), "cfprocess command does not match with droplet command")
					Expect(createdCFProcess.Spec.AppRef.Name).To(Equal(cfAppGUID), "cfprocess app ref does not match app-guid")
					Expect(createdCFProcess.Spec.Ports).To(Equal(droplet.Ports), "cfprocess ports does not match ports on droplet")
					Expect(createdCFProcess.Spec.HealthCheck.Type).To(Equal(korifiv1alpha1.HealthCheckType("process")))

					Expect(createdCFProcess.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
						{
							APIVersion: "korifi.cloudfoundry.org/v1alpha1",
							Kind:       "CFApp",
							Name:       cfApp.Name,
							UID:        cfApp.GetUID(),
						},
					}))
				}
			})
		})

		When("routes exist and CFProcesses do not exist for the app", func() {
			var (
				cfRouteGUID string
				cfRoute     *korifiv1alpha1.CFRoute
			)

			BeforeEach(func() {
				cfRouteGUID = GenerateGUID()
				cfRoute = &korifiv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfRouteGUID,
						Namespace: namespaceGUID,
					},
					Spec: korifiv1alpha1.CFRouteSpec{
						Host:     "testRouteHost",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.ObjectReference{
							Name:      "testDomainGUID",
							Namespace: namespaceGUID,
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
							{
								GUID: "destination-2-guid",
								Port: 0,
								AppRef: corev1.LocalObjectReference{
									Name: "some-other-app-guid",
								},
								ProcessType: "worked",
								Protocol:    "http1",
							},
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), cfRoute)).To(Succeed())
			})

			It("eventually creates CFProcess for each process with the correct health check type", func() {
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
					if process.Type == "web" {
						Expect(createdCFProcess.Spec.HealthCheck.Type).To(Equal(korifiv1alpha1.HealthCheckType("port")))
					} else {
						Expect(createdCFProcess.Spec.HealthCheck.Type).To(Equal(korifiv1alpha1.HealthCheckType("process")))
					}
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
				cfProcessForTypeWeb = BuildCFProcessCRObject(cfProcessForTypeWebGUID, namespaceGUID, cfAppGUID, processTypeWeb, processTypeWebCommand)
				Expect(k8sClient.Create(beforeCtx, cfProcessForTypeWeb)).To(Succeed())
			})

			It("eventually creates CFProcess for only the missing processTypes", func() {
				testCtx := context.Background()

				// Checking for worker type first ensures that we wait long enough for processes to be created.
				cfProcessList := korifiv1alpha1.CFProcessList{}
				Eventually(func() []korifiv1alpha1.CFProcess {
					Expect(
						k8sClient.List(testCtx, &cfProcessList, &client.ListOptions{
							LabelSelector: labelSelectorForAppAndProcess(cfAppGUID, processTypeWorker),
							Namespace:     cfApp.Namespace,
						}),
					).To(Succeed())
					return cfProcessList.Items
				}).Should(HaveLen(1), "Count of CFProcess is not equal to 1")

				cfProcessList = korifiv1alpha1.CFProcessList{}
				Expect(
					k8sClient.List(testCtx, &cfProcessList, &client.ListOptions{
						LabelSelector: labelSelectorForAppAndProcess(cfAppGUID, processTypeWorker),
						Namespace:     cfApp.Namespace,
					}),
				).To(Succeed())
				Expect(cfProcessList.Items).Should(HaveLen(1), "Count of CFProcess is not equal to 1")
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
				})
			})

			When("the `web` CFProcess already exists", func() {
				var existingWebProcess *korifiv1alpha1.CFProcess

				BeforeEach(func() {
					existingWebProcess = BuildCFProcessCRObject(GenerateGUID(), namespaceGUID, cfAppGUID, processTypeWeb, processTypeWebCommand)
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
				otherCFBuild = BuildCFBuildObject(otherBuildGUID, namespaceGUID, cfPackageGUID, cfAppGUID)
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
					Expect(string(createdCFProcess.Spec.HealthCheck.Type)).To(Equal("process"))
				}
			})
		})

		It("sets the staged condition to true", func() {
			Eventually(func(g Gomega) {
				createdCFApp, err := getApp(namespaceGUID, cfAppGUID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(meta.IsStatusConditionTrue(createdCFApp.Status.Conditions, workloads.StatusConditionStaged)).To(BeTrue())
			}).Should(Succeed())
		})

		When("the droplet disappears", func() {
			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					createdCFApp, err := getApp(namespaceGUID, cfAppGUID)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(meta.IsStatusConditionTrue(createdCFApp.Status.Conditions, workloads.StatusConditionStaged)).To(BeTrue())
				}).Should(Succeed())
				Expect(k8sClient.Delete(context.Background(), cfBuild)).To(Succeed())
			})

			It("unsets the staged condition", func() {
				Eventually(func(g Gomega) {
					createdCFApp, err := getApp(namespaceGUID, cfAppGUID)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(meta.IsStatusConditionTrue(createdCFApp.Status.Conditions, workloads.StatusConditionStaged)).To(BeFalse())
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
			cfApp = BuildCFAppCRObject(cfAppGUID, namespaceGUID)
			Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

			cfRouteGUID = GenerateGUID()
			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfRouteGUID,
					Namespace: namespaceGUID,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "testRouteHost",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      "testDomainGUID",
						Namespace: namespaceGUID,
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
						{
							GUID: "destination-2-guid",
							Port: 0,
							AppRef: corev1.LocalObjectReference{
								Name: "some-other-app-guid",
							},
							ProcessType: "worked",
							Protocol:    "http1",
						},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), cfRoute)).To(Succeed())
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Delete(context.Background(), cfApp))
		})

		It("eventually deletes the CFApp", func() {
			Eventually(func() bool {
				var createdCFApp korifiv1alpha1.CFApp
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfAppGUID, Namespace: namespaceGUID}, &createdCFApp)
				return apierrors.IsNotFound(err)
			}).Should(BeTrue(), "timed out waiting for app to be deleted")
		})

		It("eventually deletes the destination on the CFRoute", func() {
			var createdCFRoute korifiv1alpha1.CFRoute
			Eventually(func() []korifiv1alpha1.Destination {
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfRouteGUID, Namespace: namespaceGUID}, &createdCFRoute)
				if err != nil {
					return []korifiv1alpha1.Destination{}
				}
				return createdCFRoute.Spec.Destinations
			}).Should(HaveLen(1), "expecting length of destinations to be 1 after cfapp delete")

			Expect(createdCFRoute.Spec.Destinations).Should(ConsistOf(korifiv1alpha1.Destination{
				GUID: "destination-2-guid",
				Port: 0,
				AppRef: corev1.LocalObjectReference{
					Name: "some-other-app-guid",
				},
				ProcessType: "worked",
				Protocol:    "http1",
			}))
		})

		When("the app is referenced by tasks", func() {
			var taskGUID string
			BeforeEach(func() {
				taskGUID = PrefixedGUID("task")
				cfTask := korifiv1alpha1.CFTask{
					ObjectMeta: metav1.ObjectMeta{
						Name:      taskGUID,
						Namespace: namespaceGUID,
					},
					Spec: korifiv1alpha1.CFTaskSpec{
						Command: "sleep 1000",
						AppRef: corev1.LocalObjectReference{
							Name: cfAppGUID,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), &cfTask)).To(Succeed())
			})

			It("deletes the referencing tasks", func() {
				Eventually(func(g Gomega) {
					tasksList := korifiv1alpha1.CFTaskList{}
					g.Expect(k8sClient.List(context.Background(), &tasksList, client.InNamespace(namespaceGUID))).To(Succeed())
					g.Expect(tasksList.Items).To(BeEmpty())
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
