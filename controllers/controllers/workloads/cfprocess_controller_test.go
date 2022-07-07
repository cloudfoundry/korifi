package workloads_test

import (
	"context"
	"errors"
	"strconv"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	workloadsfakes "code.cloudfoundry.org/korifi/controllers/controllers/workloads/fake"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/controllers/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	testNamespace      = "test-ns"
	testProcessGUID    = "test-process-guid"
	testProcessType    = "web"
	testProcessCommand = "test-process-command"
	testAppGUID        = "test-app-guid"
	testBuildGUID      = "test-build-guid"
	testPackageGUID    = "test-package-guid"
)

var _ = Describe("CFProcessReconciler Unit Tests", func() {
	var (
		fakeClient *fake.Client
		envBuilder *workloadsfakes.EnvBuilder

		cfBuild     *korifiv1alpha1.CFBuild
		cfProcess   *korifiv1alpha1.CFProcess
		cfApp       *korifiv1alpha1.CFApp
		runWorkload *korifiv1alpha1.RunWorkload
		routes      []korifiv1alpha1.CFRoute

		cfBuildError         error
		cfAppError           error
		cfProcessError       error
		runWorkloadError     error
		runWorkloadListError error
		routeListError       error

		cfProcessReconciler *CFProcessReconciler
		ctx                 context.Context
		req                 ctrl.Request

		reconcileErr error
	)

	BeforeEach(func() {
		fakeClient = new(fake.Client)

		envBuilder = new(workloadsfakes.EnvBuilder)

		cfApp = BuildCFAppCRObject(testAppGUID, testNamespace)
		cfAppError = nil
		cfBuild = BuildCFBuildObject(testBuildGUID, testNamespace, testPackageGUID, testAppGUID)
		UpdateCFBuildWithDropletStatus(cfBuild)
		cfBuildError = nil
		cfProcess = BuildCFProcessCRObject(testProcessGUID, testNamespace, testAppGUID, testProcessType, testProcessCommand)
		cfProcessError = nil

		runWorkload = nil
		runWorkloadError = nil
		runWorkloadListError = nil

		fakeClient.GetStub = func(_ context.Context, name types.NamespacedName, obj client.Object) error {
			// cast obj to find its kind
			switch obj := obj.(type) {
			case *korifiv1alpha1.CFProcess:
				cfProcess.DeepCopyInto(obj)
				return cfProcessError
			case *korifiv1alpha1.CFBuild:
				cfBuild.DeepCopyInto(obj)
				return cfBuildError
			case *korifiv1alpha1.CFApp:
				cfApp.DeepCopyInto(obj)
				return cfAppError
			case *korifiv1alpha1.RunWorkload:
				if runWorkload != nil && runWorkloadError == nil {
					runWorkload.DeepCopyInto(obj)
				}
				return runWorkloadError
			default:
				panic("TestClient Get provided a weird obj")
			}
		}

		fakeClient.ListStub = func(ctx context.Context, list client.ObjectList, option ...client.ListOption) error {
			switch listObj := list.(type) {
			case *korifiv1alpha1.RunWorkloadList:
				runWorkloadList := korifiv1alpha1.RunWorkloadList{Items: []korifiv1alpha1.RunWorkload{}}
				if runWorkload != nil {
					runWorkloadList.Items = append(runWorkloadList.Items, *runWorkload)
				}
				runWorkloadList.DeepCopyInto(listObj)
				return runWorkloadListError
			case *korifiv1alpha1.CFRouteList:
				routeList := korifiv1alpha1.CFRouteList{Items: routes}

				routeList.DeepCopyInto(listObj)
				return routeListError
			default:
				panic("TestClient Get provided a weird obj")
			}
		}

		// configure a CFProcessReconciler with the client
		Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
		cfProcessReconciler = NewCFProcessReconciler(
			fakeClient,
			scheme.Scheme,
			zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)),
			envBuilder,
		)
		ctx = context.Background()
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: testNamespace,
				Name:      testProcessGUID,
			},
		}
	})

	Describe("Process Controller Reconcile", func() {
		JustBeforeEach(func() {
			_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
		})

		It("succeeds", func() {
			Expect(reconcileErr).NotTo(HaveOccurred())
		})

		When("the CFApp is created with desired state stopped", func() {
			BeforeEach(func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StoppedState
			})

			It("does not attempt to create any new RunWorkloads", func() {
				Expect(fakeClient.CreateCallCount()).To(Equal(0), "Client.Create call count mismatch")
			})
		})

		When("the CFApp is updated from desired state STARTED to STOPPED", func() {
			BeforeEach(func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StoppedState
				runWorkload = &korifiv1alpha1.RunWorkload{
					ObjectMeta: metav1.ObjectMeta{
						Name:         testProcessGUID,
						GenerateName: "",
						Namespace:    testNamespace,
						Labels: map[string]string{
							korifiv1alpha1.CFProcessGUIDLabelKey: testProcessGUID,
						},
					},
					Spec: korifiv1alpha1.RunWorkloadSpec{
						GUID:        testProcessGUID,
						ProcessType: testProcessType,
						AppGUID:     testAppGUID,
						Image:       "test-image-ref",
						Instances:   0,
						MemoryMiB:   100,
						DiskMiB:     100,
						CPUWeight:   0,
					},
					Status: korifiv1alpha1.RunWorkloadStatus{
						ReadyReplicas: 0,
					},
				}
			})

			It("deletes any existing RunWorkloads for the CFApp", func() {
				Expect(fakeClient.DeleteCallCount()).To(Equal(1), "Client.Delete call count mismatch")
			})
		})

		When("the CFApp is started and there are existing routes matching", func() {
			const testPort = 1234

			BeforeEach(func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
				runWorkloadError = apierrors.NewNotFound(schema.GroupResource{}, "some-guid")

				routes = []korifiv1alpha1.CFRoute{
					{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: metav1.Time{
								Time: time.Now(),
							},
						},
						Status: korifiv1alpha1.CFRouteStatus{
							Destinations: []korifiv1alpha1.Destination{
								{
									GUID: "some-other-guid",
									Port: testPort + 1000,
									AppRef: corev1.LocalObjectReference{
										Name: testAppGUID,
									},
									ProcessType: testProcessType,
									Protocol:    "http1",
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: metav1.Time{
								Time: time.Now().Add(-5 * time.Second),
							},
						},
						Status: korifiv1alpha1.CFRouteStatus{
							Destinations: []korifiv1alpha1.Destination{
								{
									GUID: "some-guid",
									Port: testPort,
									AppRef: corev1.LocalObjectReference{
										Name: testAppGUID,
									},
									ProcessType: testProcessType,
									Protocol:    "http1",
								},
							},
						},
					},
				}
			})

			It("builds the environment for the app", func() {
				Expect(envBuilder.BuildEnvCallCount()).To(Equal(1))
				_, actualApp := envBuilder.BuildEnvArgsForCall(0)
				Expect(actualApp).To(Equal(cfApp))
			})

			It("chooses the oldest matching route", func() {
				_, obj, _ := fakeClient.CreateArgsForCall(0)
				returnedRunWorkload := obj.(*korifiv1alpha1.RunWorkload)
				Expect(returnedRunWorkload.Spec.Env).To(ContainElements(
					Equal(corev1.EnvVar{Name: "PORT", Value: strconv.Itoa(testPort)}),
					Equal(corev1.EnvVar{Name: "VCAP_APP_PORT", Value: strconv.Itoa(testPort)}),
				))
			})
		})

		When("the app is started", func() {
			BeforeEach(func() {
				cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			})

			When("fetch CFProcess returns an error", func() {
				BeforeEach(func() {
					cfProcessError = errors.New(failsOnPurposeErrorMessage)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFProcess returns a NotFoundError", func() {
				BeforeEach(func() {
					cfProcessError = apierrors.NewNotFound(schema.GroupResource{}, cfProcess.Name)
				})

				It("doesn't return an error", func() {
					Expect(reconcileErr).NotTo(HaveOccurred())
				})
			})

			When("fetch CFApp returns an error", func() {
				BeforeEach(func() {
					cfAppError = errors.New(failsOnPurposeErrorMessage)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("fetch CFBuild returns an error", func() {
				BeforeEach(func() {
					cfBuildError = errors.New(failsOnPurposeErrorMessage)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})

			When("CFBuild does not have a build droplet status", func() {
				BeforeEach(func() {
					cfBuild.Status.Droplet = nil
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError("no build droplet status on CFBuild"))
				})
			})

			When("building the RunWorkload environment fails", func() {
				BeforeEach(func() {
					envBuilder.BuildEnvReturns(nil, errors.New("build-env-err"))
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(ContainSubstring("build-env-err")))
				})
			})

			When("fetch RunWorkloadList returns an error", func() {
				BeforeEach(func() {
					runWorkloadListError = errors.New(failsOnPurposeErrorMessage)
				})

				It("returns an error", func() {
					Expect(reconcileErr).To(MatchError(failsOnPurposeErrorMessage))
				})
			})
		})
	})

	When("generating RunWorkload CPU weight parameters", func() {
		BeforeEach(func() {
			cfApp.Spec.DesiredState = korifiv1alpha1.StartedState
			runWorkloadError = apierrors.NewNotFound(schema.GroupResource{}, "")
		})

		DescribeTable("matches expected output",
			func(processMemoryMB int64, outputCTPUWeight uint8) {
				cfProcess.Spec.MemoryMB = processMemoryMB

				_, reconcileErr = cfProcessReconciler.Reconcile(ctx, req)
				Expect(reconcileErr).To(Succeed())

				Expect(fakeClient.CreateCallCount()).To(BeNumerically(">=", 1))
				_, createObj, _ := fakeClient.CreateArgsForCall(0)
				createdRunWorkload, ok := createObj.(*korifiv1alpha1.RunWorkload)
				Expect(ok).To(BeTrue(), "client Create() object cooerce to eirini.RunWorkload failed")
				Expect(createdRunWorkload.Spec.CPUWeight).To(Equal(outputCTPUWeight))
			},
			Entry("Memory is zero", int64(0), uint8(100*128/8192)),
			Entry("Memory is less than 8192", int64(4096), uint8(100*4096/8192)),
			Entry("Memory is greater than 8192", int64(16384), uint8(100)),
		)
	})
})
