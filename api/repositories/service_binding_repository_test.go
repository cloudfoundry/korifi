package repositories_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ServiceBindingRepo", func() {
	var (
		repo                          *repositories.ServiceBindingRepo
		testCtx                       context.Context
		org                           *korifiv1alpha1.CFOrg
		space                         *korifiv1alpha1.CFSpace
		appGUID                       string
		serviceInstanceGUID           string
		doBindingControllerSimulation bool
		bindingName                   *string
		controllerCancel              chan bool
		controllerDone                *sync.WaitGroup
	)

	BeforeEach(func() {
		testCtx = context.Background()
		bindingConditionAwaiter := conditions.NewConditionAwaiter[*korifiv1alpha1.CFServiceBinding, korifiv1alpha1.CFServiceBindingList](time.Second)
		repo = repositories.NewServiceBindingRepo(namespaceRetriever, userClientFactory, nsPerms, bindingConditionAwaiter)

		org = createOrgWithCleanup(testCtx, prefixedGUID("org"))
		space = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space1"))
		appGUID = prefixedGUID("app")
		serviceInstanceGUID = prefixedGUID("service-instance")
		doBindingControllerSimulation = true
		bindingName = nil
		controllerCancel = make(chan bool, 1)
		controllerDone = &sync.WaitGroup{}

		app := &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appGUID,
				Namespace: space.Name,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  "some-app",
				DesiredState: korifiv1alpha1.DesiredState(repositories.StoppedState),
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
			k8sClient.Create(testCtx, app),
		).To(Succeed())
	})

	waitForServiceBinding := func(anchorNamespace string, done chan bool) (*korifiv1alpha1.CFServiceBinding, error) {
		for {
			select {
			case <-done:
				return nil, fmt.Errorf("waitForCFOrg was 'signalled' to stop polling")
			default:
			}

			var serviceBindingList korifiv1alpha1.CFServiceBindingList
			err := k8sClient.List(ctx, &serviceBindingList, client.InNamespace(anchorNamespace))
			if err != nil {
				return nil, fmt.Errorf("waitForServiceBinding failed")
			}

			if len(serviceBindingList.Items) > 1 {
				return nil, fmt.Errorf("waitForCFOrg found multiple anchors")
			}
			if len(serviceBindingList.Items) == 1 {
				return &serviceBindingList.Items[0], nil
			}

			time.Sleep(time.Millisecond * 100)
		}
	}

	simulateServiceBindingController := func(anchorNamespace string, controllerCancel chan bool, controllerDone *sync.WaitGroup) {
		defer GinkgoRecover()

		serviceBinding, err := waitForServiceBinding(anchorNamespace, controllerCancel)
		if err != nil {
			return
		}

		originalServiceBinding := serviceBinding.DeepCopy()

		serviceBinding.Status.Binding.Name = "service-secret-name"
		meta.SetStatusCondition(&(serviceBinding.Status.Conditions), metav1.Condition{
			Type:    repositories.VCAPServicesSecretAvailableCondition,
			Status:  metav1.ConditionTrue,
			Reason:  "blah",
			Message: "blah",
		})
		Expect(
			k8sClient.Status().Patch(ctx, serviceBinding, client.MergeFrom(originalServiceBinding)),
		).To(Succeed())

		controllerDone.Done()
	}

	JustBeforeEach(func() {
		if doBindingControllerSimulation {
			controllerDone.Add(1)
			go simulateServiceBindingController(space.Name, controllerCancel, controllerDone)
		}
	})

	AfterEach(func() {
		controllerCancel <- true
		controllerDone.Wait()
	})

	Describe("CreateServiceBinding", func() {
		var (
			record    repositories.ServiceBindingRecord
			createErr error
		)
		BeforeEach(func() {
			bindingName = nil
		})

		JustBeforeEach(func() {
			record, createErr = repo.CreateServiceBinding(testCtx, authInfo, repositories.CreateServiceBindingMessage{
				Name:                bindingName,
				ServiceInstanceGUID: serviceInstanceGUID,
				AppGUID:             appGUID,
				SpaceGUID:           space.Name,
			})
		})

		When("the user can create CFServiceBindings in the Space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates a new CFServiceBinding resource and returns a record", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(record.GUID).NotTo(BeEmpty())
				Expect(record.Type).To(Equal("app"))
				Expect(record.Name).To(BeNil())
				Expect(record.AppGUID).To(Equal(appGUID))
				Expect(record.ServiceInstanceGUID).To(Equal(serviceInstanceGUID))
				Expect(record.SpaceGUID).To(Equal(space.Name))
				Expect(record.CreatedAt).NotTo(BeEmpty())
				Expect(record.UpdatedAt).NotTo(BeEmpty())

				Expect(record.LastOperation.Type).To(Equal("create"))
				Expect(record.LastOperation.State).To(Equal("succeeded"))
				Expect(record.LastOperation.Description).To(BeNil())
				Expect(record.LastOperation.CreatedAt).To(Equal(record.CreatedAt))
				Expect(record.LastOperation.UpdatedAt).To(Equal(record.UpdatedAt))

				serviceBinding := new(korifiv1alpha1.CFServiceBinding)
				Expect(
					k8sClient.Get(testCtx, types.NamespacedName{Name: record.GUID, Namespace: space.Name}, serviceBinding),
				).To(Succeed())

				Expect(serviceBinding.Labels).To(HaveKeyWithValue("servicebinding.io/provisioned-service", "true"))
				Expect(serviceBinding.Spec).To(Equal(
					korifiv1alpha1.CFServiceBindingSpec{
						DisplayName: nil,
						Service: corev1.ObjectReference{
							Kind:       "CFServiceInstance",
							APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
							Name:       serviceInstanceGUID,
						},
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
					},
				))
			})

			When("the app does not exist", func() {
				BeforeEach(func() {
					doBindingControllerSimulation = false
					appGUID = "i-do-not-exits"
				})

				It("reuturns an UnprocessableEntity error", func() {
					Expect(createErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
				})
			})

			When("the service binding doesn't become ready in time", func() {
				BeforeEach(func() {
					doBindingControllerSimulation = false
				})

				It("returns an error", func() {
					Expect(createErr).To(MatchError(ContainSubstring("did not get the VCAPServicesSecretAvailable condition")))
				})
			})

			When("The service binding has a name", func() {
				BeforeEach(func() {
					tempName := "some-name-for-a-binding"
					bindingName = &tempName
				})

				It("creates the binding with the specified name", func() {
					Expect(record.Name).To(Equal(bindingName))
				})
			})
		})
	})

	Describe("DeleteServiceBinding", func() {
		var (
			ret                error
			serviceBindingGUID string
		)

		BeforeEach(func() {
			serviceBindingGUID = prefixedGUID("binding")
			serviceInstance := &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: "some-instance",
					SecretName:  "",
					Type:        "user-provided",
				},
			}
			Expect(
				k8sClient.Create(testCtx, serviceInstance),
			).To(Succeed())

			serviceBinding := &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceBindingGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "CFServiceInstance",
						APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
						Name:       serviceInstanceGUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
				},
			}
			Expect(
				k8sClient.Create(testCtx, serviceBinding),
			).To(Succeed())
		})

		JustBeforeEach(func() {
			ret = repo.DeleteServiceBinding(testCtx, authInfo, serviceBindingGUID)
		})

		It("returns a not-found error for users with no role in the space", func() {
			Expect(ret).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
		})

		When("the user is a space manager", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceManagerRole.Name, space.Name)
			})

			It("returns a forbidden error", func() {
				Expect(ret).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("deletes the binding", func() {
				Expect(ret).NotTo(HaveOccurred())
			})

			When("the binding doesn't exist", func() {
				BeforeEach(func() {
					serviceBindingGUID = "something-that-does-not-match"
				})

				It("returns a not-found error", func() {
					Expect(ret).To(BeAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})
	})

	Describe("ListServiceBindings", func() {
		var (
			serviceBinding1, serviceBinding2, serviceBinding3                *korifiv1alpha1.CFServiceBinding
			space2                                                           *korifiv1alpha1.CFSpace
			cfApp1, cfApp2, cfApp3                                           *korifiv1alpha1.CFApp
			serviceInstance1GUID, serviceInstance2GUID, serviceInstance3GUID string

			requestMessage          repositories.ListServiceBindingsMessage
			responseServiceBindings []repositories.ServiceBindingRecord
			listErr                 error
		)

		BeforeEach(func() {
			cfApp1 = createAppCR(testCtx, k8sClient, "app-1-name", prefixedGUID("app-1"), space.Name, "STOPPED")
			serviceInstance1GUID = prefixedGUID("instance-1")
			cfServiceInstance1 := createServiceInstanceCR(testCtx, k8sClient, serviceInstance1GUID, space.Name, "service-instance-1-name", "secret-1-name")
			serviceBinding1Name := "service-binding-1-name"
			serviceBinding1 = createServiceBindingCR(testCtx, k8sClient, prefixedGUID("binding-1"), space.Name, &serviceBinding1Name, cfServiceInstance1.Name, cfApp1.Name)

			space2 = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space-2"))
			cfApp2 = createAppCR(testCtx, k8sClient, "app-2-name", prefixedGUID("app-2"), space2.Name, "STOPPED")
			serviceInstance2GUID = prefixedGUID("instance-2")
			cfServiceInstance2 := createServiceInstanceCR(testCtx, k8sClient, serviceInstance2GUID, space2.Name, "service-instance-2-name", "secret-2-name")
			serviceBinding2 = createServiceBindingCR(testCtx, k8sClient, prefixedGUID("binding-2"), space2.Name, nil, cfServiceInstance2.Name, cfApp2.Name)

			cfApp3 = createAppCR(testCtx, k8sClient, "app-3-name", prefixedGUID("app-3"), space2.Name, "STOPPED")
			serviceInstance3GUID = prefixedGUID("instance-3")
			cfServiceInstance3 := createServiceInstanceCR(testCtx, k8sClient, serviceInstance3GUID, space2.Name, "service-instance-3-name", "secret-3-name")
			serviceBinding3 = createServiceBindingCR(testCtx, k8sClient, prefixedGUID("binding-3"), space2.Name, nil, cfServiceInstance3.Name, cfApp3.Name)

			requestMessage = repositories.ListServiceBindingsMessage{}
		})

		JustBeforeEach(func() {
			responseServiceBindings, listErr = repo.ListServiceBindings(context.Background(), authInfo, requestMessage)
		})

		When("the user has access to both namespaces", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space2.Name)
			})

			It("succeeds", func() {
				Expect(listErr).NotTo(HaveOccurred())
			})

			When("no query parameters are specified", func() {
				It("returns a list of ServiceBindingRecords in the spaces the user has access to", func() {
					Expect(responseServiceBindings).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"GUID":                Equal(serviceBinding1.Name),
							"Type":                Equal("app"),
							"Name":                Equal(serviceBinding1.Spec.DisplayName),
							"AppGUID":             Equal(serviceBinding1.Spec.AppRef.Name),
							"ServiceInstanceGUID": Equal(serviceBinding1.Spec.Service.Name),
							"SpaceGUID":           Equal(serviceBinding1.Namespace),
							"LastOperation": MatchFields(IgnoreExtras, Fields{
								"Type":  Equal("create"),
								"State": Equal("succeeded"),
							}),
						}),
						MatchFields(IgnoreExtras, Fields{
							"GUID":                Equal(serviceBinding2.Name),
							"Type":                Equal("app"),
							"Name":                Equal(serviceBinding2.Spec.DisplayName),
							"AppGUID":             Equal(serviceBinding2.Spec.AppRef.Name),
							"ServiceInstanceGUID": Equal(serviceBinding2.Spec.Service.Name),
							"SpaceGUID":           Equal(serviceBinding2.Namespace),
							"LastOperation": MatchFields(IgnoreExtras, Fields{
								"Type":  Equal("create"),
								"State": Equal("succeeded"),
							}),
						}),
						MatchFields(IgnoreExtras, Fields{
							"GUID":                Equal(serviceBinding3.Name),
							"Type":                Equal("app"),
							"Name":                Equal(serviceBinding3.Spec.DisplayName),
							"AppGUID":             Equal(serviceBinding3.Spec.AppRef.Name),
							"ServiceInstanceGUID": Equal(serviceBinding3.Spec.Service.Name),
							"SpaceGUID":           Equal(serviceBinding3.Namespace),
							"LastOperation": MatchFields(IgnoreExtras, Fields{
								"Type":  Equal("create"),
								"State": Equal("succeeded"),
							}),
						}),
					))
				})
			})

			When("filtered by service instance GUID", func() {
				BeforeEach(func() {
					requestMessage = repositories.ListServiceBindingsMessage{
						ServiceInstanceGUIDs: []string{serviceInstance2GUID, serviceInstance3GUID},
					}
				})

				It("returns only the ServiceBindings that match the provided service instance guids", func() {
					Expect(responseServiceBindings).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"GUID":                Equal(serviceBinding2.Name),
							"ServiceInstanceGUID": Equal(serviceInstance2GUID),
						}),
						MatchFields(IgnoreExtras, Fields{
							"GUID":                Equal(serviceBinding3.Name),
							"ServiceInstanceGUID": Equal(serviceInstance3GUID),
						}),
					))
				})
			})

			When("filtered by app guid", func() {
				BeforeEach(func() {
					requestMessage = repositories.ListServiceBindingsMessage{
						AppGUIDs: []string{cfApp1.Name, cfApp2.Name},
					}
				})
				It("returns only the ServiceBindings that match the provided app guids", func() {
					Expect(responseServiceBindings).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"GUID":                Equal(serviceBinding1.Name),
							"ServiceInstanceGUID": Equal(serviceInstance1GUID),
						}),
						MatchFields(IgnoreExtras, Fields{
							"GUID":                Equal(serviceBinding2.Name),
							"ServiceInstanceGUID": Equal(serviceInstance2GUID),
						}),
					))
				})
			})

			When("filtered by multiple params", func() {
				BeforeEach(func() {
					requestMessage = repositories.ListServiceBindingsMessage{
						ServiceInstanceGUIDs: []string{serviceInstance2GUID, serviceInstance3GUID},
						AppGUIDs:             []string{cfApp1.Name, cfApp2.Name},
					}
				})
				It("returns only the ServiceBindings that match all provided filters", func() {
					Expect(responseServiceBindings).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"GUID":                Equal(serviceBinding2.Name),
							"ServiceInstanceGUID": Equal(serviceInstance2GUID),
						}),
					))
				})
			})
		})

		When("the user does not have access to any namespaces", func() {
			It("returns an empty list and no error", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(responseServiceBindings).To(BeEmpty())
			})
		})

		When("fetching authorized namespaces fails", func() {
			BeforeEach(func() {
				authInfo = authorization.Info{}
			})

			It("returns the error", func() {
				Expect(listErr).To(MatchError(ContainSubstring("failed to get identity")))
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
			serviceBindingGUID = prefixedGUID("binding")
			searchGUID = serviceBindingGUID

			serviceBinding := &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceBindingGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "CFServiceInstance",
						APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
						Name:       serviceInstanceGUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
				},
			}
			Expect(
				k8sClient.Create(testCtx, serviceBinding),
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
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
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
					Service: corev1.ObjectReference{
						Kind:       "CFServiceInstance",
						APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
						Name:       serviceInstanceGUID,
					},
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
				},
			}
			Expect(
				k8sClient.Create(testCtx, serviceBinding),
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
})
