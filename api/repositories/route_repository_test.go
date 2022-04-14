package repositories_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	. "code.cloudfoundry.org/korifi/api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/networking/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("RouteRepository", func() {
	const domainName = "my-domain-name"

	var (
		testCtx context.Context

		org   *hnsv1alpha2.SubnamespaceAnchor
		space *hnsv1alpha2.SubnamespaceAnchor

		route1GUID string
		route2GUID string
		domainGUID string
		routeRepo  *RouteRepo
	)

	validateRoute := func(route RouteRecord, expectedRoute *networkingv1alpha1.CFRoute) {
		By("returning a routeRecord in the list for one of the created CRs", func() {
			Expect(route.GUID).To(Equal(expectedRoute.Name))
			Expect(route.Host).To(Equal(expectedRoute.Spec.Host))
			Expect(route.SpaceGUID).To(Equal(expectedRoute.Namespace))
			Expect(route.Path).To(Equal(expectedRoute.Spec.Path))
			Expect(route.Protocol).To(Equal(string(expectedRoute.Spec.Protocol)))
			Expect(route.Domain).To(Equal(DomainRecord{GUID: expectedRoute.Spec.DomainRef.Name}))

			Expect(route.Destinations).To(Equal([]DestinationRecord{
				{
					GUID:        expectedRoute.Spec.Destinations[0].GUID,
					AppGUID:     expectedRoute.Spec.Destinations[0].AppRef.Name,
					Port:        expectedRoute.Spec.Destinations[0].Port,
					ProcessType: expectedRoute.Spec.Destinations[0].ProcessType,
					Protocol:    expectedRoute.Spec.Destinations[0].Protocol,
				},
			}))

			validateTimestamp(route.CreatedAt, timeCheckThreshold*time.Second)
			validateTimestamp(route.UpdatedAt, timeCheckThreshold*time.Second)
		})
	}

	BeforeEach(func() {
		testCtx = context.Background()

		org = createOrgWithCleanup(testCtx, prefixedGUID("org"))
		space = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space1"))

		route1GUID = prefixedGUID("route1")
		route2GUID = prefixedGUID("route2")
		domainGUID = prefixedGUID("domain")
		routeRepo = NewRouteRepo(namespaceRetriever, userClientFactory, nsPerms)

		cfDomain := &networkingv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      domainGUID,
				Namespace: rootNamespace,
			},
			Spec: networkingv1alpha1.CFDomainSpec{
				Name: domainName,
			},
		}
		Expect(k8sClient.Create(testCtx, cfDomain)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(testCtx, &networkingv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{Name: domainGUID, Namespace: rootNamespace},
		})).To(Succeed())
	})

	Describe("GetRoute", func() {
		var (
			cfRoute1 *networkingv1alpha1.CFRoute
			cfRoute2 *networkingv1alpha1.CFRoute
			route    RouteRecord
			getErr   error
		)

		BeforeEach(func() {
			cfRoute1 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      route1GUID,
					Namespace: space.Name,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []networkingv1alpha1.Destination{
						{
							GUID: "destination-guid",
							Port: 8080,
							AppRef: corev1.LocalObjectReference{
								Name: "some-app-guid",
							},
							ProcessType: "web",
							Protocol:    "http1",
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, cfRoute1)).To(Succeed())

			cfRoute2 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      route2GUID,
					Namespace: space.Name,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-2",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []networkingv1alpha1.Destination{},
				},
			}
			Expect(k8sClient.Create(testCtx, cfRoute2)).To(Succeed())
		})

		JustBeforeEach(func() {
			route, getErr = routeRepo.GetRoute(testCtx, authInfo, route1GUID)
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(testCtx, cfRoute1)).To(Succeed())
			Expect(k8sClient.Delete(testCtx, cfRoute2)).To(Succeed())
		})

		It("returns a forbidden error for unauthorized users", func() {
			_, err := routeRepo.GetRoute(testCtx, authInfo, route1GUID)
			Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a space developer in this space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("fetches the CFRoute CR we're looking for", func() {
				Expect(getErr).ToNot(HaveOccurred())

				Expect(route.GUID).To(Equal(cfRoute1.Name))
				Expect(route.Host).To(Equal(cfRoute1.Spec.Host))
				Expect(route.SpaceGUID).To(Equal(cfRoute1.Namespace))
				Expect(route.Path).To(Equal(cfRoute1.Spec.Path))
				Expect(route.Protocol).To(Equal(string(cfRoute1.Spec.Protocol)))

				By("returning a record with destinations that match the CFRoute CR", func() {
					Expect(route.Destinations).To(HaveLen(len(cfRoute1.Spec.Destinations)), "Route Record Destinations returned was not the correct length")
					destinationRecord := route.Destinations[0]
					Expect(destinationRecord.GUID).To(Equal(cfRoute1.Spec.Destinations[0].GUID))
					Expect(destinationRecord.AppGUID).To(Equal(cfRoute1.Spec.Destinations[0].AppRef.Name))
					Expect(destinationRecord.Port).To(Equal(cfRoute1.Spec.Destinations[0].Port))
					Expect(destinationRecord.ProcessType).To(Equal(cfRoute1.Spec.Destinations[0].ProcessType))
					Expect(destinationRecord.Protocol).To(Equal(cfRoute1.Spec.Destinations[0].Protocol))
				})

				By("returning a record where the CreatedAt and UpdatedAt match the CR creation time", func() {
					validateTimestamp(route.CreatedAt, timeCheckThreshold*time.Second)
					validateTimestamp(route.UpdatedAt, timeCheckThreshold*time.Second)
				})

				Expect(route.Domain).To(Equal(DomainRecord{GUID: domainGUID}))
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the CFRoute doesn't exist", func() {
			BeforeEach(func() {
				route1GUID = "non-existent-route-guid"
			})

			It("returns an error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

		When("multiple CFRoute resources exist across namespaces with the same name", func() {
			var (
				otherNamespaceGUID string
				otherNamespace     *corev1.Namespace

				cfRoute1A *networkingv1alpha1.CFRoute
			)

			BeforeEach(func() {
				// Create second namespace aside from default within which to create a duplicate route
				otherNamespaceGUID = generateGUID()
				otherNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: otherNamespaceGUID}}
				Expect(k8sClient.Create(testCtx, otherNamespace)).To(Succeed())

				cfRoute1A = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      route1GUID,
						Namespace: otherNamespaceGUID,
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-1",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.ObjectReference{
							Name:      domainGUID,
							Namespace: rootNamespace,
						},
					},
				}
				Expect(k8sClient.Create(testCtx, cfRoute1A)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(testCtx, cfRoute1A)).To(Succeed())
				Expect(k8sClient.Delete(testCtx, otherNamespace)).To(Succeed())
			})

			It("returns an error", func() {
				// Looks like we can continue doing state-based setup for the time being
				// Assumption: when unit testing, we can ignore webhooks that might turn the uniqueness constraint into a race condition
				// If assumption is invalidated, we can implement the setup by mocking a fake client to return the non-unique ids

				Expect(getErr).To(MatchError(ContainSubstring("get-route duplicate records exist")))
			})
		})
	})

	Describe("ListRoutes", func() {
		When("multiple CFRoutes exist", func() {
			var (
				cfRoute1A, cfRoute1B *networkingv1alpha1.CFRoute
				domainGUID2          string
				space2               *hnsv1alpha2.SubnamespaceAnchor
				cfRoute2A            *networkingv1alpha1.CFRoute
				space3               *hnsv1alpha2.SubnamespaceAnchor
				cfRoute3A            *networkingv1alpha1.CFRoute

				routeRecords []RouteRecord
				message      ListRoutesMessage
			)

			BeforeEach(func() {
				cfRoute1A = createRoute(route1GUID, space.Name, "my-subdomain-1-a", "", domainGUID, prefixedGUID("RouteListApp"))
				domainGUID2 = prefixedGUID("RouteListDomain2")
				cfRoute1B = createRoute(route2GUID, space.Name, "my-subdomain-1-b", "/some/path", domainGUID2, "some-app-guid-2")

				space2 = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space2"))
				cfRoute2A = createRoute(generateGUID(), space2.Name, "my-subdomain-2-a", "/some/other/path", domainGUID, "some-app-guid-3")

				space3 = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space3"))
				cfRoute3A = createRoute(generateGUID(), space3.Name, "my-subdomain-3-a", "", domainGUID, "some-app-guid-4")

				message = ListRoutesMessage{}
			})

			JustBeforeEach(func() {
				var err error
				routeRecords, err = routeRepo.ListRoutes(testCtx, authInfo, message)
				Expect(err).NotTo(HaveOccurred())
			})

			When("the user has space developer access in space1 & space2, but not space3", func() {
				BeforeEach(func() {
					createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
					createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space2.Name)
				})

				When("filters are not provided", func() {
					It("returns a list of routeRecords for each CFRoute CR", func() {
						Expect(routeRecords).To(ContainElements(
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfRoute1A.Name)}),
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfRoute1B.Name)}),
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfRoute2A.Name)}),
						))
						Expect(routeRecords).ToNot(ContainElement(MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfRoute3A.Name)})))

						var route1A, route1B, route2A RouteRecord
						for _, routeRecord := range routeRecords {
							switch routeRecord.GUID {
							case cfRoute1A.Name:
								route1A = routeRecord
							case cfRoute1B.Name:
								route1B = routeRecord
							case cfRoute2A.Name:
								route2A = routeRecord
							default:
							}
						}

						Expect(route1A).NotTo(BeZero())
						Expect(route1B).NotTo(BeZero())
						Expect(route2A).NotTo(BeZero())

						validateRoute(route1A, cfRoute1A)
						validateRoute(route1B, cfRoute1B)
						validateRoute(route2A, cfRoute2A)
					})
				})

				When("space_guid filters are provided", func() {
					BeforeEach(func() {
						message = ListRoutesMessage{SpaceGUIDs: []string{space2.Name}}
					})

					It("returns a list of routeRecords for each CFRoute CR", func() {
						Expect(routeRecords).To(HaveLen(1))
						Expect(routeRecords[0].GUID).To(Equal(cfRoute2A.Name))
					})
				})

				When("domain_guid filters are provided", func() {
					BeforeEach(func() {
						message = ListRoutesMessage{DomainGUIDs: []string{domainGUID2}}
					})

					It("returns a list of routeRecords for each CFRoute CR", func() {
						Expect(routeRecords).To(HaveLen(1))
						Expect(routeRecords[0].GUID).To(Equal(cfRoute1B.Name))
					})
				})

				When("host filters are provided", func() {
					BeforeEach(func() {
						message = ListRoutesMessage{Hosts: []string{"my-subdomain-1-a"}}
					})

					It("returns a list of routeRecords for one of the CFRoute CRs", func() {
						Expect(routeRecords).To(HaveLen(1))
						Expect(routeRecords[0].GUID).To(Equal(cfRoute1A.Name))
					})
				})

				When("path filters are provided", func() {
					BeforeEach(func() {
						message = ListRoutesMessage{Paths: []string{"/some/path"}}
					})

					It("returns a list of routeRecords for one of the CFRoute CRs", func() {
						Expect(routeRecords).To(HaveLen(1))
						Expect(routeRecords[0].GUID).To(Equal(cfRoute1B.Name))
						Expect(routeRecords[0].Path).To(Equal("/some/path"))
					})
				})

				When("an empty path filter is provided", func() {
					BeforeEach(func() {
						message = ListRoutesMessage{
							SpaceGUIDs: []string{space.Name},
							Paths:      []string{""},
						}
					})

					It("returns a list of routeRecords for one of the CFRoute CRs", func() {
						Expect(routeRecords).To(HaveLen(1))
						Expect(routeRecords[0].GUID).To(Equal(cfRoute1A.Name))
						Expect(routeRecords[0].Path).To(Equal(""))
					})
				})

				When("app_guid filters are provided", func() {
					BeforeEach(func() {
						message = ListRoutesMessage{AppGUIDs: []string{cfRoute1A.Spec.Destinations[0].AppRef.Name}}
					})

					It("returns a list of routeRecords for each CFRoute CR", func() {
						Expect(routeRecords).To(HaveLen(1))
						route1 := routeRecords[0]
						Expect(route1).NotTo(BeZero())
						validateRoute(route1, cfRoute1A)
					})
				})

				When("non-matching space_guid filters are provided", func() {
					BeforeEach(func() {
						message = ListRoutesMessage{SpaceGUIDs: []string{"something-not-matching"}}
					})

					It("returns a list of routeRecords for each CFRoute CR", func() {
						Expect(routeRecords).To(BeEmpty())
					})
				})

				When("non-matching domain_guid filters are provided", func() {
					BeforeEach(func() {
						message = ListRoutesMessage{DomainGUIDs: []string{"something-not-matching"}}
					})

					It("returns a list of routeRecords for each CFRoute CR", func() {
						Expect(routeRecords).To(BeEmpty())
					})
				})
			})

			When("the user does not have space developer permissions", func() {
				It("returns an empty list and no error", func() {
					Expect(routeRecords).To(BeEmpty())
				})
			})
		})
	})

	Describe("ListRoutesForApp", func() {
		var (
			appGUID      string
			cfRoute1     *networkingv1alpha1.CFRoute
			cfRoute2     *networkingv1alpha1.CFRoute
			routeRecords []RouteRecord
			listErr      error
			queryAppGUID string
		)

		BeforeEach(func() {
			appGUID = generateGUID()

			cfRoute1 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      route1GUID,
					Namespace: space.Name,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []networkingv1alpha1.Destination{
						{
							GUID: "destination-guid",
							Port: 8080,
							AppRef: corev1.LocalObjectReference{
								Name: appGUID,
							},
							ProcessType: "web",
							Protocol:    "http1",
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, cfRoute1)).To(Succeed())

			queryAppGUID = appGUID
		})

		JustBeforeEach(func() {
			routeRecords, listErr = routeRepo.ListRoutesForApp(testCtx, authInfo, queryAppGUID, space.Name)
		})

		When("the user is authorized in space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			When("multiple CFRoutes exist", func() {
				BeforeEach(func() {
					cfRoute2 = &networkingv1alpha1.CFRoute{
						ObjectMeta: metav1.ObjectMeta{
							Name:      route2GUID,
							Namespace: space.Name,
						},
						Spec: networkingv1alpha1.CFRouteSpec{
							Host:     "my-subdomain-2",
							Path:     "",
							Protocol: "http",
							DomainRef: corev1.ObjectReference{
								Name:      domainGUID,
								Namespace: rootNamespace,
							},
							Destinations: []networkingv1alpha1.Destination{},
						},
					}
					Expect(k8sClient.Create(testCtx, cfRoute2)).To(Succeed())
				})

				It("returns a list of routeRecords for each CFRoute CR", func() {
					Expect(listErr).NotTo(HaveOccurred())

					By("returning a routeRecord in the list for one of the created CRs", func() {
						var route RouteRecord
						var found bool
						for _, routeRecord := range routeRecords {
							if routeRecord.GUID == cfRoute1.Name {
								found = true
								route = routeRecord
								break
							}
						}
						Expect(found).To(BeTrue(), "could not find matching record")

						By("returning a record with metadata fields from the CFRoute CR", func() {
							Expect(route.GUID).To(Equal(cfRoute1.Name))
							Expect(route.Host).To(Equal(cfRoute1.Spec.Host))
							Expect(route.SpaceGUID).To(Equal(cfRoute1.Namespace))
						})

						By("returning a record with spec fields from the CFRoute CR", func() {
							Expect(route.Path).To(Equal(cfRoute1.Spec.Path))
							Expect(route.Protocol).To(Equal(string(cfRoute1.Spec.Protocol)))
							Expect(route.Domain.GUID).To(Equal(cfRoute1.Spec.DomainRef.Name))
						})

						By("returning a record with destinations that match the CFRoute CR", func() {
							Expect(len(route.Destinations)).To(Equal(len(cfRoute1.Spec.Destinations)), "Route Record Destinations returned was not the correct length")
							destinationRecord := route.Destinations[0]
							Expect(destinationRecord.GUID).To(Equal(cfRoute1.Spec.Destinations[0].GUID))
							Expect(destinationRecord.AppGUID).To(Equal(cfRoute1.Spec.Destinations[0].AppRef.Name))
							Expect(destinationRecord.Port).To(Equal(cfRoute1.Spec.Destinations[0].Port))
							Expect(destinationRecord.ProcessType).To(Equal(cfRoute1.Spec.Destinations[0].ProcessType))
							Expect(destinationRecord.Protocol).To(Equal(cfRoute1.Spec.Destinations[0].Protocol))
						})

						By("returning a record where the CreatedAt and UpdatedAt match the CR creation time", func() {
							validateTimestamp(route.CreatedAt, timeCheckThreshold*time.Second)
							validateTimestamp(route.UpdatedAt, timeCheckThreshold*time.Second)
						})
					})
				})
			})

			When("no CFRoutes exist for the app", func() {
				BeforeEach(func() {
					queryAppGUID = "i-dont-exist"
				})

				It("returns an empty list and no error", func() {
					Expect(listErr).ToNot(HaveOccurred())
					Expect(routeRecords).To(BeEmpty())
				})
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(listErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("CreateRoute", func() {
		var (
			createdRouteRecord RouteRecord
			createdRouteErr    error
			testRouteHost      string
			testRoutePath      string
			targetNamespace    string
		)

		BeforeEach(func() {
			targetNamespace = space.Name
			testRouteHost = prefixedGUID("route-host-")
			testRoutePath = prefixedGUID("/test/route/")
			createdRouteRecord = RouteRecord{}
			createdRouteErr = nil
		})
		JustBeforeEach(func() {
			createRouteMessage := buildCreateRouteMessage(testRouteHost, testRoutePath, domainGUID, targetNamespace, rootNamespace)
			createdRouteRecord, createdRouteErr = routeRepo.CreateRoute(testCtx, authInfo, createRouteMessage)
		})

		It("errors with forbidden for users with no permissions", func() {
			Expect(createdRouteErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a space developer", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates a new CFRoute CR successfully", func() {
				Expect(createdRouteErr).NotTo(HaveOccurred())
				cfRouteLookupKey := types.NamespacedName{Name: createdRouteRecord.GUID, Namespace: space.Name}
				createdCFRoute := new(networkingv1alpha1.CFRoute)
				Expect(k8sClient.Get(context.Background(), cfRouteLookupKey, createdCFRoute)).To(Succeed())
			})

			It("returns a RouteRecord with matching fields", func() {
				Expect(createdRouteRecord.GUID).To(HavePrefix("cf-route-"))
				Expect(createdRouteRecord.Host).To(Equal(testRouteHost), "Route Host in record did not match input")
				Expect(createdRouteRecord.Path).To(Equal(testRoutePath), "Route Path in record did not match input")
				Expect(createdRouteRecord.SpaceGUID).To(Equal(space.Name), "Route Space GUID in record did not match input")
				Expect(createdRouteRecord.Domain).To(Equal(DomainRecord{GUID: domainGUID}), "Route Domain in record did not match created domain")

				validateTimestamp(createdRouteRecord.CreatedAt, 2*time.Second)
				validateTimestamp(createdRouteRecord.UpdatedAt, 2*time.Second)
			})

			When("target namespace isn't set", func() {
				BeforeEach(func() {
					targetNamespace = ""
				})
				It("returns an error", func() {
					Expect(createdRouteErr).To(MatchError("an empty namespace may not be set during creation"))
				})
			})
		})
	})

	Describe("DeleteRoute", func() {
		var cfRoute1 *networkingv1alpha1.CFRoute

		BeforeEach(func() {
			cfRoute1 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      route1GUID,
					Namespace: space.Name,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.ObjectReference{
						Name:      domainGUID,
						Namespace: rootNamespace,
					},
					Destinations: []networkingv1alpha1.Destination{
						{
							GUID: "destination-guid",
							Port: 8080,
							AppRef: corev1.LocalObjectReference{
								Name: "some-app-guid",
							},
							ProcessType: "web",
							Protocol:    "http1",
						},
					},
				},
			}
			Expect(
				k8sClient.Create(testCtx, cfRoute1),
			).To(Succeed())
		})

		When("the user has permission to delete routes and", func() {
			BeforeEach(func() {
				beforeCtx := context.Background()
				createRoleBinding(beforeCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			When("on the happy path", func() {
				It("deletes the route resource", func() {
					err := routeRepo.DeleteRoute(testCtx, authInfo, DeleteRouteMessage{
						GUID:      route1GUID,
						SpaceGUID: space.Name,
					})
					Expect(err).NotTo(HaveOccurred())

					err = k8sClient.Get(testCtx, client.ObjectKey{Namespace: space.Name, Name: route1GUID}, &networkingv1alpha1.CFRoute{})
					Expect(err).To(MatchError(ContainSubstring("not found")))
				})
			})

			When("the route doesn't exist", func() {
				AfterEach(func() {
					Expect(k8sClient.Delete(context.Background(), cfRoute1)).To(Succeed())
				})

				It("errors", func() {
					err := routeRepo.DeleteRoute(testCtx, authInfo, DeleteRouteMessage{
						GUID:      "i-don't-exist",
						SpaceGUID: space.Name,
					})
					Expect(err).To(MatchError(ContainSubstring("not found")))
				})
			})
		})

		When("the user does not have permission to delete route and", func() {
			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), cfRoute1)).To(Succeed())
			})

			It("errors with forbidden", func() {
				err := routeRepo.DeleteRoute(testCtx, authInfo, DeleteRouteMessage{
					GUID:      route1GUID,
					SpaceGUID: space.Name,
				})
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})

			When("the route doesn't exist", func() {
				It("errors with forbidden", func() {
					err := routeRepo.DeleteRoute(testCtx, authInfo, DeleteRouteMessage{
						GUID:      "i-don't-exist",
						SpaceGUID: space.Name,
					})
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})
			})
		})
	})

	Describe("GetOrCreateRoute", func() {
		const (
			testRouteHost = "test-route-host"
			testRoutePath = "/test/route/path"
		)

		var (
			createRouteMessage CreateRouteMessage
			routeRecord        RouteRecord
			routeErr           error
		)
		BeforeEach(func() {
			createRouteMessage = buildCreateRouteMessage(testRouteHost, testRoutePath, domainGUID, space.Name, rootNamespace)
		})

		JustBeforeEach(func() {
			routeRecord, routeErr = routeRepo.GetOrCreateRoute(testCtx, authInfo, createRouteMessage)
		})

		When("the user is authorized in space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates a new CFRoute CR successfully", func() {
				cfRouteLookupKey := types.NamespacedName{Name: routeRecord.GUID, Namespace: space.Name}
				createdCFRoute := new(networkingv1alpha1.CFRoute)
				Expect(k8sClient.Get(context.Background(), cfRouteLookupKey, createdCFRoute)).To(Succeed())
			})

			It("returns an RouteRecord with matching fields", func() {
				Expect(routeErr).NotTo(HaveOccurred())
				Expect(routeRecord.GUID).To(HavePrefix("cf-route-"))
				Expect(routeRecord.Host).To(Equal(testRouteHost), "Route Host in record did not match input")
				Expect(routeRecord.Path).To(Equal(testRoutePath), "Route Path in record did not match input")
				Expect(routeRecord.SpaceGUID).To(Equal(space.Name), "Route Space GUID in record did not match input")
				Expect(routeRecord.Domain).To(Equal(DomainRecord{GUID: domainGUID}), "Route Domain in record did not match created domain")

				validateTimestamp(routeRecord.CreatedAt, 2*time.Second)
				validateTimestamp(routeRecord.UpdatedAt, 2*time.Second)
			})

			When("the route already exists", func() {
				var existingRecord RouteRecord

				BeforeEach(func() {
					var err error
					existingRecord, err = routeRepo.CreateRoute(testCtx, authInfo, createRouteMessage)
					Expect(err).NotTo(HaveOccurred())
				})

				It("doesn't create a new route", func() {
					Expect(routeErr).NotTo(HaveOccurred())

					var routeList networkingv1alpha1.CFRouteList
					Expect(k8sClient.List(testCtx, &routeList, client.InNamespace(space.Name))).To(Succeed())
					Expect(routeList.Items).To(HaveLen(1))
				})

				It("returns the existing record", func() {
					Expect(routeErr).NotTo(HaveOccurred())
					Expect(routeRecord).To(Equal(existingRecord))
				})
			})
		})

		When("the user is a Space Manager", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceManagerRole.Name, space.Name)
			})

			It("errors with forbidden for users with no permissions", func() {
				Expect(routeErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user is not authorized", func() {
			It("errors with forbidden for users with no permissions", func() {
				Expect(routeErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("AddDestinationsToRoute", func() {
		const (
			testRouteHost = "test-route-host"
			testRoutePath = "/test/route/path"
		)

		When("the user is a space manager in this space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceManagerRole.Name, space.Name)
				cfRoute := initializeRouteCR(testRouteHost, testRoutePath, route1GUID, domainGUID, space.Name)
				Expect(k8sClient.Create(testCtx, cfRoute)).To(Succeed())
			})

			When("route is updated to add new destinations", func() {
				var (
					routeRecord       RouteRecord
					addDestinationErr error
				)

				BeforeEach(func() {
					appGUID1 := generateGUID()
					destinationMessages := []DestinationMessage{
						{
							AppGUID:     appGUID1,
							ProcessType: "web",
							Port:        8080,
							Protocol:    "http1",
						},
					}

					var err error
					routeRecord, err = routeRepo.GetRoute(testCtx, authInfo, route1GUID)
					Expect(err).NotTo(HaveOccurred())

					destinationListCreateMessage := initializeDestinationListMessage(routeRecord.GUID, routeRecord.SpaceGUID, routeRecord.Destinations, destinationMessages)
					_, addDestinationErr = routeRepo.AddDestinationsToRoute(testCtx, authInfo, destinationListCreateMessage)
				})

				It("returns an error", func() {
					Expect(addDestinationErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})

				It("fails to update the destination list", func() {
					currentRouteRecord, err := routeRepo.GetRoute(testCtx, authInfo, route1GUID)
					Expect(err).NotTo(HaveOccurred())
					Expect(currentRouteRecord).To(Equal(routeRecord))
				})
			})
		})

		When("the user is a space developer in this space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space.Name)
			})
			When("the route exists with no destinations", func() {
				BeforeEach(func() {
					cfRoute := initializeRouteCR(testRouteHost, testRoutePath, route1GUID, domainGUID, space.Name)
					Expect(k8sClient.Create(testCtx, cfRoute)).To(Succeed())
				})

				AfterEach(func() {
					Expect(cleanupRoute(k8sClient, testCtx, route1GUID, space.Name)).To(Succeed())
				})

				When("route is updated to add new destinations", func() {
					var (
						appGUID1            string
						appGUID2            string
						destinationMessages []DestinationMessage
						patchedRouteRecord  RouteRecord
						addDestinationErr   error
					)

					BeforeEach(func() {
						appGUID1 = generateGUID()
						appGUID2 = generateGUID()
						destinationMessages = []DestinationMessage{
							{
								AppGUID:     appGUID1,
								ProcessType: "web",
								Port:        8080,
								Protocol:    "http1",
							},
							{
								AppGUID:     appGUID2,
								ProcessType: "worker",
								Port:        9000,
								Protocol:    "http1",
							},
						}

						// initialize a DestinationListMessage
						destinationListCreateMessage := initializeDestinationListMessage(route1GUID, space.Name, []DestinationRecord{}, destinationMessages)
						patchedRouteRecord, addDestinationErr = routeRepo.AddDestinationsToRoute(testCtx, authInfo, destinationListCreateMessage)
						Expect(addDestinationErr).NotTo(HaveOccurred())
					})

					It("adds the destinations to CFRoute successfully", func() {
						cfRouteLookupKey := types.NamespacedName{Name: route1GUID, Namespace: space.Name}
						createdCFRoute := new(networkingv1alpha1.CFRoute)
						Expect(k8sClient.Get(testCtx, cfRouteLookupKey, createdCFRoute)).To(Succeed())

						Expect(createdCFRoute.Spec.Destinations).To(ConsistOf(
							MatchAllFields(
								Fields{
									"GUID": Not(BeEmpty()),
									"Port": Equal(8080),
									"AppRef": Equal(corev1.LocalObjectReference{
										Name: appGUID1,
									}),
									"ProcessType": Equal("web"),
									"Protocol":    Equal("http1"),
								},
							),
							MatchAllFields(
								Fields{
									"GUID": Not(BeEmpty()),
									"Port": Equal(9000),
									"AppRef": Equal(corev1.LocalObjectReference{
										Name: appGUID2,
									}),
									"ProcessType": Equal("worker"),
									"Protocol":    Equal("http1"),
								},
							),
						))
					})

					It("returns RouteRecord with new destinations", func() {
						Expect(patchedRouteRecord.Destinations).To(ConsistOf(
							MatchAllFields(
								Fields{
									"GUID":        Not(BeEmpty()),
									"Port":        Equal(8080),
									"AppGUID":     Equal(appGUID1),
									"ProcessType": Equal("web"),
									"Protocol":    Equal("http1"),
								},
							),
							MatchAllFields(
								Fields{
									"GUID":        Not(BeEmpty()),
									"Port":        Equal(9000),
									"AppGUID":     Equal(appGUID2),
									"ProcessType": Equal("worker"),
									"Protocol":    Equal("http1"),
								},
							),
						))
					})
				})

				When("the route destination has an invalid protocol", func() {
					It("returns an error", func() {
						appGUID := generateGUID()
						destinationMessages := []DestinationMessage{
							{
								AppGUID:     appGUID,
								ProcessType: "web",
								Port:        8080,
								Protocol:    "bad-protocol",
							},
						}

						// initialize a DestinationListMessage
						destinationListCreateMessage := initializeDestinationListMessage(route1GUID, space.Name, []DestinationRecord{}, destinationMessages)
						_, addDestinationErr := routeRepo.AddDestinationsToRoute(testCtx, authInfo, destinationListCreateMessage)
						Expect(addDestinationErr.Error()).To(ContainSubstring("Unsupported value: \"bad-protocol\": supported values: \"http1\""))
					})
				})
			})

			When("the route exists with a destination", func() {
				var (
					routeDestination networkingv1alpha1.Destination
					destinationGUID  string
					appGUID          string
				)

				BeforeEach(func() {
					cfRoute := initializeRouteCR(testRouteHost, testRoutePath, route1GUID, domainGUID, space.Name)

					destinationGUID = generateGUID()
					appGUID = generateGUID()
					routeDestination = networkingv1alpha1.Destination{
						GUID: destinationGUID,
						Port: 8000,
						AppRef: corev1.LocalObjectReference{
							Name: appGUID,
						},
						ProcessType: "web",
						Protocol:    "http1",
					}

					cfRoute.Spec.Destinations = []networkingv1alpha1.Destination{routeDestination}
					Expect(k8sClient.Create(testCtx, cfRoute)).To(Succeed())
				})

				AfterEach(func() {
					Expect(cleanupRoute(k8sClient, testCtx, route1GUID, space.Name)).To(Succeed())
				})

				When("the destinations are all new", func() {
					var (
						appGUID1            string
						appGUID2            string
						destinationMessages []DestinationMessage
						patchedRouteRecord  RouteRecord
						addDestinationErr   error
					)

					BeforeEach(func() {
						appGUID1 = generateGUID()
						appGUID2 = generateGUID()
						destinationMessages = []DestinationMessage{
							{
								AppGUID:     appGUID1,
								ProcessType: "web",
								Port:        8080,
								Protocol:    "http1",
							},
							{
								AppGUID:     appGUID2,
								ProcessType: "worker",
								Port:        9000,
								Protocol:    "http1",
							},
							// Duplicate dest that should be ignored
							{
								AppGUID:     appGUID2,
								ProcessType: "worker",
								Port:        9000,
								Protocol:    "http1",
							},
						}

						destinationListCreateMessage := initializeDestinationListMessage(route1GUID, space.Name, []DestinationRecord{
							{
								GUID:        routeDestination.GUID,
								AppGUID:     routeDestination.AppRef.Name,
								ProcessType: routeDestination.ProcessType,
								Port:        routeDestination.Port,
								Protocol:    routeDestination.Protocol,
							},
						}, destinationMessages)
						patchedRouteRecord, addDestinationErr = routeRepo.AddDestinationsToRoute(testCtx, authInfo, destinationListCreateMessage)
						Expect(addDestinationErr).NotTo(HaveOccurred())
					})

					It("adds the destinations to CFRoute successfully", func() {
						cfRouteLookupKey := types.NamespacedName{Name: route1GUID, Namespace: space.Name}
						createdCFRoute := new(networkingv1alpha1.CFRoute)
						Expect(k8sClient.Get(testCtx, cfRouteLookupKey, createdCFRoute)).To(Succeed())

						Expect(createdCFRoute.Spec.Destinations).To(ConsistOf(
							MatchAllFields(
								Fields{
									"GUID": Not(BeEmpty()),
									"Port": Equal(8080),
									"AppRef": Equal(corev1.LocalObjectReference{
										Name: appGUID1,
									}),
									"ProcessType": Equal("web"),
									"Protocol":    Equal("http1"),
								},
							),
							MatchAllFields(
								Fields{
									"GUID": Not(BeEmpty()),
									"Port": Equal(9000),
									"AppRef": Equal(corev1.LocalObjectReference{
										Name: appGUID2,
									}),
									"ProcessType": Equal("worker"),
									"Protocol":    Equal("http1"),
								},
							),
							MatchAllFields(
								Fields{
									"GUID": Equal(destinationGUID),
									"Port": Equal(8000),
									"AppRef": Equal(corev1.LocalObjectReference{
										Name: appGUID,
									}),
									"ProcessType": Equal("web"),
									"Protocol":    Equal("http1"),
								},
							),
						))
					})

					It("returns RouteRecord with new destinations", func() {
						Expect(patchedRouteRecord.Destinations).To(ConsistOf(
							MatchAllFields(
								Fields{
									"GUID":        Not(BeEmpty()),
									"Port":        Equal(8080),
									"AppGUID":     Equal(appGUID1),
									"ProcessType": Equal("web"),
									"Protocol":    Equal("http1"),
								},
							),
							MatchAllFields(
								Fields{
									"GUID":        Not(BeEmpty()),
									"Port":        Equal(9000),
									"AppGUID":     Equal(appGUID2),
									"ProcessType": Equal("worker"),
									"Protocol":    Equal("http1"),
								},
							),
							MatchAllFields(
								Fields{
									"GUID":        Equal(destinationGUID),
									"Port":        Equal(8000),
									"AppGUID":     Equal(appGUID),
									"ProcessType": Equal("web"),
									"Protocol":    Equal("http1"),
								},
							),
						))
					})
				})

				When("one of the destinations is already on the route", func() {
					var (
						appGUID2               string
						addDestinationMessages []DestinationMessage
						patchedRouteRecord     RouteRecord
					)

					BeforeEach(func() {
						appGUID2 = generateGUID()
						addDestinationMessages = []DestinationMessage{
							{
								AppGUID:     routeDestination.AppRef.Name,
								ProcessType: routeDestination.ProcessType,
								Port:        routeDestination.Port,
								Protocol:    routeDestination.Protocol,
							},
							{
								AppGUID:     appGUID2,
								ProcessType: "worker",
								Port:        9000,
								Protocol:    "http1",
							},
						}

						destinationListCreateMessage := initializeDestinationListMessage(route1GUID, space.Name, []DestinationRecord{
							{
								GUID:        routeDestination.GUID,
								AppGUID:     routeDestination.AppRef.Name,
								ProcessType: routeDestination.ProcessType,
								Port:        routeDestination.Port,
								Protocol:    routeDestination.Protocol,
							},
						}, addDestinationMessages)
						var err error
						patchedRouteRecord, err = routeRepo.AddDestinationsToRoute(testCtx, authInfo, destinationListCreateMessage)
						Expect(err).NotTo(HaveOccurred())
					})

					It("adds only the new destination to CFRoute successfully", func() {
						testCtx = context.Background()
						cfRouteLookupKey := types.NamespacedName{Name: route1GUID, Namespace: space.Name}
						createdCFRoute := new(networkingv1alpha1.CFRoute)
						Expect(k8sClient.Get(testCtx, cfRouteLookupKey, createdCFRoute)).To(Succeed())

						Expect(createdCFRoute.Spec.Destinations).To(ConsistOf(
							networkingv1alpha1.Destination{
								GUID:        routeDestination.GUID,
								AppRef:      corev1.LocalObjectReference{Name: routeDestination.AppRef.Name},
								ProcessType: routeDestination.ProcessType,
								Port:        routeDestination.Port,
								Protocol:    routeDestination.Protocol,
							},
							MatchAllFields(
								Fields{
									"GUID": Not(BeEmpty()),
									"Port": Equal(9000),
									"AppRef": Equal(corev1.LocalObjectReference{
										Name: appGUID2,
									}),
									"ProcessType": Equal("worker"),
									"Protocol":    Equal("http1"),
								},
							),
						))
					})

					It("returns RouteRecord with new destinations", func() {
						Expect(patchedRouteRecord.Destinations).To(ConsistOf(
							DestinationRecord{
								GUID:        routeDestination.GUID,
								AppGUID:     routeDestination.AppRef.Name,
								ProcessType: routeDestination.ProcessType,
								Port:        routeDestination.Port,
								Protocol:    routeDestination.Protocol,
							},
							MatchAllFields(
								Fields{
									"GUID":        Not(BeEmpty()),
									"Port":        Equal(9000),
									"AppGUID":     Equal(appGUID2),
									"ProcessType": Equal("worker"),
									"Protocol":    Equal("http1"),
								},
							),
						))
					})
				})
			})
		})
	})
})

