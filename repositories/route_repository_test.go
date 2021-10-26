package repositories_test

import (
	"context"
	"time"

	. "code.cloudfoundry.org/cf-k8s-api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("RouteRepository", func() {
	Describe("GetRoute", func() {
		var (
			testCtx    context.Context
			route1GUID string
			route2GUID string
			domainGUID string

			cfRoute1 *networkingv1alpha1.CFRoute
			cfRoute2 *networkingv1alpha1.CFRoute

			routeRepo  RouteRepo
			repoClient client.Client
		)

		BeforeEach(func() {
			testCtx = context.Background()
			route1GUID = generateGUID()
			route2GUID = generateGUID()
			domainGUID = generateGUID()

			ctx := context.Background()

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

			routeRepo = RouteRepo{}
			var err error
			repoClient, err = BuildCRClient(k8sConfig)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			ctx := context.Background()
			k8sClient.Delete(ctx, cfRoute1)
			k8sClient.Delete(ctx, cfRoute2)
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

				Expect(route.DomainRef.GUID).To(Equal(cfRoute1.Spec.DomainRef.Name))
			})
		})

		When("the CFRoute does exist", func() {
			It("returns an error", func() {
				_, err := routeRepo.FetchRoute(testCtx, repoClient, "non-existent-route-guid")
				Expect(err).To(MatchError("not found"))
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

			routeRepo = RouteRepo{}
			var err error
			repoClient, err = BuildCRClient(k8sConfig)
			Expect(err).ToNot(HaveOccurred())
		})

		When("multiple CFRoutes exist", func() {
			var (
				route1GUID string
				route2GUID string
				domainGUID string

				cfRoute1 *networkingv1alpha1.CFRoute
				cfRoute2 *networkingv1alpha1.CFRoute
			)

			BeforeEach(func() {
				route1GUID = generateGUID()
				route2GUID = generateGUID()
				domainGUID = generateGUID()

				ctx := context.Background()

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
			})

			It("eventually returns a list of routeRecords for each CFRoute CR", func() {
				var routeRecords []RouteRecord
				Eventually(func() int {
					routeRecords, _ = routeRepo.FetchRouteList(testCtx, repoClient)
					return len(routeRecords)
				}, timeCheckThreshold*time.Second).Should(Equal(2), "returned records count should equal number of created CRs")

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
						Expect(route.DomainRef.GUID).To(Equal(cfRoute1.Spec.DomainRef.Name))
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

				By("returning a routeRecord in the list that matches another of the created CRs", func() {
					var route RouteRecord
					var found bool
					for _, routeRecord := range routeRecords {
						if routeRecord.GUID == cfRoute2.Name {
							found = true
							route = routeRecord
							break
						}
					}
					Expect(found).To(BeTrue(), "could not find matching record")

					By("returning a record with metadata fields from the CFRoute CR", func() {
						Expect(route.GUID).To(Equal(cfRoute2.Name))
						Expect(route.Host).To(Equal(cfRoute2.Spec.Host))
						Expect(route.SpaceGUID).To(Equal(cfRoute2.Namespace))
					})

					By("returning a record with spec fields from the CFRoute CR", func() {
						Expect(route.Path).To(Equal(cfRoute2.Spec.Path))
						Expect(route.Protocol).To(Equal(string(cfRoute2.Spec.Protocol)))
						Expect(route.DomainRef.GUID).To(Equal(cfRoute2.Spec.DomainRef.Name))
					})

					By("returning a record with destinations that match the CFRoute CR", func() {
						Expect(len(route.Destinations)).To(Equal(len(cfRoute2.Spec.Destinations)), "Route Record Destinations returned was not the correct length")
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

		When("no CFRoutes exist", func() {
			It("returns an empty list and no error", func() {
				routeRecords, err := routeRepo.FetchRouteList(testCtx, repoClient)
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
			testDomainGUID string
			testRouteGUID  string
		)

		BeforeEach(func() {
			var err error
			client, err = BuildCRClient(k8sConfig)
			Expect(err).NotTo(HaveOccurred())

			routeRepo = RouteRepo{}

			testCtx = context.Background()
			testDomainGUID = generateGUID()
			testRouteGUID = generateGUID()
		})

		When("route does not already exist", func() {
			var (
				createdRouteRecord RouteRecord
				createdRouteErr    error
				beforeCreationTime time.Time
			)

			BeforeEach(func() {
				// Create a CFDomain
				cfDomain := &networkingv1alpha1.CFDomain{
					ObjectMeta: v1.ObjectMeta{
						Name: testDomainGUID,
					},
					Spec: networkingv1alpha1.CFDomainSpec{},
				}
				err := k8sClient.Create(context.Background(), cfDomain)
				Expect(err).NotTo(HaveOccurred())

				beforeCreationTime = time.Now().UTC().AddDate(0, 0, -1)

				routeRecord := initializeRouteRecord(testRouteHost, testRoutePath, testRouteGUID, testDomainGUID, testNamespace)
				createdRouteRecord, createdRouteErr = routeRepo.CreateRoute(testCtx, client, routeRecord)
				Expect(createdRouteErr).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				Expect(cleanupRoute(k8sClient, testCtx, testRouteGUID, testNamespace)).To(Succeed())
				Expect(cleanupDomain(k8sClient, testCtx, testDomainGUID)).To(Succeed())
			})

			It("should create a new CFRoute CR successfully", func() {
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

			It("should return an RouteRecord with matching GUID, spaceGUID, and host", func() {
				Expect(createdRouteRecord.GUID).To(Equal(testRouteGUID), "Route GUID in record did not match input")
				Expect(createdRouteRecord.Host).To(Equal(testRouteHost), "Route Host in record did not match input")
				Expect(createdRouteRecord.Path).To(Equal(testRoutePath), "Route Path in record did not match input")
				Expect(createdRouteRecord.SpaceGUID).To(Equal(testNamespace), "Route Space GUID in record did not match input")
				Expect(createdRouteRecord.DomainRef.GUID).To(Equal(testDomainGUID), "Route Domain GUID in record did not match input")
			})

			It("should return an RouteRecord with CreatedAt and UpdatedAt fields that make sense", func() {
				afterTestTime := time.Now().UTC().AddDate(0, 0, 1)

				recordCreatedTime, err := time.Parse(TimestampFormat, createdRouteRecord.CreatedAt)
				Expect(err).To(BeNil(), "There was an error converting the createRouteRecord CreatedTime to string")
				recordUpdatedTime, err := time.Parse(TimestampFormat, createdRouteRecord.UpdatedAt)
				Expect(err).To(BeNil(), "There was an error converting the createRouteRecord UpdatedTime to string")

				Expect(recordCreatedTime.After(beforeCreationTime)).To(BeTrue(), "record creation time was not after the expected creation time")
				Expect(recordCreatedTime.Before(afterTestTime)).To(BeTrue(), "record creation time was not before the expected testing time")

				Expect(recordUpdatedTime.After(beforeCreationTime)).To(BeTrue(), "record updated time was not after the expected creation time")
				Expect(recordUpdatedTime.Before(afterTestTime)).To(BeTrue(), "record updated time was not before the expected testing time")
			})
		})

		When("route creation fails", func() {
			It("should return an error", func() {
				routeRecord := RouteRecord{}
				_, err := routeRepo.CreateRoute(testCtx, client, routeRecord)
				Expect(err).To(MatchError("an empty namespace may not be set during creation"))
			})
		})
	})
})

func initializeRouteCR(routeHost, routePath, routeGUID, domainGUID, spaceGUID string) networkingv1alpha1.CFRoute {
	return networkingv1alpha1.CFRoute{
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

func initializeRouteRecord(routeHost, routePath, routeGUID, domainGUID, spaceGUID string) RouteRecord {
	return RouteRecord{
		GUID:      routeGUID,
		Host:      routeHost,
		Path:      routePath,
		SpaceGUID: spaceGUID,
		DomainRef: DomainRecord{
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
