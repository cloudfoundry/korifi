package repositories_test

import (
	"context"
	"errors"
	"slices"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fakeawaiter"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	osbapifake "code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi/fake"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"code.cloudfoundry.org/korifi/api/repositories/fake"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ServiceBindingRepo", func() {
	var (
		repo  *repositories.ServiceBindingRepo
		org   *korifiv1alpha1.CFOrg
		space *korifiv1alpha1.CFSpace

		appGUID                 string
		bindingName             *string
		bindingConditionAwaiter *fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFServiceBinding,
			korifiv1alpha1.CFServiceBindingList,
			*korifiv1alpha1.CFServiceBindingList,
		]
		appConditionAwaiter *fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFApp,
			korifiv1alpha1.CFAppList,
			*korifiv1alpha1.CFAppList,
		]
		brokerClient *osbapifake.BrokerClient
	)

	BeforeEach(func() {
		bindingConditionAwaiter = &fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFServiceBinding,
			korifiv1alpha1.CFServiceBindingList,
			*korifiv1alpha1.CFServiceBindingList,
		]{}
		appConditionAwaiter = &fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFApp,
			korifiv1alpha1.CFAppList,
			*korifiv1alpha1.CFAppList,
		]{}

		brokerClient = new(osbapifake.BrokerClient)
		brokerClientFactory := new(osbapifake.BrokerClientFactory)
		brokerClientFactory.CreateClientReturns(brokerClient, nil)

		paramsClient := repositories.NewServiceBrokerClient(
			brokerClientFactory,
			k8sClient,
			rootNamespace,
		)

		repo = repositories.NewServiceBindingRepo(
			spaceScopedKlient,
			bindingConditionAwaiter,
			appConditionAwaiter,
			paramsClient,
		)

		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space1"))
		appGUID = prefixedGUID("app")
		bindingName = nil

		app := &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appGUID,
				Namespace: space.Name,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  "some-app",
				DesiredState: korifiv1alpha1.AppState(repositories.StoppedState),
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
					Data: korifiv1alpha1.LifecycleData{
						Buildpacks: []string{},
						Stack:      "",
					},
				},
			},
		}
		Expect(
			k8sClient.Create(ctx, app),
		).To(Succeed())
	})

	Describe("GetState", func() {
		var (
			cfServiceBinding *korifiv1alpha1.CFServiceBinding
			state            repositories.ResourceState
			stateErr         error
		)
		BeforeEach(func() {
			cfServiceBinding = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "CFServiceInstance",
						APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
						Name:       uuid.NewString(),
					},
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
					Type: korifiv1alpha1.CFServiceBindingTypeApp,
				},
			}
			Expect(
				k8sClient.Create(ctx, cfServiceBinding),
			).To(Succeed())
		})

		JustBeforeEach(func() {
			state, stateErr = repo.GetState(ctx, authInfo, cfServiceBinding.Name)
		})

		It("returns a forbidden error", func() {
			Expect(stateErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user can get CFServiceBinding", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, cfServiceBinding.Namespace)
			})

			It("returns unknown state", func() {
				Expect(stateErr).NotTo(HaveOccurred())
				Expect(state).To(Equal(repositories.ResourceStateUnknown))
			})

			When("the service binding is ready", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, cfServiceBinding, func() {
						meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
							Type:    korifiv1alpha1.StatusConditionReady,
							Status:  metav1.ConditionTrue,
							Message: "Ready",
							Reason:  "Ready",
						})
						cfServiceBinding.Status.ObservedGeneration = cfServiceBinding.Generation
					})).To(Succeed())
				})

				It("returns ready state", func() {
					Expect(stateErr).NotTo(HaveOccurred())
					Expect(state).To(Equal(repositories.ResourceStateReady))
				})

				When("the ready status is stale ", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, cfServiceBinding, func() {
							cfServiceBinding.Status.ObservedGeneration = -1
						})).To(Succeed())
					})

					It("returns unknown state", func() {
						Expect(stateErr).NotTo(HaveOccurred())
						Expect(state).To(Equal(repositories.ResourceStateUnknown))
					})
				})
			})
		})
	})

	Describe("GetDeletedAt", func() {
		var (
			cfServiceBinding *korifiv1alpha1.CFServiceBinding
			deletionTime     *time.Time
			getErr           error
		)

		BeforeEach(func() {
			cfServiceBinding = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Type: korifiv1alpha1.CFServiceBindingTypeApp,
				},
			}

			Expect(k8sClient.Create(ctx, cfServiceBinding)).To(Succeed())
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
		})

		JustBeforeEach(func() {
			deletionTime, getErr = repo.GetDeletedAt(ctx, authInfo, cfServiceBinding.Name)
		})

		It("returns nil", func() {
			Expect(getErr).NotTo(HaveOccurred())
			Expect(deletionTime).To(BeNil())
		})

		When("the binding is being deleted", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, k8sClient, cfServiceBinding, func() {
					cfServiceBinding.Finalizers = append(cfServiceBinding.Finalizers, "foo")
				})).To(Succeed())

				Expect(k8sClient.Delete(ctx, cfServiceBinding)).To(Succeed())
			})

			It("returns the deletion time", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(deletionTime).To(PointTo(BeTemporally("~", time.Now(), time.Minute)))
			})
		})

		When("the binding isn't found", func() {
			BeforeEach(func() {
				Expect(k8sClient.Delete(ctx, cfServiceBinding)).To(Succeed())
			})

			It("errors", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("CreateUserProvidedServiceBinding", func() {
		var (
			cfServiceInstance    *korifiv1alpha1.CFServiceInstance
			serviceBindingRecord repositories.ServiceBindingRecord
			bindingGUID          string
			createErr            error
		)

		BeforeEach(func() {
			cfServiceInstance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space.Name,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					Type: korifiv1alpha1.UserProvidedType,
				},
			}
			Expect(
				k8sClient.Create(ctx, cfServiceInstance),
			).To(Succeed())

			bindingConditionAwaiter.AwaitConditionStub = func(ctx context.Context, _ repositories.Klient, object client.Object, _ string) (*korifiv1alpha1.CFServiceBinding, error) {
				cfServiceBinding, ok := object.(*korifiv1alpha1.CFServiceBinding)
				Expect(ok).To(BeTrue())

				Expect(k8s.Patch(ctx, k8sClient, cfServiceBinding, func() {
					cfServiceBinding.Status.MountSecretRef.Name = "service-secret-name"
					meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
						Type:    korifiv1alpha1.StatusConditionReady,
						Status:  metav1.ConditionTrue,
						Reason:  "blah",
						Message: "blah",
					})
				})).To(Succeed())
				bindingGUID = cfServiceBinding.Name

				return cfServiceBinding, nil
			}

			appConditionAwaiter.AwaitStateStub = func(ctx context.Context, _ repositories.Klient, object client.Object, checkState func(a *korifiv1alpha1.CFApp) error) (*korifiv1alpha1.CFApp, error) {
				cfApp, ok := object.(*korifiv1alpha1.CFApp)
				Expect(ok).To(BeTrue())

				Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
					cfApp.Status.ObservedGeneration = cfApp.Generation
					cfApp.Status.ServiceBindings = []korifiv1alpha1.ServiceBinding{{
						GUID: bindingGUID,
					}}
				})).To(Succeed())

				err := checkState(cfApp)
				return cfApp, err
			}

			bindingName = nil
		})

		JustBeforeEach(func() {
			serviceBindingRecord, createErr = repo.CreateServiceBinding(ctx, authInfo, repositories.CreateServiceBindingMessage{
				Type:                korifiv1alpha1.CFServiceBindingTypeApp,
				ServiceInstanceGUID: cfServiceInstance.Name,
				AppGUID:             appGUID,
				SpaceGUID:           space.Name,
				Name:                bindingName,
			})
		})

		It("returns a unprocessable entity error", func() {
			Expect(createErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
		})

		When("the user can create CFServiceBindings in the Space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates a new CFServiceBinding resource and returns a record", func() {
				Expect(createErr).NotTo(HaveOccurred())
				Expect(serviceBindingRecord.Type).To(Equal(korifiv1alpha1.CFServiceBindingTypeApp))
				Expect(serviceBindingRecord.GUID).To(matchers.BeValidUUID())
				Expect(serviceBindingRecord.Name).To(BeNil())
				Expect(serviceBindingRecord.AppGUID).To(Equal(appGUID))
				Expect(serviceBindingRecord.ServiceInstanceGUID).To(Equal(cfServiceInstance.Name))
				Expect(serviceBindingRecord.SpaceGUID).To(Equal(space.Name))
				Expect(serviceBindingRecord.CreatedAt).NotTo(BeZero())
				Expect(serviceBindingRecord.UpdatedAt).NotTo(BeNil())

				Expect(serviceBindingRecord.Relationships()).To(Equal(map[string]string{
					"app":              appGUID,
					"service_instance": cfServiceInstance.Name,
				}))

				serviceBinding := new(korifiv1alpha1.CFServiceBinding)
				Expect(
					k8sClient.Get(ctx, types.NamespacedName{Name: serviceBindingRecord.GUID, Namespace: space.Name}, serviceBinding),
				).To(Succeed())

				Expect(serviceBinding.Labels).To(HaveKeyWithValue("servicebinding.io/provisioned-service", "true"))
				Expect(serviceBinding.Spec).To(Equal(
					korifiv1alpha1.CFServiceBindingSpec{
						DisplayName: nil,
						Service: corev1.ObjectReference{
							Kind:       "CFServiceInstance",
							APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
							Name:       cfServiceInstance.Name,
						},
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
						Type: korifiv1alpha1.CFServiceBindingTypeApp,
					},
				))
			})

			It("awaits the binding to become ready", func() {
				Expect(createErr).NotTo(HaveOccurred())

				cfServiceBinding := new(korifiv1alpha1.CFServiceBinding)
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: space.Name, Name: serviceBindingRecord.GUID}, cfServiceBinding)).To(Succeed())

				Expect(bindingConditionAwaiter.AwaitConditionCallCount()).To(Equal(1))
				obj, conditionType := bindingConditionAwaiter.AwaitConditionArgsForCall(0)
				Expect(obj.GetName()).To(Equal(cfServiceBinding.Name))
				Expect(obj.GetNamespace()).To(Equal(space.Name))
				Expect(conditionType).To(Equal(korifiv1alpha1.StatusConditionReady))
			})

			It("awaits the binding to become available in the app status", func() {
				Expect(createErr).NotTo(HaveOccurred())

				cfServiceBinding := new(korifiv1alpha1.CFServiceBinding)
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: space.Name, Name: serviceBindingRecord.GUID}, cfServiceBinding)).To(Succeed())

				Expect(appConditionAwaiter.AwaitStateCallCount()).To(Equal(1))
				actualObj, _ := appConditionAwaiter.AwaitStateArgsForCall(0)
				Expect(actualObj.GetName()).To(Equal(appGUID))
			})

			When("the binding never becomes ready", func() {
				BeforeEach(func() {
					bindingConditionAwaiter.AwaitConditionReturns(&korifiv1alpha1.CFServiceBinding{}, errors.New("time-out-err"))
				})

				It("errors", func() {
					Expect(createErr).To(MatchError(ContainSubstring("time-out-err")))
				})
			})

			When("the binding never makes it to the app status", func() {
				BeforeEach(func() {
					appConditionAwaiter.AwaitStateStub = func(ctx context.Context, _ repositories.Klient, object client.Object, checkState func(a *korifiv1alpha1.CFApp) error) (*korifiv1alpha1.CFApp, error) {
						return nil, errors.New("time-out-err")
					}
				})

				It("errors", func() {
					Expect(createErr).To(MatchError(ContainSubstring("time-out-err")))
				})
			})

			When("the app status is outdated", func() {
				BeforeEach(func() {
					appConditionAwaiter.AwaitStateStub = func(ctx context.Context, _ repositories.Klient, object client.Object, checkState func(a *korifiv1alpha1.CFApp) error) (*korifiv1alpha1.CFApp, error) {
						cfApp, ok := object.(*korifiv1alpha1.CFApp)
						Expect(ok).To(BeTrue())

						Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
							cfApp.Status.ServiceBindings = []korifiv1alpha1.ServiceBinding{{
								GUID: bindingGUID,
							}}
						})).To(Succeed())

						err := checkState(cfApp)
						return cfApp, err
					}
				})

				It("errors", func() {
					Expect(createErr).To(MatchError(ContainSubstring("outdated")))
				})
			})

			When("the app does not exist", func() {
				BeforeEach(func() {
					appGUID = "i-do-not-exits"
				})

				It("reuturns an UnprocessableEntity error", func() {
					Expect(createErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
				})
			})

			When("The service binding has a name", func() {
				BeforeEach(func() {
					tempName := "some-name-for-a-binding"
					bindingName = &tempName
				})

				It("creates the binding with the specified name", func() {
					Expect(serviceBindingRecord.Name).To(Equal(bindingName))
				})
			})
		})
	})

	Describe("binding record last operation", func() {
		var (
			cfServiceBinding     *korifiv1alpha1.CFServiceBinding
			serviceBindingRecord repositories.ServiceBindingRecord
		)

		BeforeEach(func() {
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			cfServiceBinding = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "CFServiceInstance",
						APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
						Name:       uuid.NewString(),
					},
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
					Type: korifiv1alpha1.CFServiceBindingTypeApp,
				},
			}
			Expect(
				k8sClient.Create(ctx, cfServiceBinding),
			).To(Succeed())
		})

		JustBeforeEach(func() {
			var err error
			serviceBindingRecord, err = repo.GetServiceBinding(ctx, authInfo, cfServiceBinding.Name)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns initial last operation", func() {
			Expect(serviceBindingRecord.LastOperation.Type).To(Equal("create"))
			Expect(serviceBindingRecord.LastOperation.State).To(Equal("initial"))
			Expect(serviceBindingRecord.LastOperation.CreatedAt).To(Equal(serviceBindingRecord.CreatedAt))
			Expect(serviceBindingRecord.LastOperation.UpdatedAt).To(PointTo(BeTemporally("~", serviceBindingRecord.CreatedAt, time.Second)))
		})

		When("the binding is being deleted", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, k8sClient, cfServiceBinding, func() {
					cfServiceBinding.Finalizers = append(cfServiceBinding.Finalizers, "do-not-delete-me")
				})).To(Succeed())

				Expect(k8sClient.Delete(ctx, cfServiceBinding)).To(Succeed())
			})

			It("returns delete last operation", func() {
				Expect(serviceBindingRecord.LastOperation.Type).To(Equal("delete"))
				Expect(serviceBindingRecord.LastOperation.State).To(Equal("in progress"))
				Expect(serviceBindingRecord.LastOperation.CreatedAt).To(BeTemporally("~", time.Now(), 5*time.Second))
			})
		})

		When("binding has succeeded", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, cfServiceBinding, func() {
					meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
						Type:               korifiv1alpha1.StatusConditionReady,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(time.UnixMilli(1000)),
						Reason:             "Ready",
					})
				})).To(Succeed())
			})

			It("returns succeeded last operation", func() {
				Expect(serviceBindingRecord.LastOperation.Type).To(Equal("create"))
				Expect(serviceBindingRecord.LastOperation.State).To(Equal("succeeded"))
				Expect(serviceBindingRecord.LastOperation.CreatedAt).To(Equal(serviceBindingRecord.CreatedAt))
				Expect(serviceBindingRecord.LastOperation.UpdatedAt).To(Equal(serviceBindingRecord.UpdatedAt))
			})
		})

		When("binding is in progress", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, cfServiceBinding, func() {
					meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
						Type:               korifiv1alpha1.StatusConditionReady,
						Status:             metav1.ConditionFalse,
						LastTransitionTime: metav1.NewTime(time.UnixMilli(1000)),
						Reason:             "NotReady",
					})
				})).To(Succeed())
			})

			It("returns in progress last operation", func() {
				Expect(serviceBindingRecord.LastOperation.Type).To(Equal("create"))
				Expect(serviceBindingRecord.LastOperation.State).To(Equal("in progress"))
				Expect(serviceBindingRecord.LastOperation.CreatedAt).To(Equal(serviceBindingRecord.CreatedAt))
				Expect(serviceBindingRecord.LastOperation.UpdatedAt).To(PointTo(Equal(time.UnixMilli(1000))))
			})
		})

		When("binding has failed", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, cfServiceBinding, func() {
					meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
						Type:               korifiv1alpha1.StatusConditionReady,
						Status:             metav1.ConditionFalse,
						LastTransitionTime: metav1.NewTime(time.UnixMilli(2000)),
						Reason:             "Failed",
					})

					meta.SetStatusCondition(&cfServiceBinding.Status.Conditions, metav1.Condition{
						Type:   korifiv1alpha1.BindingFailedCondition,
						Status: metav1.ConditionTrue,
						Reason: "Failed",
					})
				})).To(Succeed())
			})
			It("returns failed last operation", func() {
				Expect(serviceBindingRecord.LastOperation.Type).To(Equal("create"))
				Expect(serviceBindingRecord.LastOperation.State).To(Equal("failed"))
				Expect(serviceBindingRecord.LastOperation.CreatedAt).To(Equal(serviceBindingRecord.CreatedAt))
				Expect(serviceBindingRecord.LastOperation.UpdatedAt).To(PointTo(Equal(time.UnixMilli(2000))))
			})
		})
	})

	Describe("CreateManagedServiceBinding", func() {
		var (
			cfServiceInstance    *korifiv1alpha1.CFServiceInstance
			serviceBindingRecord repositories.ServiceBindingRecord
			createErr            error
			createMsg            repositories.CreateServiceBindingMessage
			serviceBindingName   string = "service-binding-name"
		)

		BeforeEach(func() {
			cfServiceInstance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space.Name,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					Type: korifiv1alpha1.ManagedType,
				},
			}
			Expect(
				k8sClient.Create(ctx, cfServiceInstance),
			).To(Succeed())

			createMsg = repositories.CreateServiceBindingMessage{
				Type:                korifiv1alpha1.CFServiceBindingTypeApp,
				Name:                nil,
				ServiceInstanceGUID: cfServiceInstance.Name,
				AppGUID:             appGUID,
				SpaceGUID:           space.Name,
				Parameters: map[string]any{
					"p1": "p1-value",
				},
			}
		})

		JustBeforeEach(func() {
			serviceBindingRecord, createErr = repo.CreateServiceBinding(ctx, authInfo, createMsg)
		})

		When("the user is not allowed to create CFServiceBindings", func() {
			It("returns a forbidden error", func() {
				Expect(createErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
			})
		})

		When("the user is allowed to create CFServiceBindings in the Space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates a new CFServiceBinding resource and returns a record", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(serviceBindingRecord.GUID).To(matchers.BeValidUUID())
				Expect(serviceBindingRecord.Type).To(Equal("app"))
				Expect(serviceBindingRecord.Name).To(BeNil())
				Expect(serviceBindingRecord.AppGUID).To(Equal(appGUID))
				Expect(serviceBindingRecord.ServiceInstanceGUID).To(Equal(cfServiceInstance.Name))
				Expect(serviceBindingRecord.SpaceGUID).To(Equal(space.Name))
				Expect(serviceBindingRecord.CreatedAt).NotTo(BeZero())
				Expect(serviceBindingRecord.UpdatedAt).NotTo(BeNil())

				Expect(serviceBindingRecord.Relationships()).To(Equal(map[string]string{
					"app":              appGUID,
					"service_instance": cfServiceInstance.Name,
				}))

				serviceBinding := new(korifiv1alpha1.CFServiceBinding)
				Expect(
					k8sClient.Get(ctx, types.NamespacedName{Name: serviceBindingRecord.GUID, Namespace: space.Name}, serviceBinding),
				).To(Succeed())

				Expect(*serviceBinding).To(MatchFields(IgnoreExtras, Fields{
					"ObjectMeta": MatchFields(IgnoreExtras, Fields{
						"Labels": HaveKeyWithValue("servicebinding.io/provisioned-service", "true"),
					}),
					"Spec": MatchFields(IgnoreExtras, Fields{
						"Type":        Equal(korifiv1alpha1.CFServiceBindingTypeApp),
						"DisplayName": BeNil(),
						"Service": Equal(corev1.ObjectReference{
							Kind:       "CFServiceInstance",
							APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
							Name:       cfServiceInstance.Name,
						}),
						"AppRef": Equal(corev1.LocalObjectReference{
							Name: appGUID,
						}),
						"Parameters": MatchAllFields(Fields{
							"Name": Not(BeEmpty()),
						}),
					}),
				}))
			})

			It("creates the parameters secret", func() {
				serviceBinding := new(korifiv1alpha1.CFServiceBinding)
				Expect(
					k8sClient.Get(ctx, types.NamespacedName{Name: serviceBindingRecord.GUID, Namespace: space.Name}, serviceBinding),
				).To(Succeed())

				paramsSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: serviceBinding.Namespace,
						Name:      serviceBinding.Spec.Parameters.Name,
					},
				}
				Expect(
					k8sClient.Get(ctx, client.ObjectKeyFromObject(paramsSecret), paramsSecret),
				).To(Succeed())

				Expect(paramsSecret.Data).To(Equal(map[string][]byte{
					tools.ParametersSecretKey: []byte(`{"p1":"p1-value"}`),
				}))

				Expect(paramsSecret.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Kind": Equal("CFServiceBinding"),
					"Name": Equal(serviceBinding.Name),
				})))
			})

			When("the app does not exist", func() {
				BeforeEach(func() {
					createMsg.AppGUID = "i-do-not-exits"
				})

				It("returns an UnprocessableEntity error", func() {
					Expect(createErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
				})
			})

			When("the service binding has a name", func() {
				BeforeEach(func() {
					createMsg.Name = tools.PtrTo("some-name-for-a-binding")
				})

				It("creates the binding with the specified name", func() {
					Expect(serviceBindingRecord.Name).To(Equal(tools.PtrTo("some-name-for-a-binding")))
				})
			})

			When("binding type is key", func() {
				BeforeEach(func() {
					createMsg.Type = korifiv1alpha1.CFServiceBindingTypeKey
					createMsg.AppGUID = ""
					createMsg.Name = tools.PtrTo(serviceBindingName)
					createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
				})

				It("creates a key binding", func() {
					Expect(serviceBindingRecord.GUID).To(matchers.BeValidUUID())
					Expect(serviceBindingRecord.AppGUID).To(Equal(""))
					Expect(serviceBindingRecord.Type).To(Equal(korifiv1alpha1.CFServiceBindingTypeKey))
					Expect(*(serviceBindingRecord.Name)).To(Equal(serviceBindingName))
					Expect(serviceBindingRecord.ServiceInstanceGUID).To(Equal(cfServiceInstance.Name))
					Expect(serviceBindingRecord.SpaceGUID).To(Equal(space.Name))
					Expect(serviceBindingRecord.CreatedAt).NotTo(BeZero())
					Expect(serviceBindingRecord.UpdatedAt).NotTo(BeNil())
					Expect(serviceBindingRecord.Relationships()).To(Equal(map[string]string{
						"app":              "",
						"service_instance": cfServiceInstance.Name,
					}))

					Expect(createErr).NotTo(HaveOccurred())

					serviceBinding := new(korifiv1alpha1.CFServiceBinding)
					Expect(
						k8sClient.Get(ctx, types.NamespacedName{Name: serviceBindingRecord.GUID, Namespace: space.Name}, serviceBinding),
					).To(Succeed())

					Expect(*serviceBinding).To(MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Labels": HaveKeyWithValue("servicebinding.io/provisioned-service", "true"),
						}),
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Type":        Equal(korifiv1alpha1.CFServiceBindingTypeKey),
							"DisplayName": PointTo(Equal(serviceBindingName)),
							"Service": Equal(corev1.ObjectReference{
								Kind:       "CFServiceInstance",
								APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
								Name:       cfServiceInstance.Name,
							}),
							"Parameters": MatchAllFields(Fields{
								"Name": Not(BeEmpty()),
							}),
						}),
					}))
				})

				It("does not await for app state", func() {
					Expect(appConditionAwaiter.AwaitStateCallCount()).To(BeZero())
				})
			})
		})
	})

	Describe("DeleteServiceBinding", func() {
		var (
			deleteErr          error
			serviceBindingGUID string
		)

		BeforeEach(func() {
			serviceBindingGUID = uuid.NewString()

			serviceBinding := &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceBindingGUID,
					Namespace: space.Name,
					Annotations: map[string]string{
						korifiv1alpha1.ServiceInstanceTypeAnnotation: korifiv1alpha1.UserProvidedType,
					},
					Finalizers: []string{korifiv1alpha1.CFServiceBindingFinalizerName},
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "CFServiceInstance",
						APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
						Name:       uuid.NewString(),
					},
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
					Type: korifiv1alpha1.CFServiceBindingTypeApp,
				},
			}
			Expect(
				k8sClient.Create(ctx, serviceBinding),
			).To(Succeed())

			appConditionAwaiter.AwaitStateStub = func(ctx context.Context, _ repositories.Klient, object client.Object, checkState func(a *korifiv1alpha1.CFApp) error) (*korifiv1alpha1.CFApp, error) {
				cfApp, ok := object.(*korifiv1alpha1.CFApp)
				Expect(ok).To(BeTrue())

				Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
					cfApp.Status.ObservedGeneration = cfApp.Generation
					cfApp.Status.ServiceBindings = slices.Collect(it.Exclude(slices.Values(cfApp.Status.ServiceBindings), func(b korifiv1alpha1.ServiceBinding) bool {
						return b.GUID == serviceBindingGUID
					}))
				})).To(Succeed())

				err := checkState(cfApp)
				return cfApp, err
			}
		})

		JustBeforeEach(func() {
			deleteErr = repo.DeleteServiceBinding(ctx, authInfo, serviceBindingGUID)
		})

		It("returns a not-found error for users with no role in the space", func() {
			Expect(deleteErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
		})

		When("the user is a space manager", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceManagerRole.Name, space.Name)
			})

			It("returns a forbidden error", func() {
				Expect(deleteErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("deletes the binding", func() {
				Expect(deleteErr).NotTo(HaveOccurred())
			})

			When("the binding doesn't exist", func() {
				BeforeEach(func() {
					serviceBindingGUID = "something-that-does-not-match"
				})

				It("returns a not-found error", func() {
					Expect(deleteErr).To(BeAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			It("awaits bindings to disappear from the app status", func() {
				Expect(deleteErr).NotTo(HaveOccurred())

				Expect(appConditionAwaiter.AwaitStateCallCount()).To(Equal(1))
				actualObj, _ := appConditionAwaiter.AwaitStateArgsForCall(0)
				Expect(actualObj.GetName()).To(Equal(appGUID))
			})

			When("the binding is not removed from the status", func() {
				BeforeEach(func() {
					appConditionAwaiter.AwaitStateStub = func(ctx context.Context, _ repositories.Klient, object client.Object, checkState func(a *korifiv1alpha1.CFApp) error) (*korifiv1alpha1.CFApp, error) {
						return nil, errors.New("time-out-err")
					}
				})

				It("returns error", func() {
					Expect(deleteErr).To(MatchError(ContainSubstring("time-out-err")))
				})
			})

			When("the app status is outdated", func() {
				BeforeEach(func() {
					appConditionAwaiter.AwaitStateStub = func(ctx context.Context, _ repositories.Klient, object client.Object, checkState func(a *korifiv1alpha1.CFApp) error) (*korifiv1alpha1.CFApp, error) {
						cfApp, ok := object.(*korifiv1alpha1.CFApp)
						Expect(ok).To(BeTrue())

						Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
							cfApp.Status.ServiceBindings = slices.Collect(it.Exclude(slices.Values(cfApp.Status.ServiceBindings), func(b korifiv1alpha1.ServiceBinding) bool {
								return b.GUID == serviceBindingGUID
							}))
						})).To(Succeed())

						err := checkState(cfApp)
						return cfApp, err
					}
				})

				It("returns error", func() {
					Expect(deleteErr).To(MatchError(ContainSubstring("outdated")))
				})
			})
		})
	})

	Describe("ListServiceBindings", func() {
		var (
			serviceBinding *korifiv1alpha1.CFServiceBinding

			requestMessage          repositories.ListServiceBindingsMessage
			responseServiceBindings []repositories.ServiceBindingRecord
			listErr                 error
		)

		BeforeEach(func() {
			serviceBinding = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space.Name,
					Labels: map[string]string{
						korifiv1alpha1.PlanGUIDLabelKey: "plan-1",
					},
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "ServiceInstance",
						Name:       uuid.NewString(),
						APIVersion: "korifi.cloudfoundry.org/v1alpha1",
					},
					Type: korifiv1alpha1.CFServiceBindingTypeKey,
				},
			}
			Expect(k8sClient.Create(ctx, serviceBinding)).To(Succeed())

			requestMessage = repositories.ListServiceBindingsMessage{}
		})

		JustBeforeEach(func() {
			responseServiceBindings, listErr = repo.ListServiceBindings(ctx, authInfo, requestMessage)
		})

		It("returns an empty list and no error", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(responseServiceBindings).To(BeEmpty())
		})

		When("the user has access to spaces", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns a list of ServiceBindingRecords in the spaces the user has access to", func() {
				Expect(listErr).NotTo(HaveOccurred())

				Expect(responseServiceBindings).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(serviceBinding.Name),
					}),
				))
			})

			Describe("filtering", func() {
				var fakeKlient *fake.Klient

				BeforeEach(func() {
					fakeKlient = new(fake.Klient)
					repo = repositories.NewServiceBindingRepo(fakeKlient, nil, nil, nil)
				})

				Describe("filter parameters to list options", func() {
					BeforeEach(func() {
						requestMessage = repositories.ListServiceBindingsMessage{
							AppGUIDs:             []string{"a1", "a2"},
							ServiceInstanceGUIDs: []string{"s1", "s2"},
							LabelSelector:        "foo=bar",
							PlanGUIDs:            []string{"p1", "p2"},
						}
					})

					It("translates filter parameters to klient list options", func() {
						Expect(listErr).NotTo(HaveOccurred())
						Expect(fakeKlient.ListCallCount()).To(Equal(1))
						_, _, listOptions := fakeKlient.ListArgsForCall(0)
						Expect(listOptions).To(ConsistOf(
							repositories.WithLabelSelector("foo=bar"),
							repositories.WithLabelIn(korifiv1alpha1.PlanGUIDLabelKey, []string{"p1", "p2"}),
							repositories.WithLabelIn(korifiv1alpha1.CFAppGUIDLabelKey, []string{"a1", "a2"}),
							repositories.WithLabelIn(korifiv1alpha1.CFServiceInstanceGUIDLabelKey, []string{"s1", "s2"}),
						))
					})

					When("binding type filter is set", func() {
						BeforeEach(func() {
							requestMessage.Type = korifiv1alpha1.CFServiceBindingTypeApp
						})

						It("translates binding type to label selector", func() {
							Expect(listErr).NotTo(HaveOccurred())
							Expect(fakeKlient.ListCallCount()).To(Equal(1))
							_, _, listOptions := fakeKlient.ListArgsForCall(0)
							Expect(listOptions).To(ContainElement(
								repositories.WithLabel(korifiv1alpha1.CFServiceBindingTypeLabelKey, korifiv1alpha1.CFServiceBindingTypeApp),
							))
						})
					})
				})
			})
		})
	})

	Describe("GetServiceBinding", func() {
		var (
			serviceBindingGUID string
			searchGUID         string
			serviceBinding     repositories.ServiceBindingRecord
			getErr             error
		)

		BeforeEach(func() {
			serviceBindingGUID = uuid.NewString()
			searchGUID = serviceBindingGUID

			serviceBinding := &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceBindingGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "CFServiceInstance",
						APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
						Name:       uuid.NewString(),
					},
					Type: korifiv1alpha1.CFServiceBindingTypeApp,
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
				},
			}
			Expect(
				k8sClient.Create(ctx, serviceBinding),
			).To(Succeed())
		})

		JustBeforeEach(func() {
			serviceBinding, getErr = repo.GetServiceBinding(ctx, authInfo, searchGUID)
		})

		It("returns a forbidden error as no user bindings are in place", func() {
			Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("fetches the CFServiceBinding we're looking for", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(serviceBinding.GUID).To(Equal(serviceBindingGUID))
			})

			When("no CFServiceBinding exists", func() {
				BeforeEach(func() {
					searchGUID = "i-dont-exist"
				})

				It("returns an error", func() {
					Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})
	})

	Describe("GetServiceBindingParameters", func() {
		var (
			serviceBindingGUID   string
			serviceBindingParams map[string]any
			getErr               error
		)

		BeforeEach(func() {
			serviceBrokerGUID := uuid.NewString()
			serviceOfferingGUID := uuid.NewString()
			servicePlanGUID := uuid.NewString()
			serviceInstanceGUID := uuid.NewString()
			serviceBindingGUID = uuid.NewString()

			brokerClient.GetServiceBindingReturns(osbapi.BindingResponse{
				Parameters: map[string]any{
					"foo": "val1",
					"bar": "val2",
				},
			}, nil)

			serviceBroker := &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      serviceBrokerGUID,
				},
				Spec: korifiv1alpha1.CFServiceBrokerSpec{
					Name: uuid.NewString(),
				},
			}
			Expect(k8sClient.Create(ctx, serviceBroker)).To(Succeed())

			offering := &korifiv1alpha1.CFServiceOffering{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      serviceOfferingGUID,
				},
			}
			Expect(k8sClient.Create(ctx, offering)).To(Succeed())

			servicePlan := &korifiv1alpha1.CFServicePlan{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      servicePlanGUID,
					Labels: map[string]string{
						korifiv1alpha1.RelServiceBrokerGUIDLabel:   serviceBrokerGUID,
						korifiv1alpha1.RelServiceOfferingGUIDLabel: serviceOfferingGUID,
					},
				},
				Spec: korifiv1alpha1.CFServicePlanSpec{
					Visibility: korifiv1alpha1.ServicePlanVisibility{
						Type: korifiv1alpha1.PublicServicePlanVisibilityType,
					},
				},
			}
			Expect(k8sClient.Create(ctx, servicePlan)).To(Succeed())

			serviceInstance := &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space.Name,
					Name:      serviceInstanceGUID,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					Type:     korifiv1alpha1.ManagedType,
					PlanGUID: servicePlanGUID,
				},
			}
			Expect(k8sClient.Create(ctx, serviceInstance)).To(Succeed())

			serviceBinding := &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceBindingGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "CFServiceInstance",
						APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
						Name:       serviceInstanceGUID,
					},
					Type: korifiv1alpha1.CFServiceBindingTypeApp,
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
				},
			}
			Expect(
				k8sClient.Create(ctx, serviceBinding),
			).To(Succeed())
		})

		JustBeforeEach(func() {
			serviceBindingParams, getErr = repo.GetServiceBindingParameters(ctx, authInfo, serviceBindingGUID)
		})

		It("returns a forbidden error as no user bindings are in place", func() {
			Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("gets the service binding parameters", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(serviceBindingParams).To(SatisfyAll(
					HaveKeyWithValue("foo", "val1"),
					HaveKeyWithValue("bar", "val2")),
				)
			})
		})

		When("no CFServiceBinding exists", func() {
			BeforeEach(func() {
				serviceBindingGUID = "i-dont-exist"
			})

			It("returns an error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("UpdateServiceBinding", func() {
		var (
			serviceBinding        *korifiv1alpha1.CFServiceBinding
			updatedServiceBinding repositories.ServiceBindingRecord
			updateMessage         repositories.UpdateServiceBindingMessage
			updateErr             error
		)

		BeforeEach(func() {
			serviceBinding = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      prefixedGUID("binding"),
					Namespace: space.Name,
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{
						"baz": "bat",
					},
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Type: korifiv1alpha1.CFServiceBindingTypeApp,
					Service: corev1.ObjectReference{
						Kind:       "CFServiceInstance",
						APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
						Name:       uuid.NewString(),
					},
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
				},
			}
			Expect(
				k8sClient.Create(ctx, serviceBinding),
			).To(Succeed())

			updateMessage = repositories.UpdateServiceBindingMessage{
				GUID: serviceBinding.Name,
				MetadataPatch: repositories.MetadataPatch{
					Labels: map[string]*string{
						"foo": tools.PtrTo("new-foo"),
					},
					Annotations: map[string]*string{
						"baz": tools.PtrTo("new-baz"),
					},
				},
			}
		})

		JustBeforeEach(func() {
			updatedServiceBinding, updateErr = repo.UpdateServiceBinding(ctx, authInfo, updateMessage)
		})

		It("fails because the user has no bindings", func() {
			Expect(updateErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a CFAdmin", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, space.Name)
			})

			It("updates the service binding metadata", func() {
				Expect(updateErr).NotTo(HaveOccurred())

				Expect(updatedServiceBinding.Labels).To(HaveKeyWithValue("foo", "new-foo"))
				Expect(updatedServiceBinding.Annotations).To(HaveKeyWithValue("baz", "new-baz"))
			})

			It("updates the service binding metadata in kubernetes", func() {
				updatedServiceBinding := new(korifiv1alpha1.CFServiceBinding)
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceBinding), updatedServiceBinding)).To(Succeed())

				Expect(updatedServiceBinding.Labels).To(HaveKeyWithValue("foo", "new-foo"))
				Expect(updatedServiceBinding.Annotations).To(HaveKeyWithValue("baz", "new-baz"))
			})

			When("the service binding does not exist", func() {
				BeforeEach(func() {
					updateMessage.GUID = "i-do-not-exist"
				})

				It("returns a not found error", func() {
					Expect(updateErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})
	})

	Describe("GetServiceBindingDetails", func() {
		var (
			serviceBindingGUID   string
			getErr               error
			serviceBindingRecord repositories.ServiceBindingDetailsRecord
			credentialsSecret    *corev1.Secret
		)

		BeforeEach(func() {
			serviceBindingGUID = uuid.NewString()

			sbDisplayName := "test-service-binding"
			serviceBinding := &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceBindingGUID,
					Namespace: space.Name,
					Labels:    map[string]string{"servicebinding.io/provisioned-service": "true"},
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					DisplayName: &sbDisplayName,
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
					Type: "app",
				},
			}

			Expect(
				k8sClient.Create(ctx, serviceBinding),
			).To(Succeed())

			Expect(
				k8s.Patch(ctx, k8sClient, serviceBinding, func() {
					serviceBinding.Status.EnvSecretRef.Name = serviceBindingGUID
					serviceBinding.Status.ObservedGeneration = serviceBinding.Generation
					serviceBinding.Status.Conditions = append(serviceBinding.Status.Conditions, metav1.Condition{
						Type:               korifiv1alpha1.StatusConditionReady,
						Status:             metav1.ConditionStatus("True"),
						Reason:             "reason",
						LastTransitionTime: metav1.Time{Time: time.Now()},
					})
				}),
			).To(Succeed())

			credentials := map[string]any{
				"foo": "val1",
				"bar": "val2",
			}

			creds, err := tools.ToCredentialsSecretData(credentials)
			Expect(err).ToNot(HaveOccurred())

			credentialsSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceBinding.Status.EnvSecretRef.Name,
					Namespace: space.Name,
				},
				Data: creds,
			}
			Expect(
				k8sClient.Create(ctx, credentialsSecret),
			).To(Succeed())
		})

		JustBeforeEach(func() {
			serviceBindingRecord, getErr = repo.GetServiceBindingDetails(ctx, authInfo, serviceBindingGUID)
		})

		It("returns a forbidden error as no user bindings are in place", func() {
			Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns details", func() {
				Expect(getErr).ToNot(HaveOccurred())
				Expect(serviceBindingRecord.Credentials).To(SatisfyAll(
					HaveKeyWithValue("bar", "val2"),
					HaveKeyWithValue("foo", "val1"),
				))
			})

			When("binding does not exist", func() {
				BeforeEach(func() {
					serviceBindingGUID = "i-dont-exist"
				})

				It("returns an error", func() {
					Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})
	})
})
