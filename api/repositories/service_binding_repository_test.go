package repositories_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
)

var _ = Describe("ServiceBindingRepo", func() {
	var (
		repo                *repositories.ServiceBindingRepo
		testCtx             context.Context
		clientFactory       repositories.UnprivilegedClientFactory
		org                 *hnsv1alpha2.SubnamespaceAnchor
		space               *hnsv1alpha2.SubnamespaceAnchor
		appGUID             string
		serviceInstanceGUID string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		clientFactory = repositories.NewUnprivilegedClientFactory(k8sConfig)
		repo = repositories.NewServiceBindingRepo(clientFactory, nsPerms)

		rootNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}}
		Expect(k8sClient.Create(testCtx, rootNs)).To(Succeed())

		org = createOrgAnchorAndNamespace(testCtx, rootNamespace, prefixedGUID("org"))
		space = createSpaceAnchorAndNamespace(testCtx, org.Name, prefixedGUID("space1"))
		appGUID = prefixedGUID("app")
		serviceInstanceGUID = prefixedGUID("service-instance")
	})

	Describe("CreateServiceBinding", func() {
		When("the user can create CFServiceBindings in the Space", func() {
			var (
				bindingName *string
				record      repositories.ServiceBindingRecord
			)
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
				bindingName = nil
			})

			JustBeforeEach(func() {
				var err error
				record, err = repo.CreateServiceBinding(testCtx, authInfo, repositories.CreateServiceBindingMessage{
					Name:                bindingName,
					ServiceInstanceGUID: serviceInstanceGUID,
					AppGUID:             appGUID,
					SpaceGUID:           space.Name,
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates a new CFServiceBinding resource and returns a record", func() {
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

				serviceBinding := new(servicesv1alpha1.CFServiceBinding)
				Expect(
					k8sClient.Get(testCtx, types.NamespacedName{Name: record.GUID, Namespace: space.Name}, serviceBinding),
				).To(Succeed())

				Expect(serviceBinding.Labels).To(HaveKeyWithValue("servicebinding.io/provisioned-service", "true"))
				Expect(serviceBinding.Spec).To(Equal(
					servicesv1alpha1.CFServiceBindingSpec{
						Name: nil,
						Service: corev1.ObjectReference{
							Kind:       "CFServiceInstance",
							APIVersion: servicesv1alpha1.GroupVersion.Identifier(),
							Name:       serviceInstanceGUID,
						},
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
					},
				))
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

		When("the user doesn't have permission to create CFServiceBindings in the Space", func() {
			It("returns a Forbidden error", func() {
				_, err := repo.CreateServiceBinding(testCtx, authInfo, repositories.CreateServiceBindingMessage{
					Name:                nil,
					ServiceInstanceGUID: serviceInstanceGUID,
					AppGUID:             appGUID,
					SpaceGUID:           space.Name,
				})
				Expect(err).To(BeAssignableToTypeOf(repositories.ForbiddenError{}))
			})
		})
	})

	Describe("ServiceBindingExists", func() {
		BeforeEach(func() {
			app := &workloadsv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appGUID,
					Namespace: space.Name,
				},
				Spec: workloadsv1alpha1.CFAppSpec{
					Name:         "some-app",
					DesiredState: workloadsv1alpha1.DesiredState(repositories.StoppedState),
					Lifecycle: workloadsv1alpha1.Lifecycle{
						Type: "buildpack",
						Data: workloadsv1alpha1.LifecycleData{
							Buildpacks: []string{},
							Stack:      "",
						},
					},
				},
			}
			Expect(
				k8sClient.Create(testCtx, app),
			).To(Succeed())

			serviceInstance := &servicesv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appGUID,
					Namespace: space.Name,
				},
				Spec: servicesv1alpha1.CFServiceInstanceSpec{
					Name:       "some-instance",
					SecretName: "",
					Type:       "user-provided",
				},
			}
			Expect(
				k8sClient.Create(testCtx, serviceInstance),
			).To(Succeed())
		})

		When("the user can list ServiceBindings in the Space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			When("a ServiceBinding exists for the App and the ServiceInstance in the Space", func() {
				BeforeEach(func() {
					serviceBinding := &servicesv1alpha1.CFServiceBinding{
						ObjectMeta: metav1.ObjectMeta{
							Name:      appGUID,
							Namespace: space.Name,
						},
						Spec: servicesv1alpha1.CFServiceBindingSpec{
							Service: corev1.ObjectReference{
								Kind:       "CFServiceInstance",
								APIVersion: servicesv1alpha1.GroupVersion.Identifier(),
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

				It("returns true", func() {
					exists, err := repo.ServiceBindingExists(testCtx, authInfo, space.Name, appGUID, serviceInstanceGUID)
					Expect(err).NotTo(HaveOccurred())
					Expect(exists).To(BeTrue())
				})
			})

			When("no ServiceBinding exists for the App and the ServiceInstance in the Space", func() {
				BeforeEach(func() {
					serviceBinding := &servicesv1alpha1.CFServiceBinding{
						ObjectMeta: metav1.ObjectMeta{
							Name:      appGUID,
							Namespace: space.Name,
						},
						Spec: servicesv1alpha1.CFServiceBindingSpec{
							Service: corev1.ObjectReference{
								Kind:       "CFServiceInstance",
								APIVersion: servicesv1alpha1.GroupVersion.Identifier(),
								Name:       serviceInstanceGUID,
							},
							AppRef: corev1.LocalObjectReference{
								Name: "another-app-guid",
							},
						},
					}
					Expect(
						k8sClient.Create(testCtx, serviceBinding),
					).To(Succeed())
				})

				It("returns false", func() {
					exists, err := repo.ServiceBindingExists(testCtx, authInfo, space.Name, appGUID, serviceInstanceGUID)
					Expect(err).NotTo(HaveOccurred())
					Expect(exists).To(BeFalse())
				})
			})
		})

		When("the user doesn't have permission to list ServiceBindings in the Space", func() {
			It("returns a Forbidden error", func() {
				_, err := repo.ServiceBindingExists(testCtx, authInfo, space.Name, appGUID, serviceInstanceGUID)
				Expect(err).To(BeAssignableToTypeOf(repositories.ForbiddenError{}))
			})
		})
	})

	Describe("ListServiceBindings", func() {
		var (
			serviceBinding1, serviceBinding2 *servicesv1alpha1.CFServiceBinding
			space2                           *hnsv1alpha2.SubnamespaceAnchor

			serviceInstance2GUID string

			requestMessage repositories.ListServiceBindingsMessage
		)

		BeforeEach(func() {
			cfApp1 := createAppCR(testCtx, k8sClient, "app-1-name", generateGUID(), space.Name, "STOPPED")
			cfServiceInstance1 := createServiceInstanceCR(testCtx, k8sClient, generateGUID(), space.Name, "service-instance-1-name", "secret-1-name")
			serviceBinding1Name := "service-binding-1-name"
			serviceBinding1 = createServiceBindingCR(testCtx, k8sClient, generateGUID(), space.Name, &serviceBinding1Name, cfServiceInstance1.Name, cfApp1.Name)

			space2 = createSpaceAnchorAndNamespace(testCtx, org.Name, prefixedGUID("space2"))
			cfApp2 := createAppCR(testCtx, k8sClient, "app-2-name", generateGUID(), space2.Name, "STOPPED")
			serviceInstance2GUID = generateGUID()
			cfServiceInstance2 := createServiceInstanceCR(testCtx, k8sClient, serviceInstance2GUID, space2.Name, "service-instance-2-name", "secret-2-name")
			serviceBinding2 = createServiceBindingCR(testCtx, k8sClient, generateGUID(), space2.Name, nil, cfServiceInstance2.Name, cfApp2.Name)

			requestMessage = repositories.ListServiceBindingsMessage{}
		})

		When("the user has access to both namespaces", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space2.Name)
			})

			When("no query parameters are specified", func() {
				It("returns a list of ServiceBindingRecords in the spaces the user has access to", func() {
					Eventually(func() []repositories.ServiceBindingRecord {
						responseServiceBindings, err := repo.ListServiceBindings(context.Background(), authInfo, requestMessage)
						Expect(err).NotTo(HaveOccurred())
						return responseServiceBindings
					}, timeCheckThreshold*time.Second).Should(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"GUID":                Equal(serviceBinding1.Name),
							"Type":                Equal("app"),
							"Name":                Equal(serviceBinding1.Spec.Name),
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
							"Name":                Equal(serviceBinding2.Spec.Name),
							"AppGUID":             Equal(serviceBinding2.Spec.AppRef.Name),
							"ServiceInstanceGUID": Equal(serviceBinding2.Spec.Service.Name),
							"SpaceGUID":           Equal(serviceBinding2.Namespace),
							"LastOperation": MatchFields(IgnoreExtras, Fields{
								"Type":  Equal("create"),
								"State": Equal("succeeded"),
							}),
						}),
					))
				})
			})

			When("query parameters are specified", func() {
				It("eventually returns only a subset of the ServiceBindings that match the provided filter", func() {
					requestMessage = repositories.ListServiceBindingsMessage{
						ServiceInstanceGUIDs: []string{serviceInstance2GUID},
					}

					Eventually(func() []repositories.ServiceBindingRecord {
						responseServiceBindings, err := repo.ListServiceBindings(context.Background(), authInfo, requestMessage)
						Expect(err).NotTo(HaveOccurred())
						return responseServiceBindings
					}, timeCheckThreshold*time.Second).Should(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"GUID":                Equal(serviceBinding2.Name),
							"ServiceInstanceGUID": Equal(serviceInstance2GUID),
						}),
					))
				})
			})
		})

		When("the user does not have access to any namespaces", func() {
			It("always returns an empty list and no error", func() {
				Consistently(func() []repositories.ServiceBindingRecord {
					responseServiceBindings, err := repo.ListServiceBindings(testCtx, authInfo, requestMessage)
					Expect(err).NotTo(HaveOccurred())
					return responseServiceBindings
				}, 500*time.Millisecond).Should(HaveLen(0))
			})
		})

		When("fetching authorized namespaces fails", func() {
			var listErr error

			BeforeEach(func() {
				_, listErr = repo.ListServiceBindings(testCtx, authorization.Info{}, repositories.ListServiceBindingsMessage{ServiceInstanceGUIDs: []string{"org1", "org3"}})
			})

			It("returns the error", func() {
				Expect(listErr).To(MatchError(ContainSubstring("failed to get identity")))
			})
		})
	})
})
