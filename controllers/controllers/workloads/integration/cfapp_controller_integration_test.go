package integration_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"
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

var _ = Describe("CFAppReconciler", func() {
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
		const (
			cfAppGUID = "test-app-guid"
		)

		It("sets its status.conditions", func() {
			ctx := context.Background()
			cfApp := &v1alpha1.CFApp{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFApp",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespaceGUID,
				},
				Spec: v1alpha1.CFAppSpec{
					DisplayName:  "test-app",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())

			cfAppLookupKey := types.NamespacedName{Name: cfAppGUID, Namespace: namespaceGUID}
			createdCFApp := new(v1alpha1.CFApp)

			Eventually(func() string {
				err := k8sClient.Get(ctx, cfAppLookupKey, createdCFApp)
				if err != nil {
					return ""
				}
				return string(createdCFApp.Status.ObservedDesiredState)
			}).Should(Equal(string(cfApp.Spec.DesiredState)))

			runningConditionFalse := meta.IsStatusConditionFalse(createdCFApp.Status.Conditions, "Running")
			Expect(runningConditionFalse).To(BeTrue())
			Expect(createdCFApp.Status.ObservedDesiredState).To(Equal(createdCFApp.Spec.DesiredState))
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
			cfApp               *v1alpha1.CFApp
			cfPackage           *v1alpha1.CFPackage
			cfBuild             *v1alpha1.CFBuild
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
				droplet := cfBuild.Status.BuildDropletStatus
				processTypes := droplet.ProcessTypes
				for _, process := range processTypes {
					cfProcessList := v1alpha1.CFProcessList{}
					Eventually(func() []v1alpha1.CFProcess {
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

		When("CFProcesses exist for the app", func() {
			var (
				cfProcessForTypeWebGUID string
				cfProcessForTypeWeb     *v1alpha1.CFProcess
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
				cfProcessList := v1alpha1.CFProcessList{}
				Eventually(func() []v1alpha1.CFProcess {
					Expect(
						k8sClient.List(testCtx, &cfProcessList, &client.ListOptions{
							LabelSelector: labelSelectorForAppAndProcess(cfAppGUID, processTypeWorker),
							Namespace:     cfApp.Namespace,
						}),
					).To(Succeed())
					return cfProcessList.Items
				}).Should(HaveLen(1), "Count of CFProcess is not equal to 1")

				cfProcessList = v1alpha1.CFProcessList{}
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
					var webProcessesList v1alpha1.CFProcessList
					Eventually(func() []v1alpha1.CFProcess {
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
				var existingWebProcess *v1alpha1.CFProcess

				BeforeEach(func() {
					existingWebProcess = BuildCFProcessCRObject(GenerateGUID(), namespaceGUID, cfAppGUID, processTypeWeb, processTypeWebCommand)
					Expect(
						k8sClient.Create(context.Background(), existingWebProcess),
					).To(Succeed())
				})

				It("doesn't alter the existing `web` process", func() {
					var webProcessesList v1alpha1.CFProcessList
					Consistently(func() []v1alpha1.CFProcess {
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
				otherCFBuild   *v1alpha1.CFBuild
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
				droplet := cfBuild.Status.BuildDropletStatus
				processTypes := droplet.ProcessTypes
				for _, process := range processTypes {
					cfProcessList := v1alpha1.CFProcessList{}
					Eventually(func() []v1alpha1.CFProcess {
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
	})

	When("a CFApp resource is deleted", func() {
		var (
			cfAppGUID   string
			cfRouteGUID string
			cfApp       *v1alpha1.CFApp
			cfRoute     *v1alpha1.CFRoute
		)

		BeforeEach(func() {
			cfAppGUID = GenerateGUID()
			cfApp = BuildCFAppCRObject(cfAppGUID, namespaceGUID)
			Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

			cfRouteGUID = GenerateGUID()
			cfRoute = &v1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfRouteGUID,
					Namespace: namespaceGUID,
				},
				Spec: v1alpha1.CFRouteSpec{
					Host:     "testRouteHost",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      "testDomainGUID",
						Namespace: namespaceGUID,
					},
					Destinations: []v1alpha1.Destination{
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

			Expect(k8sClient.Delete(context.Background(), cfApp))
		})

		It("eventually deletes the CFApp", func() {
			Eventually(func() bool {
				var createdCFApp v1alpha1.CFApp
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfAppGUID, Namespace: namespaceGUID}, &createdCFApp)
				return apierrors.IsNotFound(err)
			}).Should(BeTrue(), "timed out waiting for app to be deleted")
		})

		It("eventually deletes the destination on the CFRoute", func() {
			var createdCFRoute v1alpha1.CFRoute
			Eventually(func() []v1alpha1.Destination {
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfRouteGUID, Namespace: namespaceGUID}, &createdCFRoute)
				if err != nil {
					return []v1alpha1.Destination{}
				}
				return createdCFRoute.Spec.Destinations
			}).Should(HaveLen(1), "expecting length of destinations to be 1 after cfapp delete")

			Expect(createdCFRoute.Spec.Destinations).Should(ConsistOf(v1alpha1.Destination{
				GUID: "destination-2-guid",
				Port: 0,
				AppRef: corev1.LocalObjectReference{
					Name: "some-other-app-guid",
				},
				ProcessType: "worked",
				Protocol:    "http1",
			}))
		})
	})
})
