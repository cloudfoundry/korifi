package osbapi_test

import (
	"encoding/base64"
	"net/http"
	"strconv"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/brokers/osbapi"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/broker"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("OSBAPI Client", func() {
	var (
		testNamespace string
		brokerClient  *osbapi.Client
		serviceBroker *korifiv1alpha1.CFServiceBroker
		brokerServer  *broker.BrokerServer
	)

	BeforeEach(func() {
		testNamespace = uuid.NewString()
		Expect(adminClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		})).To(Succeed())

		brokerServer = broker.NewServer()

		brokerClient = osbapi.NewClient(adminClient, true)

		creds := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			Data: map[string][]byte{
				korifiv1alpha1.CredentialsSecretKey: []byte(`{"username":"broker-user","password":"broker-password"}`),
			},
		}
		helpers.EnsureCreate(adminClient, creds)

		serviceBroker = &korifiv1alpha1.CFServiceBroker{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			Spec: korifiv1alpha1.CFServiceBrokerSpec{
				ServiceBroker: services.ServiceBroker{
					Name: uuid.NewString(),
				},
				Credentials: corev1.LocalObjectReference{Name: creds.Name},
			},
		}
		helpers.EnsureCreate(adminClient, serviceBroker)
	})

	JustBeforeEach(func() {
		brokerServer.Start()
		DeferCleanup(func() {
			brokerServer.Stop()
		})

		helpers.EnsurePatch(adminClient, serviceBroker, func(b *korifiv1alpha1.CFServiceBroker) {
			b.Spec.URL = brokerServer.URL()
		})
	})

	Describe("GetCatalog", func() {
		var (
			catalog       *osbapi.Catalog
			getCatalogErr error
		)

		BeforeEach(func() {
			brokerServer.WithCatalog(&osbapi.Catalog{
				Services: []osbapi.Service{{
					ID:          "123456",
					Name:        "test-service",
					Description: "test service description",
					BrokerCatalogFeatures: services.BrokerCatalogFeatures{
						Bindable: true,
					},
				}},
			})
		})

		JustBeforeEach(func() {
			catalog, getCatalogErr = brokerClient.GetCatalog(ctx, serviceBroker)
		})

		It("gets the catalog", func() {
			Expect(getCatalogErr).NotTo(HaveOccurred())
			Expect(catalog).To(PointTo(Equal(osbapi.Catalog{
				Services: []osbapi.Service{{
					ID:          "123456",
					Name:        "test-service",
					Description: "test service description",
					BrokerCatalogFeatures: services.BrokerCatalogFeatures{
						Bindable: true,
					},
				}},
			})))
		})

		It("sends broker credentials in the Authorization request header", func() {
			Expect(brokerServer.ServedRequests()).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Header": HaveKeyWithValue(
					"Authorization", ConsistOf("Basic "+base64.StdEncoding.EncodeToString([]byte("broker-user:broker-password"))),
				),
			}))))
		})

		It("sends OSBAPI version request header", func() {
			Expect(brokerServer.ServedRequests()).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Header": HaveKeyWithValue(
					"X-Broker-Api-Version", ConsistOf("2.17"),
				),
			}))))
		})

		When("the client does not trust insecure brokers", func() {
			BeforeEach(func() {
				brokerClient = osbapi.NewClient(adminClient, false)
			})

			It("returns an error", func() {
				Expect(getCatalogErr).To(MatchError(ContainSubstring("failed to verify certificate")))
			})
		})

		When("getting the catalog fails", func() {
			BeforeEach(func() {
				brokerServer = broker.NewServer().WithHandler("/v2/catalog", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusTeapot)
				}))
			})

			It("returns an error", func() {
				Expect(getCatalogErr).To(MatchError(ContainSubstring(strconv.Itoa(http.StatusTeapot))))
			})
		})

		When("the catalog response cannot be unmarshalled", func() {
			BeforeEach(func() {
				brokerServer = broker.NewServer().WithHandler("/v2/catalog", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte("hello"))
				}))
			})

			It("returns an error", func() {
				Expect(getCatalogErr).To(MatchError(ContainSubstring("failed to unmarshal catalog")))
			})
		})

		When("broker credentials secret does not exist", func() {
			BeforeEach(func() {
				helpers.EnsurePatch(adminClient, serviceBroker, func(b *korifiv1alpha1.CFServiceBroker) {
					b.Spec.Credentials.Name = "i-do-not-exist"
				})
			})

			It("returns an error", func() {
				Expect(getCatalogErr).To(MatchError(ContainSubstring("failed to get credentials secret")))
			})
		})

		When("broker credentials secret is invalid", func() {
			BeforeEach(func() {
				creds := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      serviceBroker.Spec.Credentials.Name,
					},
				}
				Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(creds), creds)).To(Succeed())
				helpers.EnsurePatch(adminClient, creds, func(s *corev1.Secret) {
					s.Data = map[string][]byte{"foo": []byte("bar")}
				})
			})

			It("returns an error", func() {
				Expect(getCatalogErr).To(MatchError(ContainSubstring("data of secret")))
			})
		})
	})
})