func createRoute(guid, namespace, host, path, domainGUID, appGUID string) *networkingv1alpha1.CFRoute {
	toReturn := &networkingv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: namespace,
		},
		Spec: networkingv1alpha1.CFRouteSpec{
			Host:     host,
			Path:     path,
			Protocol: "http",
			DomainRef: corev1.ObjectReference{
				Name:      domainGUID,
				Namespace: rootNamespace,
			},
			Destinations: []networkingv1alpha1.Destination{
				{
					GUID: appGUID + "destination",
					Port: 8080,
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
					ProcessType: "web",
					Protocol:    "http1",
				},
			},
		},
	}
	Expect(
		k8sClient.Create(context.Background(), toReturn),
	).To(Succeed())
	return toReturn
}

func initializeRouteCR(routeHost, routePath, routeGUID, domainGUID, spaceGUID string) *networkingv1alpha1.CFRoute {
	return &networkingv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeGUID,
			Namespace: spaceGUID,
		},
		Spec: networkingv1alpha1.CFRouteSpec{
			Host: routeHost,
			Path: routePath,
			DomainRef: corev1.ObjectReference{
				Name:      domainGUID,
				Namespace: spaceGUID,
			},
		},
	}
}

func initializeDestinationListMessage(routeGUID string, spaceGUID string, existingDestinations []DestinationRecord, addDestinations []DestinationMessage) AddDestinationsToRouteMessage {
	return AddDestinationsToRouteMessage{
		RouteGUID:            routeGUID,
		SpaceGUID:            spaceGUID,
		ExistingDestinations: existingDestinations,
		NewDestinations:      addDestinations,
	}
}

func buildCreateRouteMessage(routeHost, routePath, domainGUID, spaceGUID, domainNamespace string) CreateRouteMessage {
	return CreateRouteMessage{
		Host:            routeHost,
		Path:            routePath,
		SpaceGUID:       spaceGUID,
		DomainGUID:      domainGUID,
		DomainNamespace: domainNamespace,
	}
}

func cleanupRoute(k8sClient client.Client, ctx context.Context, routeGUID, routeNamespace string) error {
	return k8sClient.Delete(ctx, &networkingv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeGUID,
			Namespace: routeNamespace,
		},
	})
}

func validateTimestamp(timestamp string, interval time.Duration) {
	recordCreatedTime, err := time.Parse(TimestampFormat, timestamp)
	Expect(err).NotTo(HaveOccurred(), "There was an error converting the createRouteRecord CreatedTime to string")
	Expect(recordCreatedTime).To(BeTemporally("~", time.Now(), interval))
}
