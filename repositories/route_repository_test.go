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
		var testCtx context.Context

		BeforeEach(func() {
			testCtx = context.Background()
		})

		When("multiple CFRoute resources exist", func() {
			var (
				cfRoute1 *networkingv1alpha1.CFRoute
				cfRoute2 *networkingv1alpha1.CFRoute
			)

			BeforeEach(func() {
				ctx := context.Background()

				cfRoute1 = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "route-id-1",
						Namespace: "default",
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-1",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.LocalObjectReference{
							Name: "my-domain",
						},
						Destinations: []networkingv1alpha1.Destination{},
					},
				}
				Expect(k8sClient.Create(ctx, cfRoute1)).To(Succeed())

				cfRoute2 = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "route-id-2",
						Namespace: "default",
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "my-subdomain-2",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.LocalObjectReference{
							Name: "my-domain",
						},
						Destinations: []networkingv1alpha1.Destination{},
					},
				}
				Expect(k8sClient.Create(ctx, cfRoute2)).To(Succeed())
			})

			It("fetches the CFRoute CR we're looking for", func() {
				routeRepo := RouteRepo{}
				client, err := BuildCRClient(k8sConfig)
				Expect(err).ToNot(HaveOccurred())

				route := RouteRecord{}
				route, err = routeRepo.FetchRoute(testCtx, client, "route-id-1")
				Expect(err).ToNot(HaveOccurred())

				Expect(route.GUID).To(Equal("route-id-1"))
				Expect(route.Host).To(Equal("my-subdomain-1"))
				Expect(route.SpaceGUID).To(Equal("default"))
				Expect(route.Path).To(Equal(""))
				Expect(route.Protocol).To(Equal("http"))
				Expect(route.Destinations).To(Equal([]Destination{}))
				Expect(route.DomainRef.GUID).To(Equal("my-domain"))
			})

			AfterEach(func() {
				ctx := context.Background()
				Expect(k8sClient.Delete(ctx, cfRoute1)).To(Succeed())
				Expect(k8sClient.Delete(ctx, cfRoute2)).To(Succeed())
			})
		})

		When("no CFRoute exists", func() {
			It("returns an error", func() {
				routeRepo := RouteRepo{}
				client, err := BuildCRClient(k8sConfig)
				Expect(err).ToNot(HaveOccurred())

				_, err = routeRepo.FetchRoute(testCtx, client, "non-existent-route-guid")
				Expect(err).To(MatchError("not found"))
			})
		})

		When("multiple CFRoute resources exist across namespaces with the same name", func() {
			var (
				cfRoute1            *networkingv1alpha1.CFRoute
				cfRoute2            *networkingv1alpha1.CFRoute
				nonDefaultNamespace *corev1.Namespace
			)

			BeforeEach(func() {
				ctx := context.Background()
				// Create second namespace aside from default within which to create a duplicate route
				nonDefaultNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "non-default-namespace"}}
				Expect(k8sClient.Create(ctx, nonDefaultNamespace)).To(Succeed())

				cfRoute1 = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "non-unique-route-id",
						Namespace: "default",
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.LocalObjectReference{
							Name: "my-domain",
						},
						Destinations: []networkingv1alpha1.Destination{},
					},
				}
				Expect(k8sClient.Create(ctx, cfRoute1)).To(Succeed())

				cfRoute2 = &networkingv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "non-unique-route-id",
						Namespace: "non-default-namespace",
					},
					Spec: networkingv1alpha1.CFRouteSpec{
						Host:     "",
						Path:     "",
						Protocol: "http",
						DomainRef: corev1.LocalObjectReference{
							Name: "my-domain",
						},
						Destinations: []networkingv1alpha1.Destination{},
					},
				}
				Expect(k8sClient.Create(ctx, cfRoute2)).To(Succeed())
			})

			It("returns an error", func() {
				routeRepo := RouteRepo{}
				client, err := BuildCRClient(k8sConfig)
				Expect(err).ToNot(HaveOccurred())

				// Looks like we can continue doing state-based setup for the time being
				// Assumption: when unit testing, we can ignore webhooks that might turn the uniqueness constraint into a race condition
				// If assumption is invalidated, we can implement the setup by mocking a fake client to return the non-unique ids

				_, err = routeRepo.FetchRoute(testCtx, client, "non-unique-route-id")
				Expect(err).To(MatchError("duplicate route GUID exists"))
			})

			AfterEach(func() {
				afterCtx := context.Background()
				Expect(k8sClient.Delete(afterCtx, cfRoute1)).To(Succeed())
				Expect(k8sClient.Delete(afterCtx, cfRoute2)).To(Succeed())
				Expect(k8sClient.Delete(afterCtx, nonDefaultNamespace)).To(Succeed())
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
