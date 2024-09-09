package osbapi_test

import (
	"net/http"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tests/helpers/broker"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ClientFactory", func() {
	var (
		brokerServer    *broker.BrokerServer
		cfServiceBroker *korifiv1alpha1.CFServiceBroker
		factory         *osbapi.ClientFactory
		osbapiClient    osbapi.BrokerClient
		createClientErr error
	)

	BeforeEach(func() {
		brokerServer = broker.NewServer().WithResponse(
			"/v2/catalog",
			map[string]any{
				"services": []map[string]any{
					{
						"name": "test-service",
					},
				},
			},
			http.StatusOK,
		).Start()

		credentialsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      uuid.NewString(),
			},
			Data: map[string][]byte{
				tools.CredentialsSecretKey: []byte(`{"username": "broker-user", "password": "broker-password"}`),
			},
		}
		Expect(adminClient.Create(ctx, credentialsSecret)).To(Succeed())

		cfServiceBroker = &korifiv1alpha1.CFServiceBroker{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      uuid.NewString(),
			},
			Spec: korifiv1alpha1.CFServiceBrokerSpec{
				ServiceBroker: services.ServiceBroker{
					Name: uuid.NewString(),
					URL:  brokerServer.URL(),
				},
				Credentials: corev1.LocalObjectReference{
					Name: credentialsSecret.Name,
				},
			},
		}
		Expect(adminClient.Create(ctx, cfServiceBroker)).To(Succeed())

		factory = osbapi.NewClientFactory(adminClient, true)
	})

	JustBeforeEach(func() {
		osbapiClient, createClientErr = factory.CreateClient(ctx, cfServiceBroker)
	})

	It("creates a osbapi client", func() {
		Expect(createClientErr).NotTo(HaveOccurred())

		catalog, err := osbapiClient.GetCatalog(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(catalog.Services).NotTo(BeEmpty())
	})

	When("the credentials secret cannot be found", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, cfServiceBroker, func() {
				cfServiceBroker.Spec.Credentials.Name = "i-do-not-exist"
			})).To(Succeed())
		})

		It("returns an error", func() {
			Expect(createClientErr).To(MatchError(ContainSubstring("not found")))
		})
	})

	When("the broker credentials are invalid", func() {
		var credentialsSecret *corev1.Secret

		BeforeEach(func() {
			credentialsSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      cfServiceBroker.Spec.Credentials.Name,
				},
			}
			Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)).To(Succeed())
			Expect(k8s.PatchResource(ctx, adminClient, credentialsSecret, func() {
				credentialsSecret.Data = map[string][]byte{
					"foo": []byte("bar"),
				}
			})).To(Succeed())
		})

		It("returns an error", func() {
			Expect(createClientErr).To(MatchError(ContainSubstring("failed to unmarshal broker credentials secret")))
		})

		When("username is not set", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, credentialsSecret, func() {
					credentialsSecret.Data = map[string][]byte{
						tools.CredentialsSecretKey: []byte(`{"password": "my-password"}`),
					}
				})).To(Succeed())
			})

			It("returns an error", func() {
				Expect(createClientErr).To(MatchError(ContainSubstring("username: cannot be blank")))
			})
		})

		When("password is not set", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, credentialsSecret, func() {
					credentialsSecret.Data = map[string][]byte{
						tools.CredentialsSecretKey: []byte(`{"username": "my-user"}`),
					}
				})).To(Succeed())
			})

			It("returns an error", func() {
				Expect(createClientErr).To(MatchError(ContainSubstring("password: cannot be blank")))
			})
		})
	})

	When("the client does not trust insecure brokers", func() {
		BeforeEach(func() {
			factory = osbapi.NewClientFactory(adminClient, false)
		})

		It("creates a client that does not trust insecure brokers", func() {
			Expect(createClientErr).NotTo(HaveOccurred())

			_, createClientErr = osbapiClient.GetCatalog(ctx)
			Expect(createClientErr).To(MatchError(ContainSubstring("failed to verify certificate")))
		})
	})
})
