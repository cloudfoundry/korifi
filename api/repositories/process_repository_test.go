package repositories_test

import (
	"context"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ProcessRepo", func() {
	var (
		processRepo *repositories.ProcessRepo
		org         *korifiv1alpha1.CFOrg
		space       *korifiv1alpha1.CFSpace
		appGUID     string
		cfProcess   *korifiv1alpha1.CFProcess
	)

	BeforeEach(func() {
		processRepo = repositories.NewProcessRepo(klient)
		org = createOrgWithCleanup(ctx, uuid.NewString())
		space = createSpaceWithCleanup(ctx, org.Name, uuid.NewString())

		appGUID = uuid.NewString()
		cfProcess = &korifiv1alpha1.CFProcess{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: space.Name,
			},
			Spec: korifiv1alpha1.CFProcessSpec{
				AppRef: corev1.LocalObjectReference{
					Name: appGUID,
				},
				ProcessType: "web",
				HealthCheck: korifiv1alpha1.HealthCheck{
					Type: "process",
					Data: korifiv1alpha1.HealthCheckData{
						HTTPEndpoint:             "/healthz",
						InvocationTimeoutSeconds: 5,
						TimeoutSeconds:           6,
					},
				},
				DesiredInstances: tools.PtrTo[int32](1),
				MemoryMB:         500,
				DiskQuotaMB:      512,
			},
		}
		Expect(k8sClient.Create(ctx, cfProcess)).To(Succeed())

		Expect(k8s.Patch(ctx, k8sClient, cfProcess, func() {
			cfProcess.Status.InstancesStatus = map[string]korifiv1alpha1.InstanceStatus{
				"1": {
					State: korifiv1alpha1.InstanceStateDown,
				},
			}
		})).To(Succeed())
	})

	Describe("GetProcess", func() {
		var (
			getProcessGUID string
			processRecord  repositories.ProcessRecord
			getErr         error
		)

		BeforeEach(func() {
			getProcessGUID = cfProcess.Name
		})

		JustBeforeEach(func() {
			processRecord, getErr = processRepo.GetProcess(ctx, authInfo, getProcessGUID)
		})

		It("returns a forbidden error", func() {
			Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user has permission to get the process", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns a Process record for the Process CR we request", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(processRecord.GUID).To(Equal(cfProcess.Name))
				Expect(processRecord.SpaceGUID).To(Equal(space.Name))
				Expect(processRecord.AppGUID).To(Equal(appGUID))
				Expect(processRecord.Type).To(Equal(cfProcess.Spec.ProcessType))
				Expect(processRecord.Command).To(Equal(cfProcess.Spec.Command))
				Expect(processRecord.DesiredInstances).To(BeEquivalentTo(1))
				Expect(processRecord.MemoryMB).To(BeEquivalentTo(500))
				Expect(processRecord.DiskQuotaMB).To(BeEquivalentTo(512))
				Expect(processRecord.HealthCheck.Type).To(Equal("process"))
				Expect(processRecord.HealthCheck.Data.InvocationTimeoutSeconds).To(BeEquivalentTo(5))
				Expect(processRecord.HealthCheck.Data.TimeoutSeconds).To(BeEquivalentTo(6))
				Expect(processRecord.HealthCheck.Data.HTTPEndpoint).To(Equal("/healthz"))
				Expect(processRecord.InstancesStatus).To(Equal(map[string]korifiv1alpha1.InstanceStatus{
					"1": {
						State: korifiv1alpha1.InstanceStateDown,
					},
				}))

				Expect(processRecord.Relationships()).To(Equal(map[string]string{
					"app": appGUID,
				}))
			})

			When("desired instances are not set on the process spec", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, cfProcess, func() {
						cfProcess.Spec.DesiredInstances = nil
					})).To(Succeed())
				})

				It("returns zero desired instances on the record", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(processRecord.DesiredInstances).To(BeZero())
				})
			})
		})

		When("the privileged list call fails", func() {
			var cancelFn context.CancelFunc

			BeforeEach(func() {
				ctx, cancelFn = context.WithDeadline(ctx, time.Now().Add(-1*time.Minute))
			})

			AfterEach(func() {
				cancelFn()
			})

			It("returns an untyped error", func() {
				Expect(getErr).To(HaveOccurred())
				Expect(getErr).To(MatchError(ContainSubstring("failed to list Process")))
			})
		})

		When("duplicate Processes exist across namespaces with the same GUIDs", func() {
			BeforeEach(func() {
				app2GUID := uuid.NewString()

				anotherNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: uuid.NewString()}}
				Expect(k8sClient.Create(ctx, anotherNamespace)).To(Succeed())

				Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFProcess{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfProcess.Name,
						Namespace: anotherNamespace.Name,
					},
					Spec: korifiv1alpha1.CFProcessSpec{
						AppRef: corev1.LocalObjectReference{
							Name: app2GUID,
						},
						ProcessType: "web",
						HealthCheck: korifiv1alpha1.HealthCheck{
							Type: "process",
						},
					},
				})).To(Succeed())
			})

			It("returns an untyped error", func() {
				Expect(getErr).To(HaveOccurred())
				Expect(getErr).To(MatchError(ContainSubstring("get-process duplicate records exist")))
			})
		})

		When("no matching processes exist", func() {
			BeforeEach(func() {
				getProcessGUID = "i don't exist"
			})

			It("returns a not found error", func() {
				Expect(getErr).To(HaveOccurred())
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("ListProcesses", func() {
		var (
			space2     *korifiv1alpha1.CFSpace
			cfProcess2 *korifiv1alpha1.CFProcess

			listProcessesMessage repositories.ListProcessesMessage
			processes            []repositories.ProcessRecord
		)

		BeforeEach(func() {
			space2 = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space2"))
			cfProcess2 = &korifiv1alpha1.CFProcess{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space2.Name,
				},
				Spec: korifiv1alpha1.CFProcessSpec{
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
					ProcessType: "web",
					HealthCheck: korifiv1alpha1.HealthCheck{
						Type: "process",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfProcess2)).To(Succeed())

			listProcessesMessage = repositories.ListProcessesMessage{
				AppGUIDs: []string{appGUID},
			}
		})

		JustBeforeEach(func() {
			var err error
			processes, err = processRepo.ListProcesses(ctx, authInfo, listProcessesMessage)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an empty list to unauthorized users", func() {
			Expect(processes).To(BeEmpty())
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space2.Name)
			})

			It("returns Process records for the AppGUID we request", func() {
				Expect(processes).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfProcess.Name)}),
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfProcess2.Name)}),
				))
			})

			Describe("filtering", func() {
				var fakeKlient *fake.Klient

				BeforeEach(func() {
					fakeKlient = new(fake.Klient)
					processRepo = repositories.NewProcessRepo(fakeKlient)
				})

				Describe("filter parameters to list options", func() {
					BeforeEach(func() {
						listProcessesMessage = repositories.ListProcessesMessage{
							AppGUIDs:     []string{"app-guid", "another-app-guid"},
							SpaceGUIDs:   []string{"space-guid", "another-space-guid"},
							ProcessTypes: []string{"web", "worker"},
						}
					})

					It("translates filter parameters to klient list options", func() {
						Expect(fakeKlient.ListCallCount()).To(Equal(1))
						_, _, listOptions := fakeKlient.ListArgsForCall(0)
						Expect(listOptions).To(ConsistOf(
							repositories.WithLabelIn(korifiv1alpha1.CFAppGUIDLabelKey, []string{"app-guid", "another-app-guid"}),
							repositories.WithLabelIn(korifiv1alpha1.SpaceGUIDKey, []string{"space-guid", "another-space-guid"}),
							repositories.WithLabelIn(korifiv1alpha1.CFProcessTypeLabelKey, []string{"web", "worker"}),
						))
					})
				})
			})

			When("no Processes exist for an app", func() {
				BeforeEach(func() {
					listProcessesMessage.AppGUIDs = []string{uuid.NewString()}
				})

				It("returns an empty list", func() {
					Expect(processes).To(BeEmpty())
				})
			})
		})
	})

	Describe("ScaleProcess", func() {
		var (
			scaleProcessMessage repositories.ScaleProcessMessage
			scaleErr            error

			scaledRecord repositories.ProcessRecord
		)

		BeforeEach(func() {
			scaleProcessMessage = repositories.ScaleProcessMessage{
				GUID:      cfProcess.Name,
				SpaceGUID: space.Name,
				ProcessScaleValues: repositories.ProcessScaleValues{
					Instances: tools.PtrTo[int32](7),
					MemoryMB:  tools.PtrTo[int64](900),
					DiskMB:    tools.PtrTo[int64](80),
				},
			}
		})

		JustBeforeEach(func() {
			scaledRecord, scaleErr = processRepo.ScaleProcess(ctx, authInfo, scaleProcessMessage)
		})

		It("returns a forbidden error to unauthorized users", func() {
			Expect(scaleErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user has the SpaceDeveloper role", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("scales the process", func() {
				Expect(scaleErr).NotTo(HaveOccurred())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfProcess), cfProcess)).To(Succeed())

				Expect(scaledRecord.DesiredInstances).To(BeEquivalentTo(7))
				Expect(cfProcess.Spec.DesiredInstances).To(PointTo(BeEquivalentTo(7)))

				Expect(scaledRecord.DiskQuotaMB).To(BeEquivalentTo(80))
				Expect(cfProcess.Spec.DiskQuotaMB).To(BeEquivalentTo(80))

				Expect(scaledRecord.MemoryMB).To(BeEquivalentTo(900))
				Expect(cfProcess.Spec.MemoryMB).To(BeEquivalentTo(900))
			})

			When("process scale values are not specified", func() {
				BeforeEach(func() {
					scaleProcessMessage.ProcessScaleValues = repositories.ProcessScaleValues{}
				})

				It("is noop", func() {
					Expect(scaleErr).NotTo(HaveOccurred())
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfProcess), cfProcess)).To(Succeed())

					Expect(scaledRecord.DesiredInstances).To(BeEquivalentTo(1))
					Expect(cfProcess.Spec.DesiredInstances).To(PointTo(BeEquivalentTo(1)))

					Expect(scaledRecord.DiskQuotaMB).To(BeEquivalentTo(512))
					Expect(cfProcess.Spec.DiskQuotaMB).To(BeEquivalentTo(512))

					Expect(scaledRecord.MemoryMB).To(BeEquivalentTo(500))
					Expect(cfProcess.Spec.MemoryMB).To(BeEquivalentTo(500))
				})
			})

			When("the process does not exist", func() {
				BeforeEach(func() {
					scaleProcessMessage.GUID = "i-dont-exist"
				})

				It("returns an error", func() {
					Expect(scaleErr).To(HaveOccurred())
				})
			})
		})
	})

	Describe("CreateProcess", func() {
		var (
			createErr    error
			anotherSpace *korifiv1alpha1.CFSpace
		)

		BeforeEach(func() {
			anotherSpace = createSpaceWithCleanup(ctx, org.Name, uuid.NewString())
		})

		JustBeforeEach(func() {
			createErr = processRepo.CreateProcess(ctx, authInfo, repositories.CreateProcessMessage{
				AppGUID:     appGUID,
				SpaceGUID:   anotherSpace.Name,
				Type:        "web",
				Command:     "start-web",
				DiskQuotaMB: 123,
				HealthCheck: repositories.HealthCheck{
					Type: "http",
					Data: repositories.HealthCheckData{
						HTTPEndpoint:             "/healthz",
						InvocationTimeoutSeconds: 20,
						TimeoutSeconds:           10,
					},
				},
				DesiredInstances: tools.PtrTo[int32](42),
				MemoryMB:         456,
			})
		})

		It("returns a forbidden error", func() {
			Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("user has permissions", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, anotherSpace.Name)
			})

			It("creates a CFProcess resource", func() {
				Expect(createErr).NotTo(HaveOccurred())
				var list korifiv1alpha1.CFProcessList
				Expect(k8sClient.List(ctx, &list, client.InNamespace(anotherSpace.Name))).To(Succeed())
				Expect(list.Items).To(HaveLen(1))

				process := list.Items[0]
				Expect(process.Spec).To(Equal(korifiv1alpha1.CFProcessSpec{
					AppRef:      corev1.LocalObjectReference{Name: appGUID},
					ProcessType: "web",
					Command:     "start-web",
					HealthCheck: korifiv1alpha1.HealthCheck{
						Type: "http",
						Data: korifiv1alpha1.HealthCheckData{
							HTTPEndpoint:             "/healthz",
							InvocationTimeoutSeconds: 20,
							TimeoutSeconds:           10,
						},
					},
					DesiredInstances: tools.PtrTo[int32](42),
					MemoryMB:         456,
					DiskQuotaMB:      123,
				}))
			})

			When("a process with that process type already exists", func() {
				BeforeEach(func() {
					Expect(processRepo.CreateProcess(ctx, authInfo, repositories.CreateProcessMessage{
						AppGUID:   appGUID,
						SpaceGUID: anotherSpace.Name,
						Type:      "web",
					})).To(Succeed())
				})

				It("returns an already exists error", func() {
					Expect(createErr).To(MatchError(ContainSubstring("already exists")))
				})
			})
		})
	})

	Describe("GetAppRevisionForProcess", func() {
		var (
			appRevision string
			getErr      error
		)
		BeforeEach(func() {
			app := &korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appGUID,
					Namespace: space.Name,
					Annotations: map[string]string{
						"korifi.cloudfoundry.org/app-rev": "revision",
					},
				},
				Spec: korifiv1alpha1.CFAppSpec{
					DesiredState: "STOPPED",
					DisplayName:  "app1",
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: korifiv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
		})
		JustBeforeEach(func() {
			appRevision, getErr = processRepo.GetAppRevision(ctx, authInfo, appGUID)
		})

		It("returns a forbidden error", func() {
			Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user has permission to get the process", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns the App Revision", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(appRevision).To(ContainSubstring("revision"))
			})
		})

		When("there is no matching app", func() {
			BeforeEach(func() {
				appGUID = "i don't exist"
			})

			It("returns a not found error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("PatchProcess", func() {
		var message repositories.PatchProcessMessage

		BeforeEach(func() {
			message = repositories.PatchProcessMessage{
				ProcessGUID:                         cfProcess.Name,
				SpaceGUID:                           space.Name,
				Command:                             tools.PtrTo("start-web"),
				HealthCheckType:                     tools.PtrTo("http"),
				HealthCheckHTTPEndpoint:             tools.PtrTo("/healthz"),
				HealthCheckInvocationTimeoutSeconds: tools.PtrTo(int32(20)),
				HealthCheckTimeoutSeconds:           tools.PtrTo(int32(10)),
				DesiredInstances:                    tools.PtrTo[int32](42),
				MemoryMB:                            tools.PtrTo(int64(456)),
				DiskQuotaMB:                         tools.PtrTo(int64(123)),
				MetadataPatch: &repositories.MetadataPatch{
					Labels:      map[string]*string{"fool": tools.PtrTo("fool")},
					Annotations: map[string]*string{"fooa": tools.PtrTo("fooa")},
				},
			}
		})

		It("returns a forbidden error to unauthorized users", func() {
			_, err := processRepo.PatchProcess(ctx, authInfo, message)
			Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("user has permission", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("updates the process", func() {
				updatedProcessRecord, err := processRepo.PatchProcess(ctx, authInfo, message)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedProcessRecord.GUID).To(Equal(cfProcess.Name))

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfProcess), cfProcess)).To(Succeed())
				Expect(cfProcess.Labels).To(HaveKeyWithValue("fool", "fool"))
				Expect(cfProcess.Annotations).To(HaveKeyWithValue("fooa", "fooa"))
				Expect(cfProcess.Spec).To(MatchFields(IgnoreExtras, Fields{
					"Command": Equal("start-web"),
					"HealthCheck": MatchAllFields(Fields{
						"Type": BeEquivalentTo("http"),
						"Data": MatchAllFields(Fields{
							"HTTPEndpoint":             BeEquivalentTo("/healthz"),
							"InvocationTimeoutSeconds": BeEquivalentTo(20),
							"TimeoutSeconds":           BeEquivalentTo(10),
						}),
					}),
					"DesiredInstances": PointTo(BeEquivalentTo(42)),
					"MemoryMB":         BeEquivalentTo(456),
					"DiskQuotaMB":      BeEquivalentTo(123),
				}))
			})
		})
	})
})
