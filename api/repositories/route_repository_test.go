package repositories_test

import (
	"errors"
	"time"

	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("RouteRepository", func() {
	const domainName = "my-domain-name"

	var (
		org   *korifiv1alpha1.CFOrg
		space *korifiv1alpha1.CFSpace

		routeGUID  string
		domainGUID string
		routeRepo  *repositories.RouteRepo
	)

	BeforeEach(func() {
		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space1"))

		routeGUID = prefixedGUID("route1")
		domainGUID = prefixedGUID("domain")
		routeRepo = repositories.NewRouteRepo(spaceScopedKlient)

		cfDomain := &korifiv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      domainGUID,
				Namespace: rootNamespace,
			},
			Spec: korifiv1alpha1.CFDomainSpec{
				Name: domainName,
			},
		}
		Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())
	})

	Describe("GetRoute", func() {
		var (
			cfRoute *korifiv1alpha1.CFRoute
			route   repositories.RouteRecord
			getErr  error
		)

		BeforeEach(func() {
			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []korifiv1alpha1.Destination{
						{
							GUID: "destination-guid",
							Port: tools.PtrTo[int32](8080),
							AppRef: corev1.LocalObjectReference{
								Name: "some-app-guid",
							},
							ProcessType: "web",
							Protocol:    tools.PtrTo("http1"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())
		})

		JustBeforeEach(func() {
			route, getErr = routeRepo.GetRoute(ctx, authInfo, routeGUID)
		})

		It("returns a forbidden error for unauthorized users", func() {
			Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a space developer in this space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("fetches the CFRoute CR we're looking for", func() {
				Expect(getErr).ToNot(HaveOccurred())

				Expect(route.GUID).To(Equal(cfRoute.Name))
				Expect(route.Host).To(Equal(cfRoute.Spec.Host))
				Expect(route.SpaceGUID).To(Equal(cfRoute.Namespace))
				Expect(route.Path).To(Equal(cfRoute.Spec.Path))
				Expect(route.Protocol).To(Equal(string(cfRoute.Spec.Protocol)))

				Expect(route.Relationships()).To(Equal(map[string]string{
					"space":  cfRoute.Namespace,
					"domain": domainGUID,
				}))

				By("returning a record with destinations that match the CFRoute CR", func() {
					Expect(route.Destinations).To(HaveLen(len(cfRoute.Spec.Destinations)), "Route Record Destinations returned was not the correct length")
					destinationRecord := route.Destinations[0]
					Expect(destinationRecord.GUID).To(Equal(cfRoute.Spec.Destinations[0].GUID))
					Expect(destinationRecord.AppGUID).To(Equal(cfRoute.Spec.Destinations[0].AppRef.Name))
					Expect(destinationRecord.Port).To(Equal(cfRoute.Spec.Destinations[0].Port))
					Expect(destinationRecord.ProcessType).To(Equal(cfRoute.Spec.Destinations[0].ProcessType))
					Expect(destinationRecord.Protocol).To(Equal(cfRoute.Spec.Destinations[0].Protocol))
				})

				By("returning a record with timestamps", func() {
					Expect(route.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
					Expect(route.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
					Expect(route.DeletedAt).To(BeNil())
				})

				Expect(route.Domain).To(Equal(repositories.DomainRecord{GUID: domainGUID}))
			})

			When("the route destination does not have a port", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, k8sClient, cfRoute, func() {
						cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{{
							GUID: "destination-guid",
							AppRef: corev1.LocalObjectReference{
								Name: "some-app-guid",
							},
							ProcessType: "web",
						}}
					})).To(Succeed())
				})

				It("returns a destination record without port", func() {
					Expect(getErr).ToNot(HaveOccurred())
					Expect(route.Destinations).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
						"Port":     BeNil(),
						"Protocol": BeNil(),
					})))
				})

				When("the destination in the route status has port", func() {
					BeforeEach(func() {
						Expect(k8s.Patch(ctx, k8sClient, cfRoute, func() {
							cfRoute.Status = korifiv1alpha1.CFRouteStatus{
								Destinations: []korifiv1alpha1.Destination{{
									GUID: "destination-guid",
									AppRef: corev1.LocalObjectReference{
										Name: "some-app-guid",
									},
									ProcessType: "web",
									Port:        tools.PtrTo[int32](2345),
									Protocol:    tools.PtrTo("http1"),
								}},
							}
						})).To(Succeed())
					})

					It("returns a destination record with the port in the route status", func() {
						Expect(getErr).ToNot(HaveOccurred())
						Expect(route.Destinations).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
							"Port":     PointTo(BeEquivalentTo(2345)),
							"Protocol": PointTo(Equal("http1")),
						})))
					})
				})
			})
		})

		When("the CFRoute doesn't exist", func() {
			BeforeEach(func() {
				routeGUID = "non-existent-route-guid"
			})

			It("returns an error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("ListRoutes", func() {
		var (
			cfRoute *korifiv1alpha1.CFRoute

			listResult repositories.ListResult[repositories.RouteRecord]
			message    repositories.ListRoutesMessage
		)

		BeforeEach(func() {
			appGUID := uuid.NewString()
			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1-a",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []korifiv1alpha1.Destination{{
						Protocol: tools.PtrTo("http1"),
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
					}},
				},
			}
			Expect(
				k8sClient.Create(ctx, cfRoute),
			).To(Succeed())

			message = repositories.ListRoutesMessage{}
		})

		JustBeforeEach(func() {
			var err error
			listResult, err = routeRepo.ListRoutes(ctx, authInfo, message)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an empty list as the user is not authorized", func() {
			Expect(listResult.Records).To(BeEmpty())
		})

		When("the user has space developer access", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("returns a list of routeRecords for each CFRoute CR", func() {
				Expect(listResult.Records).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfRoute.Name)}),
				))
				Expect(listResult.PageInfo.TotalResults).To(Equal(1))
			})
		})

		Describe("parameters to list options", func() {
			var fakeKlient *fake.Klient

			BeforeEach(func() {
				fakeKlient = new(fake.Klient)
				routeRepo = repositories.NewRouteRepo(fakeKlient)

				message = repositories.ListRoutesMessage{
					AppGUIDs:    []string{"g1"},
					SpaceGUIDs:  []string{"sg1", "sg2"},
					DomainGUIDs: []string{"domainGUID"},
					Hosts:       []string{"my-subdomain-1-a"},
					Paths:       []string{"/some/path"},
					IsUnmapped:  tools.PtrTo(false),
					OrderBy:     "created_at",
					Pagination: repositories.Pagination{
						PerPage: 3,
						Page:    2,
					},
				}
			})

			It("translates filter parameters to klient list options", func() {
				Expect(fakeKlient.ListCallCount()).To(Equal(1))
				_, _, listOptions := fakeKlient.ListArgsForCall(0)
				Expect(listOptions).To(ConsistOf(
					repositories.WithLabelIn(korifiv1alpha1.CFDomainGUIDLabelKey, []string{"domainGUID"}),
					repositories.WithLabelIn(korifiv1alpha1.CFRouteHostLabelKey, []string{"my-subdomain-1-a"}),
					repositories.WithLabelIn(korifiv1alpha1.CFRoutePathLabelKey, tools.EncodeValuesToSha224("/some/path")),
					repositories.WithLabelIn(korifiv1alpha1.SpaceGUIDLabelKey, []string{"sg1", "sg2"}),
					repositories.WithLabel(korifiv1alpha1.CFRouteIsUnmappedLabelKey, "false"),
					repositories.WithLabelExists(korifiv1alpha1.DestinationAppGUIDLabelPrefix+"g1"),
					repositories.WithOrdering("created_at"),
					repositories.WithPaging(repositories.Pagination{
						PerPage: 3,
						Page:    2,
					}),
				))
			})
		})
	})

	Describe("CreateRoute", func() {
		var (
			createdRouteRecord repositories.RouteRecord
			createdRouteErr    error
			routeHost          string
			routePath          string
			routeNamespace     string
		)

		BeforeEach(func() {
			routeNamespace = space.Name
			routeHost = prefixedGUID("route-host-")
			routePath = prefixedGUID("/test/route/")
			createdRouteRecord = repositories.RouteRecord{}
			createdRouteErr = nil
		})

		JustBeforeEach(func() {
			createdRouteRecord, createdRouteErr = routeRepo.CreateRoute(ctx, authInfo, repositories.CreateRouteMessage{
				Host:            routeHost,
				Path:            routePath,
				SpaceGUID:       routeNamespace,
				DomainGUID:      domainGUID,
				DomainNamespace: rootNamespace,
			})
		})

		It("errors with forbidden for users with no permissions", func() {
			Expect(createdRouteErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates a new CFRoute CR successfully", func() {
				Expect(createdRouteErr).NotTo(HaveOccurred())
				createdCFRoute := &korifiv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: space.Name,
						Name:      createdRouteRecord.GUID,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(createdCFRoute), createdCFRoute)).To(Succeed())

				Expect(createdCFRoute.Spec.Host).To(Equal(createdRouteRecord.Host))
				Expect(createdCFRoute.Spec.Path).To(Equal(createdRouteRecord.Path))
				Expect(string(createdCFRoute.Spec.Protocol)).To(Equal(createdRouteRecord.Protocol))
				Expect(createdCFRoute.Spec.DomainRef.Namespace).To(Equal(rootNamespace))
				Expect(createdCFRoute.Spec.DomainRef.Name).To(Equal(domainGUID))
				Expect(createdCFRoute.Spec.Destinations).To(BeEmpty())
			})

			It("returns a RouteRecord with matching fields", func() {
				Expect(createdRouteRecord.GUID).To(matchers.BeValidUUID())
				Expect(createdRouteRecord.Host).To(Equal(routeHost), "Route Host in record did not match input")
				Expect(createdRouteRecord.Path).To(Equal(routePath), "Route Path in record did not match input")
				Expect(createdRouteRecord.SpaceGUID).To(Equal(space.Name), "Route Space GUID in record did not match input")
				Expect(createdRouteRecord.Domain).To(Equal(repositories.DomainRecord{GUID: domainGUID}), "Route Domain in record did not match created domain")

				Expect(createdRouteRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(createdRouteRecord.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
			})

			When("target namespace isn't set", func() {
				BeforeEach(func() {
					routeNamespace = ""
				})
				It("returns an error", func() {
					Expect(createdRouteErr).To(MatchError("an empty namespace may not be set during creation"))
				})
			})
		})
	})

	Describe("DeleteRoute", func() {
		var (
			cfRoute   *korifiv1alpha1.CFRoute
			deleteErr error
		)

		BeforeEach(func() {
			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []korifiv1alpha1.Destination{
						{
							GUID: "destination-guid",
							Port: tools.PtrTo[int32](8080),
							AppRef: corev1.LocalObjectReference{
								Name: "some-app-guid",
							},
							ProcessType: "web",
							Protocol:    tools.PtrTo("http1"),
						},
					},
				},
			}
			Expect(
				k8sClient.Create(ctx, cfRoute),
			).To(Succeed())
		})

		JustBeforeEach(func() {
			deleteErr = routeRepo.DeleteRoute(ctx, authInfo, repositories.DeleteRouteMessage{
				GUID:      routeGUID,
				SpaceGUID: space.Name,
			})
		})

		It("errors with forbidden", func() {
			Expect(deleteErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user has permission to delete routes and", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("deletes the route resource", func() {
				Expect(deleteErr).NotTo(HaveOccurred())
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: space.Name, Name: routeGUID}, &korifiv1alpha1.CFRoute{})).To(MatchError(ContainSubstring("not found")))
			})

			When("the route doesn't exist", func() {
				BeforeEach(func() {
					routeGUID = "i-dont-exist"
				})

				It("errors", func() {
					Expect(deleteErr).To(MatchError(ContainSubstring("not found")))
				})
			})
		})
	})

	Describe("DeleteUnmappedRoutes", func() {
		var (
			cfRoute1  *korifiv1alpha1.CFRoute
			cfRoute2  *korifiv1alpha1.CFRoute
			deleteErr error
		)

		BeforeEach(func() {
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)

			cfRoute1 = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []korifiv1alpha1.Destination{
						{
							GUID: "destination-guid",
							Port: tools.PtrTo[int32](8080),
							AppRef: corev1.LocalObjectReference{
								Name: "some-app-guid",
							},
							ProcessType: "web",
							Protocol:    tools.PtrTo("http1"),
						},
					},
				},
			}
			Expect(
				k8sClient.Create(ctx, cfRoute1),
			).To(Succeed())

			cfRoute2 = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []korifiv1alpha1.Destination{},
				},
			}
			Expect(
				k8sClient.Create(ctx, cfRoute2),
			).To(Succeed())
		})

		JustBeforeEach(func() {
			deleteErr = routeRepo.DeleteUnmappedRoutes(ctx, authInfo, space.Name)
		})

		It("deletes only the unmapped route resource", func() {
			Expect(deleteErr).NotTo(HaveOccurred())

			var routeList korifiv1alpha1.CFRouteList
			Expect(k8sClient.List(ctx, &routeList, client.InNamespace(space.Name))).To(Succeed())

			Expect(routeList.Items).To(HaveLen(1))
			Expect(routeList.Items[0].Name).To(Equal(cfRoute1.Name))
		})
	})

	Describe("GetOrCreateRoute", func() {
		const (
			routeHost = "test-route-host"
			routePath = "/test/route/path"
		)

		var (
			createRouteMessage repositories.CreateRouteMessage
			routeRecord        repositories.RouteRecord
			routeErr           error
		)
		BeforeEach(func() {
			createRouteMessage = repositories.CreateRouteMessage{
				Host:            routeHost,
				Path:            routePath,
				SpaceGUID:       space.Name,
				DomainGUID:      domainGUID,
				DomainNamespace: rootNamespace,
			}
		})

		JustBeforeEach(func() {
			routeRecord, routeErr = routeRepo.GetOrCreateRoute(ctx, authInfo, createRouteMessage)
		})

		It("errors with forbidden for users with no permissions", func() {
			Expect(routeErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is authorized in space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates a new CFRoute CR successfully", func() {
				cfRoute := &korifiv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: space.Name,
						Name:      routeRecord.GUID,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)).To(Succeed())
			})

			It("returns an RouteRecord with matching fields", func() {
				Expect(routeErr).NotTo(HaveOccurred())
				Expect(routeRecord.GUID).To(matchers.BeValidUUID())
				Expect(routeRecord.Host).To(Equal(routeHost), "Route Host in record did not match input")
				Expect(routeRecord.Path).To(Equal(routePath), "Route Path in record did not match input")
				Expect(routeRecord.SpaceGUID).To(Equal(space.Name), "Route Space GUID in record did not match input")
				Expect(routeRecord.Domain).To(Equal(repositories.DomainRecord{GUID: domainGUID}), "Route Domain in record did not match created domain")

				Expect(routeRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(routeRecord.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
			})

			When("the route already exists", func() {
				var existingRecord repositories.RouteRecord

				BeforeEach(func() {
					var err error
					existingRecord, err = routeRepo.CreateRoute(ctx, authInfo, createRouteMessage)
					Expect(err).NotTo(HaveOccurred())
				})

				It("doesn't create a new route", func() {
					Expect(routeErr).NotTo(HaveOccurred())

					var routeList korifiv1alpha1.CFRouteList
					Expect(k8sClient.List(ctx, &routeList, client.InNamespace(space.Name))).To(Succeed())
					Expect(routeList.Items).To(HaveLen(1))
				})

				It("returns the existing record", func() {
					Expect(routeErr).NotTo(HaveOccurred())
					Expect(routeRecord.GUID).To(Equal(existingRecord.GUID))
				})
			})
		})

		When("the user is a Space Manager", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceManagerRole.Name, space.Name)
			})

			It("errors with forbidden for users with no permissions", func() {
				Expect(routeErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("AddDestinationsToRoute", func() {
		const (
			routeHost = "test-route-host"
			routePath = "/test/route/path"
		)

		var (
			appGUID                string
			addDestinationsMessage repositories.AddDestinationsMessage
			addDestinationErr      error
			cfRoute                *korifiv1alpha1.CFRoute
			routeRecord            repositories.RouteRecord
		)

		BeforeEach(func() {
			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host: routeHost,
					Path: routePath,
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: space.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())

			appGUID = uuid.NewString()

			addDestinationsMessage = repositories.AddDestinationsMessage{
				RouteGUID: routeGUID,
				SpaceGUID: space.Name,
				NewDestinations: []repositories.DesiredDestination{
					{
						AppGUID:     appGUID,
						ProcessType: "web",
						Port:        tools.PtrTo[int32](9090),
						Protocol:    tools.PtrTo("http1"),
					},
				},
			}
		})

		JustBeforeEach(func() {
			routeRecord, addDestinationErr = routeRepo.AddDestinationsToRoute(ctx, authInfo, addDestinationsMessage)
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)).To(Succeed())
		})

		When("the user is a space manager in this space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceManagerRole.Name, space.Name)
			})

			When("route is updated to add new destinations", func() {
				It("returns an error", func() {
					Expect(addDestinationErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})

				It("fails to update the destination list", func() {
					currentRouteRecord, err := routeRepo.GetRoute(ctx, authInfo, routeGUID)
					Expect(err).NotTo(HaveOccurred())
					Expect(currentRouteRecord.Destinations).To(BeEmpty())
				})
			})
		})

		When("the user is a space developer in this space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("adds the destinations", func() {
				Expect(addDestinationErr).NotTo(HaveOccurred())
				Expect(routeRecord.Destinations).To(ConsistOf(
					MatchAllFields(
						Fields{
							"GUID":        Not(BeEmpty()),
							"Port":        PointTo(BeEquivalentTo(9090)),
							"AppGUID":     Equal(appGUID),
							"ProcessType": Equal("web"),
							"Protocol":    PointTo(Equal("http1")),
						},
					),
				))

				Expect(cfRoute.Spec.Destinations).To(ConsistOf(
					MatchAllFields(
						Fields{
							"GUID": Not(BeEmpty()),
							"Port": PointTo(BeEquivalentTo(9090)),
							"AppRef": Equal(corev1.LocalObjectReference{
								Name: appGUID,
							}),
							"ProcessType": Equal("web"),
							"Protocol":    PointTo(Equal("http1")),
						},
					),
				))
			})

			When("the destination has no port and protocol set", func() {
				BeforeEach(func() {
					addDestinationsMessage.NewDestinations[0].Port = nil
					addDestinationsMessage.NewDestinations[0].Protocol = nil
				})

				It("adds the destination", func() {
					Expect(addDestinationErr).NotTo(HaveOccurred())
					Expect(routeRecord.Destinations).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"AppGUID":  Equal(appGUID),
							"Port":     BeNil(),
							"Protocol": BeNil(),
						}),
					))

					Expect(cfRoute.Spec.Destinations).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"AppRef": Equal(corev1.LocalObjectReference{
								Name: appGUID,
							}),
							"Port":     BeNil(),
							"Protocol": BeNil(),
						}),
					))
				})
			})

			When("the route destination has an invalid protocol", func() {
				BeforeEach(func() {
					addDestinationsMessage.NewDestinations[0].Protocol = tools.PtrTo("bad-protocol")
				})

				It("returns an error", func() {
					Expect(addDestinationErr.Error()).To(ContainSubstring("Unsupported value: \"bad-protocol\": supported values: \"http1\""))
				})
			})

			When("there are already destinations on the route", func() {
				var routeDestination korifiv1alpha1.Destination

				BeforeEach(func() {
					routeDestination = korifiv1alpha1.Destination{
						GUID: prefixedGUID("existing-route-guid"),
						Port: tools.PtrTo[int32](8000),
						AppRef: corev1.LocalObjectReference{
							Name: prefixedGUID("existing-route-app"),
						},
						ProcessType: "web",
						Protocol:    tools.PtrTo("http1"),
					}

					Expect(k8s.Patch(ctx, k8sClient, cfRoute, func() {
						cfRoute.Spec.Destinations = []korifiv1alpha1.Destination{routeDestination}
					})).To(Succeed())

					addDestinationsMessage.ExistingDestinations = []repositories.DestinationRecord{{
						GUID:        routeDestination.GUID,
						AppGUID:     routeDestination.AppRef.Name,
						ProcessType: routeDestination.ProcessType,
						Port:        routeDestination.Port,
						Protocol:    routeDestination.Protocol,
					}}
				})

				When("the destinations are all new", func() {
					var (
						appGUID1 string
						appGUID2 string
					)

					BeforeEach(func() {
						appGUID1 = uuid.NewString()
						appGUID2 = uuid.NewString()

						addDestinationsMessage.NewDestinations = []repositories.DesiredDestination{
							{
								AppGUID:     appGUID1,
								ProcessType: "weba",
							},
							{
								AppGUID:     appGUID2,
								ProcessType: "workerb",
							},
							// Duplicate dest that should be ignored
							{
								AppGUID:     appGUID2,
								ProcessType: "workerb",
							},
						}
					})

					It("adds the destinations successfully", func() {
						Expect(cfRoute.Spec.Destinations).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{
								"AppRef": Equal(corev1.LocalObjectReference{
									Name: appGUID1,
								}),
								"ProcessType": Equal("weba"),
							}),
							MatchFields(IgnoreExtras, Fields{
								"AppRef": Equal(corev1.LocalObjectReference{
									Name: appGUID2,
								}),
								"ProcessType": Equal("workerb"),
							}),
							MatchFields(IgnoreExtras, Fields{
								"AppRef": Equal(corev1.LocalObjectReference{
									Name: routeDestination.AppRef.Name,
								}),
								"ProcessType": Equal("web"),
							}),
						))

						Expect(routeRecord.Destinations).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{
								"AppGUID":     Equal(appGUID1),
								"ProcessType": Equal("weba"),
							}),
							MatchFields(IgnoreExtras, Fields{
								"AppGUID":     Equal(appGUID2),
								"ProcessType": Equal("workerb"),
							}),
							MatchFields(IgnoreExtras, Fields{
								"AppGUID":     Equal(routeDestination.AppRef.Name),
								"ProcessType": Equal("web"),
							}),
						))
					})
				})

				When("one of the destinations is already on the route", func() {
					var appGUID2 string

					BeforeEach(func() {
						appGUID2 = uuid.NewString()

						addDestinationsMessage.NewDestinations = []repositories.DesiredDestination{
							{
								AppGUID:     routeDestination.AppRef.Name,
								ProcessType: routeDestination.ProcessType,
								Port:        routeDestination.Port,
								Protocol:    routeDestination.Protocol,
							},
							{
								AppGUID:     appGUID2,
								ProcessType: "worker",
							},
						}
					})

					It("adds only the new destination", func() {
						Expect(addDestinationErr).NotTo(HaveOccurred())
						Expect(cfRoute.Spec.Destinations).To(ConsistOf(
							korifiv1alpha1.Destination{
								GUID: routeDestination.GUID,
								AppRef: corev1.LocalObjectReference{
									Name: routeDestination.AppRef.Name,
								},
								ProcessType: routeDestination.ProcessType,
								Port:        routeDestination.Port,
								Protocol:    routeDestination.Protocol,
							},
							MatchFields(IgnoreExtras, Fields{
								"AppRef": Equal(corev1.LocalObjectReference{
									Name: appGUID2,
								}),
							}),
						))

						Expect(routeRecord.Destinations).To(ConsistOf(
							repositories.DestinationRecord{
								GUID:        routeDestination.GUID,
								AppGUID:     routeDestination.AppRef.Name,
								ProcessType: routeDestination.ProcessType,
								Port:        routeDestination.Port,
								Protocol:    routeDestination.Protocol,
							},
							MatchFields(IgnoreExtras, Fields{
								"AppGUID": Equal(appGUID2),
							}),
						))
					})
				})
			})
		})
	})

	Describe("RemoveDestinationFromRoute", func() {
		const (
			routeHost = "test-route-host"
			routePath = "/test/route/path"
		)

		var (
			destinationGUID      string
			removeDestinationErr error
		)

		BeforeEach(func() {
			destinationGUID = uuid.NewString()

			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host: routeHost,
					Path: routePath,
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: space.Name,
					},
					Destinations: []korifiv1alpha1.Destination{{
						GUID: destinationGUID,
						Port: tools.PtrTo[int32](8000),
						AppRef: corev1.LocalObjectReference{
							Name: uuid.NewString(),
						},
						ProcessType: "web",
						Protocol:    tools.PtrTo("http1"),
					}},
				},
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			_, removeDestinationErr = routeRepo.RemoveDestinationFromRoute(ctx, authInfo, repositories.RemoveDestinationMessage{
				RouteGUID: routeGUID,
				SpaceGUID: space.Name,
				GUID:      destinationGUID,
			})
		})

		It("returns an error as the user is not authorized", func() {
			Expect(removeDestinationErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))

			cfRoute := &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space.Name,
					Name:      routeGUID,
				},
			}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)).To(Succeed())
			Expect(cfRoute.Spec.Destinations).To(HaveLen(1))
		})

		When("the user is a space developer in this space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("removes the destination from CFRoute successfully", func() {
				Expect(removeDestinationErr).NotTo(HaveOccurred())
				updatedCFRoute := &korifiv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: space.Name,
						Name:      routeGUID,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(updatedCFRoute), updatedCFRoute)).To(Succeed())

				Expect(updatedCFRoute.Spec.Destinations).To(BeEmpty())
			})

			When("the destination isn't on the route", func() {
				BeforeEach(func() {
					destinationGUID = "some-bogus-guid"
				})

				It("returns an unprocessable entity error", func() {
					Expect(removeDestinationErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
				})
			})
		})
	})

	Describe("PatchRouteMetadata", func() {
		var (
			cfRoute                       *korifiv1alpha1.CFRoute
			labelsPatch, annotationsPatch map[string]*string
			patchErr                      error
			routeRecord                   repositories.RouteRecord
		)

		BeforeEach(func() {
			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1-a",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
				},
			}
			Expect(
				k8sClient.Create(ctx, cfRoute),
			).To(Succeed())

			labelsPatch = nil
			annotationsPatch = nil
		})

		JustBeforeEach(func() {
			patchMsg := repositories.PatchRouteMetadataMessage{
				RouteGUID: routeGUID,
				SpaceGUID: space.Name,
				MetadataPatch: repositories.MetadataPatch{
					Annotations: annotationsPatch,
					Labels:      labelsPatch,
				},
			}

			routeRecord, patchErr = routeRepo.PatchRouteMetadata(ctx, authInfo, patchMsg)
		})

		It("return a forbidden error as the user is not authorized", func() {
			Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is authorized", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			When("the route doesn't have any labels or annotations", func() {
				BeforeEach(func() {
					labelsPatch = map[string]*string{
						"key-one": tools.PtrTo("value-one"),
						"key-two": tools.PtrTo("value-two"),
					}
					annotationsPatch = map[string]*string{
						"key-one": tools.PtrTo("value-one"),
						"key-two": tools.PtrTo("value-two"),
					}
				})

				It("returns the updated route record", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					Expect(routeRecord.GUID).To(Equal(routeGUID))
					Expect(routeRecord.SpaceGUID).To(Equal(space.Name))
					Expect(routeRecord.Labels).To(HaveKeyWithValue("key-one", "value-one"))
					Expect(routeRecord.Labels).To(HaveKeyWithValue("key-two", "value-two"))
					Expect(routeRecord.Annotations).To(HaveKeyWithValue("key-one", "value-one"))
					Expect(routeRecord.Annotations).To(HaveKeyWithValue("key-two", "value-two"))
				})

				It("sets the k8s CFRoute resource", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					updatedCFRoute := new(korifiv1alpha1.CFRoute)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), updatedCFRoute)).To(Succeed())
					Expect(updatedCFRoute.Labels).To(HaveKeyWithValue("key-one", "value-one"))
					Expect(updatedCFRoute.Labels).To(HaveKeyWithValue("key-two", "value-two"))
					Expect(updatedCFRoute.Annotations).To(HaveKeyWithValue("key-one", "value-one"))
					Expect(updatedCFRoute.Annotations).To(HaveKeyWithValue("key-two", "value-two"))
				})
			})

			When("the route already has labels and annotations", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, k8sClient, cfRoute, func() {
						cfRoute.Labels = map[string]string{
							"before-key-one": "value-one",
							"before-key-two": "value-two",
							"key-one":        "value-one",
						}
						cfRoute.Annotations = map[string]string{
							"before-key-one": "value-one",
							"before-key-two": "value-two",
							"key-one":        "value-one",
						}
					})).To(Succeed())

					labelsPatch = map[string]*string{
						"key-one":        tools.PtrTo("value-one-updated"),
						"key-two":        tools.PtrTo("value-two"),
						"before-key-two": nil,
					}
					annotationsPatch = map[string]*string{
						"key-one":        tools.PtrTo("value-one-updated"),
						"key-two":        tools.PtrTo("value-two"),
						"before-key-two": nil,
					}
				})

				It("returns the updated route record", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					Expect(routeRecord.GUID).To(Equal(routeGUID))
					Expect(routeRecord.SpaceGUID).To(Equal(space.Name))
					Expect(routeRecord.Labels).To(SatisfyAll(
						HaveKeyWithValue("before-key-one", "value-one"),
						HaveKeyWithValue("key-one", "value-one-updated"),
						HaveKeyWithValue("key-two", "value-two"),
					))
					Expect(routeRecord.Annotations).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
				})

				It("sets the k8s CFRoute resource", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					updatedCFRoute := new(korifiv1alpha1.CFRoute)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), updatedCFRoute)).To(Succeed())
					Expect(updatedCFRoute.Labels).To(SatisfyAll(
						HaveKeyWithValue("before-key-one", "value-one"),
						HaveKeyWithValue("key-one", "value-one-updated"),
						HaveKeyWithValue("key-two", "value-two"),
					))
					Expect(updatedCFRoute.Annotations).To(Equal(
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
						"-bad-annotation": tools.PtrTo("stuff"),
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
						"-bad-label": tools.PtrTo("stuff"),
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

			When("the route does not exist", func() {
				BeforeEach(func() {
					routeGUID = "invalidRouteGUID"
				})

				It("fails to get the Route", func() {
					Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})
	})

	Describe("GetDeletedAt", func() {
		var (
			cfRoute   *korifiv1alpha1.CFRoute
			deletedAt *time.Time
			getErr    error
		)

		BeforeEach(func() {
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)

			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeGUID,
					Namespace: space.Name,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []korifiv1alpha1.Destination{
						{
							GUID: "destination-guid",
							Port: tools.PtrTo[int32](8080),
							AppRef: corev1.LocalObjectReference{
								Name: "some-app-guid",
							},
							ProcessType: "web",
							Protocol:    tools.PtrTo("http1"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute)).To(Succeed())
		})

		JustBeforeEach(func() {
			deletedAt, getErr = routeRepo.GetDeletedAt(ctx, authInfo, routeGUID)
		})

		It("returns nil", func() {
			Expect(getErr).NotTo(HaveOccurred())
			Expect(deletedAt).To(BeNil())
		})

		When("the route is being deleted", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, k8sClient, cfRoute, func() {
					cfRoute.Finalizers = append(cfRoute.Finalizers, "foo")
				})).To(Succeed())

				Expect(k8sClient.Delete(ctx, cfRoute)).To(Succeed())
			})

			It("returns the deletion time", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(deletedAt).To(PointTo(BeTemporally("~", time.Now(), time.Minute)))
			})
		})

		When("the route isn't found", func() {
			BeforeEach(func() {
				Expect(k8sClient.Delete(ctx, cfRoute)).To(Succeed())
			})

			It("errors", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})
})
