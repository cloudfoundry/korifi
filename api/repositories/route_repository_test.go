package repositories_test

import (
	"context"
	"fmt"
	"time"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("RouteRepository", func() {
	Describe("GetRoute", func() {
		const domainName = "my-domain-name"
		var (
			testCtx    context.Context
			route1GUID string
			route2GUID string
			domainGUID string

			cfRoute1 *networkingv1alpha1.CFRoute
			cfRoute2 *networkingv1alpha1.CFRoute
			cfDomain *networkingv1alpha1.CFDomain

			routeRepo  RouteRepo
			repoClient client.Client
		)

		BeforeEach(func() {
			testCtx = context.Background()
			route1GUID = generateGUID()
			route2GUID = generateGUID()
			domainGUID = generateGUID()

			ctx := context.Background()

			cfDomain = &networkingv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name: domainGUID,
				},
				Spec: networkingv1alpha1.CFDomainSpec{
					Name: domainName,
				},
			}
			Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())

			cfRoute1 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      route1GUID,
					Namespace: "default",
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: domainGUID,
					},
					Destinations: []networkingv1alpha1.Destination{
						{
							GUID: "destination-guid",
							Port: 8080,
							AppRef: corev1.LocalObjectReference{
								Name: "some-app-guid",
							},
							ProcessType: "web",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute1)).To(Succeed())

			cfRoute2 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      route2GUID,
					Namespace: "default",
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-2",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: domainGUID,
					},
					Destinations: []networkingv1alpha1.Destination{},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute2)).To(Succeed())

			var err error
			repoClient, err = BuildCRClient(k8sConfig)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			ctx := context.Background()
			k8sClient.Delete(ctx, cfRoute1)
			k8sClient.Delete(ctx, cfRoute2)
			k8sClient.Delete(ctx, cfDomain)
		})

		When("multiple CFRoute resources exist", func() {
			It("fetches the CFRoute CR we're looking for", func() {
				route, err := routeRepo.FetchRoute(testCtx, repoClient, route1GUID)
				Expect(err).ToNot(HaveOccurred())

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
				})

				By("returning a record where the CreatedAt and UpdatedAt match the CR creation time", func() {
					createdAt, err := time.Parse(time.RFC3339, route.CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

					updatedAt, err := time.Parse(time.RFC3339, route.CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
				})

				Expect(route.Domain).To(Equal(DomainRecord{GUID: domainGUID}))
			})
		})

		When("the CFRoute doesn't exist", func() {
			It("returns an error", func() {
				_, err := routeRepo.FetchRoute(testCtx, repoClient, "non-existent-route-guid")
				Expect(err).To(MatchError(NotFoundError{}))
			})
		})

		When("multiple CFRoute resources exist across namespaces with the same name", func() {
			var (
				otherNamespaceGUID string
				otherNamespace     *corev1.Namespace

				cfRoute1A *networkingv1alpha1.CFRoute
			)

			BeforeEach(func() {
				ctx := context.Background()
				// Create second namespace aside from default within which to create a duplicate route
				otherNamespaceGUID = generateGUID()
				otherNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: otherNamespaceGUID}}
				Expect(k8sClient.Create(ctx, otherNamespace)).To(Succeed())

				cfRoute1A = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      route1GUID,
						Namespace: otherNamespaceGUID,
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-1",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.LocalObjectReference{
							Name: domainGUID,
						},
					},
				}
				Expect(k8sClient.Create(ctx, cfRoute1A)).To(Succeed())
			})

			AfterEach(func() {
				afterCtx := context.Background()
				k8sClient.Delete(afterCtx, cfRoute1A)
				k8sClient.Delete(afterCtx, otherNamespace)
			})

			It("returns an error", func() {
				// Looks like we can continue doing state-based setup for the time being
				// Assumption: when unit testing, we can ignore webhooks that might turn the uniqueness constraint into a race condition
				// If assumption is invalidated, we can implement the setup by mocking a fake client to return the non-unique ids

				_, err := routeRepo.FetchRoute(testCtx, repoClient, route1GUID)
				Expect(err).To(MatchError("duplicate route GUID exists"))
			})
		})
	})

	Describe("GetRouteList", func() {
		var (
			testCtx context.Context

			routeRepo  RouteRepo
			repoClient client.Client
		)

		BeforeEach(func() {
			testCtx = context.Background()

			var err error
			repoClient, err = BuildCRClient(k8sConfig)
			Expect(err).ToNot(HaveOccurred())
		})

		When("multiple CFRoutes exist", func() {
			const domainName = "my-domain-name"
			var (
				route1GUID string
				route2GUID string
				domainGUID string

				cfRoute1 *networkingv1alpha1.CFRoute
				cfRoute2 *networkingv1alpha1.CFRoute
				cfDomain *networkingv1alpha1.CFDomain
			)

			BeforeEach(func() {
				route1GUID = generateGUID()
				route2GUID = generateGUID()
				domainGUID = generateGUID()

				ctx := context.Background()

				cfDomain = &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name: domainGUID,
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: domainName,
					},
				}
				Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())

				cfRoute1 = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      route1GUID,
						Namespace: "default",
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-1",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.LocalObjectReference{
							Name: domainGUID,
						},
						Destinations: []networkingv1alpha1.Destination{
							{
								GUID: "destination-guid",
								Port: 8080,
								AppRef: corev1.LocalObjectReference{
									Name: "some-app-guid",
								},
								ProcessType: "web",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, cfRoute1)).To(Succeed())

				cfRoute2 = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      route2GUID,
						Namespace: "default",
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-2",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.LocalObjectReference{
							Name: domainGUID,
						},
						Destinations: []networkingv1alpha1.Destination{},
					},
				}
				Expect(k8sClient.Create(ctx, cfRoute2)).To(Succeed())
			})

			AfterEach(func() {
				ctx := context.Background()
				k8sClient.Delete(ctx, cfRoute1)
				k8sClient.Delete(ctx, cfRoute2)
				k8sClient.Delete(ctx, cfDomain)
			})

			It("eventually returns a list of routeRecords for each CFRoute CR", func() {
				var routeRecords []RouteRecord
				Eventually(func() []RouteRecord {
					routeRecords, _ = routeRepo.FetchRouteList(testCtx, repoClient)
					return routeRecords
				}, timeCheckThreshold*time.Second).Should(HaveLen(2), "returned records count should equal number of created CRs")

				var route1, route2 RouteRecord
				for _, routeRecord := range routeRecords {
					switch routeRecord.GUID {
					case cfRoute1.Name:
						route1 = routeRecord
					case cfRoute2.Name:
						route2 = routeRecord
					default:
						Fail(fmt.Sprintf("Unknown routeRecord: %v", routeRecord))
					}
				}

				Expect(route1).NotTo(BeZero())
				Expect(route2).NotTo(BeZero())

				By("returning a routeRecord in the list for one of the created CRs", func() {
					Expect(route1.GUID).To(Equal(cfRoute1.Name))
					Expect(route1.Host).To(Equal(cfRoute1.Spec.Host))
					Expect(route1.SpaceGUID).To(Equal(cfRoute1.Namespace))
					Expect(route1.Path).To(Equal(cfRoute1.Spec.Path))
					Expect(route1.Protocol).To(Equal(string(cfRoute1.Spec.Protocol)))
					Expect(route1.Domain).To(Equal(DomainRecord{GUID: domainGUID}))

					Expect(route1.Destinations).To(Equal([]DestinationRecord{
						{
							GUID:        cfRoute1.Spec.Destinations[0].GUID,
							AppGUID:     cfRoute1.Spec.Destinations[0].AppRef.Name,
							Port:        cfRoute1.Spec.Destinations[0].Port,
							ProcessType: cfRoute1.Spec.Destinations[0].ProcessType,
						},
					}))

					createdAt, err := time.Parse(time.RFC3339, route1.CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

					updatedAt, err := time.Parse(time.RFC3339, route1.CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
				})

				By("returning a routeRecord in the list that matches another of the created CRs", func() {
					Expect(route2.GUID).To(Equal(cfRoute2.Name))
					Expect(route2.Host).To(Equal(cfRoute2.Spec.Host))
					Expect(route2.SpaceGUID).To(Equal(cfRoute2.Namespace))
					Expect(route2.Path).To(Equal(cfRoute2.Spec.Path))
					Expect(route2.Protocol).To(Equal(string(cfRoute2.Spec.Protocol)))
					Expect(route2.Domain).To(Equal(DomainRecord{GUID: domainGUID}))

					Expect(route2.Destinations).To(BeEmpty())

					createdAt, err := time.Parse(time.RFC3339, route2.CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

					updatedAt, err := time.Parse(time.RFC3339, route2.CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
				})
			})
		})

		When("no CFRoutes exist", func() {
			It("returns an empty list and no error", func() {
				routeRecords, err := routeRepo.FetchRouteList(testCtx, repoClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(routeRecords).To(BeEmpty())
			})
		})
	})

	Describe("GetRoutesForApp", func() {
		const (
			testNamespace = "default"
		)

		var (
			testCtx context.Context

			routeRepo  RouteRepo
			repoClient client.Client

			appGUID    string
			route1GUID string
			route2GUID string
			domainGUID string

			cfRoute1 *networkingv1alpha1.CFRoute
			cfRoute2 *networkingv1alpha1.CFRoute
		)

		BeforeEach(func() {
			testCtx = context.Background()

			var err error
			repoClient, err = BuildCRClient(k8sConfig)
			Expect(err).ToNot(HaveOccurred())

			appGUID = generateGUID()
			route1GUID = generateGUID()
			route2GUID = generateGUID()
			domainGUID = generateGUID()

			ctx := context.Background()

			cfRoute1 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      route1GUID,
					Namespace: testNamespace,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-1",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: domainGUID,
					},
					Destinations: []networkingv1alpha1.Destination{
						{
							GUID: "destination-guid",
							Port: 8080,
							AppRef: corev1.LocalObjectReference{
								Name: appGUID,
							},
							ProcessType: "web",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute1)).To(Succeed())

			cfRoute2 = &networkingv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      route2GUID,
					Namespace: testNamespace,
				},
				Spec: networkingv1alpha1.CFRouteSpec{
					Host:     "my-subdomain-2",
					Path:     "",
					Protocol: "http",
					DomainRef: corev1.LocalObjectReference{
						Name: domainGUID,
					},
					Destinations: []networkingv1alpha1.Destination{},
				},
			}
			Expect(k8sClient.Create(ctx, cfRoute2)).To(Succeed())
		})

		AfterEach(func() {
			ctx := context.Background()
			k8sClient.Delete(ctx, cfRoute1)
			k8sClient.Delete(ctx, cfRoute2)
		})

		When("multiple CFRoutes exist", func() {
			It("eventually returns a list of routeRecords for each CFRoute CR", func() {
				var routeRecords []RouteRecord
				Eventually(func() int {
					routeRecords, _ = routeRepo.FetchRoutesForApp(testCtx, repoClient, appGUID, testNamespace)
					return len(routeRecords)
				}, timeCheckThreshold*time.Second).Should(Equal(1), "returned records count should equal number of created CRs with destinations to the App")

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
					})

					By("returning a record where the CreatedAt and UpdatedAt match the CR creation time", func() {
						createdAt, err := time.Parse(time.RFC3339, route.CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

						updatedAt, err := time.Parse(time.RFC3339, route.CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
					})
				})
			})
		})

		When("no CFRoutes exist for the app", func() {
			It("returns an empty list and no error", func() {
				routeRecords, err := routeRepo.FetchRoutesForApp(testCtx, repoClient, "i-dont-exist", testNamespace)
				Expect(err).ToNot(HaveOccurred())
				Expect(routeRecords).To(BeEmpty())
			})
		})
	})

	Describe("CreateRoute", func() {
		const (
			testNamespace = "default"
			testRouteHost = "test-route-host"
			testRoutePath = "/test/route/path"
		)

		var (
			client         client.Client
			routeRepo      RouteRepo
			testCtx        context.Context
			cfDomain       *networkingv1alpha1.CFDomain
			testDomainGUID string
			testRouteGUID  string
		)

		BeforeEach(func() {
			var err error
			client, err = BuildCRClient(k8sConfig)
			Expect(err).NotTo(HaveOccurred())

			testCtx = context.Background()
			testDomainGUID = generateGUID()
			testRouteGUID = generateGUID()
		})

		When("route does not already exist", func() {
			const domainName = "my-domain-name"
			var (
				createdRouteRecord RouteRecord
				createdRouteErr    error
			)

			BeforeEach(func() {
				cfDomain = &networkingv1alpha1.CFDomain{
					ObjectMeta: v1.ObjectMeta{
						Name: testDomainGUID,
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: domainName,
					},
				}
				err := k8sClient.Create(context.Background(), cfDomain)
				Expect(err).NotTo(HaveOccurred())

				routeRecord := initializeRouteRecord(testRouteHost, testRoutePath, testRouteGUID, testDomainGUID, testNamespace)
				createdRouteRecord, createdRouteErr = routeRepo.CreateRoute(testCtx, client, routeRecord)
				Expect(createdRouteErr).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				Expect(cleanupRoute(k8sClient, testCtx, testRouteGUID, testNamespace)).To(Succeed())
				Expect(cleanupDomain(k8sClient, testCtx, testDomainGUID)).To(Succeed())
			})

			It("creates a new CFRoute CR successfully", func() {
				cfRouteLookupKey := types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}
				createdCFRoute := new(networkingv1alpha1.CFRoute)
				Eventually(func() string {
					err := k8sClient.Get(context.Background(), cfRouteLookupKey, createdCFRoute)
					if err != nil {
						return ""
					}
					return createdCFRoute.Name
				}, 10*time.Second, 250*time.Millisecond).Should(Equal(testRouteGUID))
			})

			It("returns an RouteRecord with matching fields", func() {
				Expect(createdRouteRecord.GUID).To(Equal(testRouteGUID), "Route GUID in record did not match input")
				Expect(createdRouteRecord.Host).To(Equal(testRouteHost), "Route Host in record did not match input")
				Expect(createdRouteRecord.Path).To(Equal(testRoutePath), "Route Path in record did not match input")
				Expect(createdRouteRecord.SpaceGUID).To(Equal(testNamespace), "Route Space GUID in record did not match input")
				Expect(createdRouteRecord.Domain).To(Equal(DomainRecord{GUID: testDomainGUID}), "Route Domain in record did not match created domain")

				recordCreatedTime, err := time.Parse(TimestampFormat, createdRouteRecord.CreatedAt)
				Expect(err).NotTo(HaveOccurred(), "There was an error converting the createRouteRecord CreatedTime to string")
				recordUpdatedTime, err := time.Parse(TimestampFormat, createdRouteRecord.UpdatedAt)
				Expect(err).NotTo(HaveOccurred(), "There was an error converting the createRouteRecord UpdatedTime to string")

				Expect(recordCreatedTime).To(BeTemporally("~", time.Now(), 2*time.Second))
				Expect(recordUpdatedTime).To(BeTemporally("~", time.Now(), 2*time.Second))
			})
		})

		When("route creation fails", func() {
			When("namespace doesn't exist", func() {
				It("returns an error", func() {
					routeRecord := RouteRecord{}
					_, err := routeRepo.CreateRoute(testCtx, client, routeRecord)
					Expect(err).To(MatchError("an empty namespace may not be set during creation"))
				})
			})
		})
	})

	Describe("AddDestinationsToRoute", func() {
		const (
			testRouteHost = "test-route-host"
			testRoutePath = "/test/route/path"
		)

		var (
			testDomainGUID string
			testRouteGUID  string
			testNamespace  string
			client         client.Client
			routeRepo      RouteRepo
			testCtx        context.Context
			newNamespace   *corev1.Namespace
		)

		BeforeEach(func() {
			var err error
			client, err = BuildCRClient(k8sConfig)
			Expect(err).NotTo(HaveOccurred())

			testCtx = context.Background()
			testDomainGUID = generateGUID()
			testRouteGUID = generateGUID()
			testNamespace = generateGUID()
		})

		When("the route exists with no destinations", func() {
			BeforeEach(func() {
				beforeCtx := context.Background()

				newNamespace = &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}
				Expect(k8sClient.Create(beforeCtx, newNamespace)).To(Succeed())

				cfDomain := &networkingv1alpha1.CFDomain{
					ObjectMeta: v1.ObjectMeta{
						Name: testDomainGUID,
					},
					Spec: networkingv1alpha1.CFDomainSpec{},
				}
				Expect(k8sClient.Create(beforeCtx, cfDomain)).To(Succeed())

				cfRoute := initializeRouteCR(testRouteHost, testRoutePath, testRouteGUID, testDomainGUID, testNamespace)
				Expect(k8sClient.Create(beforeCtx, cfRoute)).To(Succeed())
			})

			AfterEach(func() {
				Expect(cleanupRoute(k8sClient, testCtx, testRouteGUID, testNamespace)).To(Succeed())
				Expect(cleanupDomain(k8sClient, testCtx, testDomainGUID)).To(Succeed())
			})

			When("route is updated to add new destinations", func() {
				var (
					destinationGUID1   string
					destinationGUID2   string
					appGUID1           string
					appGUID2           string
					destionationRecord []DestinationRecord
					patchedRouteRecord RouteRecord
					addDestinationErr  error
				)
				BeforeEach(func() {
					beforeCtx := context.Background()
					destinationGUID1 = generateGUID()
					destinationGUID2 = generateGUID()
					appGUID1 = generateGUID()
					appGUID2 = generateGUID()
					destionationRecord = []DestinationRecord{
						{
							GUID:        destinationGUID1,
							AppGUID:     appGUID1,
							ProcessType: "web",
							Port:        8080,
						},
						{
							GUID:        destinationGUID2,
							AppGUID:     appGUID2,
							ProcessType: "worker",
							Port:        9000,
						},
					}

					routeRecord, err := routeRepo.FetchRoute(beforeCtx, client, testRouteGUID)
					Expect(err).NotTo(HaveOccurred())

					// initialize a DestinationListMessage
					destinationListCreateMessage := initializeDestinationListMessage(routeRecord, destionationRecord)
					patchedRouteRecord, addDestinationErr = routeRepo.AddDestinationsToRoute(beforeCtx, client, destinationListCreateMessage)
					Expect(addDestinationErr).NotTo(HaveOccurred())
				})

				It("adds the destinations to CFRoute successfully", func() {
					testCtx = context.Background()
					cfRouteLookupKey := types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}
					createdCFRoute := new(networkingv1alpha1.CFRoute)
					Eventually(func() []networkingv1alpha1.Destination {
						err := k8sClient.Get(testCtx, cfRouteLookupKey, createdCFRoute)
						if err != nil {
							return nil
						}
						return createdCFRoute.Spec.Destinations
					}, 5*time.Second).Should(HaveLen(2), "could not retrieve cfRoute having exactly 2 destinations")

					Expect(createdCFRoute.Spec.Destinations).To(ConsistOf([]networkingv1alpha1.Destination{
						{
							GUID: destinationGUID1,
							Port: 8080,
							AppRef: corev1.LocalObjectReference{
								Name: appGUID1,
							},
							ProcessType: "web",
						},
						{
							GUID: destinationGUID2,
							Port: 9000,
							AppRef: corev1.LocalObjectReference{
								Name: appGUID2,
							},
							ProcessType: "worker",
						},
					}))
				})

				It("returns RouteRecord with new destinations", func() {
					Expect(patchedRouteRecord.Destinations).To(ConsistOf(destionationRecord))
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
				beforeCtx := context.Background()

				newNamespace = &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}
				Expect(k8sClient.Create(beforeCtx, newNamespace)).To(Succeed())

				cfDomain := &networkingv1alpha1.CFDomain{
					ObjectMeta: v1.ObjectMeta{
						Name: testDomainGUID,
					},
					Spec: networkingv1alpha1.CFDomainSpec{},
				}
				Expect(k8sClient.Create(beforeCtx, cfDomain)).To(Succeed())

				cfRoute := initializeRouteCR(testRouteHost, testRoutePath, testRouteGUID, testDomainGUID, testNamespace)

				destinationGUID = generateGUID()
				appGUID = generateGUID()
				routeDestination = networkingv1alpha1.Destination{
					GUID: destinationGUID,
					Port: 8000,
					AppRef: corev1.LocalObjectReference{
						Name: appGUID,
					},
					ProcessType: "web",
				}

				cfRoute.Spec.Destinations = []networkingv1alpha1.Destination{routeDestination}
				Expect(k8sClient.Create(beforeCtx, cfRoute)).To(Succeed())
			})

			AfterEach(func() {
				Expect(cleanupRoute(k8sClient, testCtx, testRouteGUID, testNamespace)).To(Succeed())
				Expect(cleanupDomain(k8sClient, testCtx, testDomainGUID)).To(Succeed())
			})

			When("route is updated to append new destinations", func() {
				var (
					destinationGUID1   string
					destinationGUID2   string
					appGUID1           string
					appGUID2           string
					destinationRecord  []DestinationRecord
					patchedRouteRecord RouteRecord
					addDestinationErr  error
				)

				BeforeEach(func() {
					beforeCtx := context.Background()
					destinationGUID1 = generateGUID()
					destinationGUID2 = generateGUID()
					appGUID1 = generateGUID()
					appGUID2 = generateGUID()
					destinationRecord = []DestinationRecord{
						{
							GUID:        destinationGUID1,
							AppGUID:     appGUID1,
							ProcessType: "web",
							Port:        8080,
						},
						{
							GUID:        destinationGUID2,
							AppGUID:     appGUID2,
							ProcessType: "worker",
							Port:        9000,
						},
					}

					routeRecord, err := routeRepo.FetchRoute(beforeCtx, client, testRouteGUID)
					Expect(err).NotTo(HaveOccurred())

					destinationListCreateMessage := initializeDestinationListMessage(routeRecord, destinationRecord)
					patchedRouteRecord, addDestinationErr = routeRepo.AddDestinationsToRoute(beforeCtx, client, destinationListCreateMessage)
					Expect(addDestinationErr).NotTo(HaveOccurred())
				})

				It("adds the destinations to CFRoute successfully", func() {
					testCtx = context.Background()
					cfRouteLookupKey := types.NamespacedName{Name: testRouteGUID, Namespace: testNamespace}
					createdCFRoute := new(networkingv1alpha1.CFRoute)
					Eventually(func() []networkingv1alpha1.Destination {
						err := k8sClient.Get(testCtx, cfRouteLookupKey, createdCFRoute)
						if err != nil {
							return nil
						}
						return createdCFRoute.Spec.Destinations
					}, 5*time.Second).Should(HaveLen(3), "could not retrieve cfRoute having exactly 3 destinations")

					Expect(createdCFRoute.Spec.Destinations).To(ConsistOf([]networkingv1alpha1.Destination{
						{
							GUID: destinationGUID1,
							Port: 8080,
							AppRef: corev1.LocalObjectReference{
								Name: appGUID1,
							},
							ProcessType: "web",
						},
						{
							GUID: destinationGUID2,
							Port: 9000,
							AppRef: corev1.LocalObjectReference{
								Name: appGUID2,
							},
							ProcessType: "worker",
						},
						{
							GUID: destinationGUID,
							Port: 8000,
							AppRef: corev1.LocalObjectReference{
								Name: appGUID,
							},
							ProcessType: "web",
						},
					}))
				})

				It("returns RouteRecord with new destinations", func() {
					Expect(patchedRouteRecord.Destinations).To(ConsistOf(append(destinationRecord, DestinationRecord{
						GUID:        destinationGUID,
						AppGUID:     appGUID,
						ProcessType: "web",
						Port:        8000,
					})))
				})
			})
		})
	})
})

func initializeRouteCR(routeHost, routePath, routeGUID, domainGUID, spaceGUID string) *networkingv1alpha1.CFRoute {
	return &networkingv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeGUID,
			Namespace: spaceGUID,
		},
		Spec: networkingv1alpha1.CFRouteSpec{
			Host: routeHost,
			Path: routePath,
			DomainRef: corev1.LocalObjectReference{
				Name: domainGUID,
			},
		},
	}
}

func initializeDestinationListMessage(routeRecord RouteRecord, destinationRecords []DestinationRecord) RouteAddDestinationsMessage {
	return RouteAddDestinationsMessage{
		Route:        routeRecord,
		Destinations: destinationRecords,
	}
}

func initializeRouteRecord(routeHost, routePath, routeGUID, domainGUID, spaceGUID string) RouteRecord {
	return RouteRecord{
		GUID:      routeGUID,
		Host:      routeHost,
		Path:      routePath,
		SpaceGUID: spaceGUID,
		Domain: DomainRecord{
			GUID: domainGUID,
		},
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

func cleanupDomain(k8sClient client.Client, ctx context.Context, domainGUID string) error {
	return k8sClient.Delete(ctx, &networkingv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name: domainGUID,
		},
	})
}
