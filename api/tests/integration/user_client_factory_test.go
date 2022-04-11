package integration_test

import (
	"context"
	"sync"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	thelpers "code.cloudfoundry.org/cf-k8s-controllers/api/tests/helpers"
	"code.cloudfoundry.org/cf-k8s-controllers/api/tests/integration/helpers"
	"code.cloudfoundry.org/cf-k8s-controllers/tests/matchers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var _ = Describe("Unprivileged User Client Factory", func() {
	var (
		userClient     client.Client
		buildClientErr error
		authInfo       authorization.Info
		ctx            context.Context
		userName       string
		clientFactory  repositories.UnprivilegedClientFactory
	)

	BeforeEach(func() {
		authInfo = authorization.Info{}
		ctx = context.Background()
		userName = uuid.NewString()
		mapper, err := apiutil.NewDynamicRESTMapper(k8sConfig)
		Expect(err).NotTo(HaveOccurred())
		clientFactory = repositories.NewUnprivilegedClientFactory(k8sConfig, mapper)
	})

	JustBeforeEach(func() {
		userClient, buildClientErr = clientFactory.BuildClient(authInfo)
	})

	allowListingPods := func(user string) {
		listPodClusterRole := rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: userName + "-list-pods",
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"list"},
					APIGroups: []string{""},
					Resources: []string{"pods"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, &listPodClusterRole)).To(Succeed())

		Expect(k8sClient.Create(ctx, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: userName,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind: rbacv1.UserKind,
					Name: user,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     listPodClusterRole.Name,
			},
		})).To(Succeed())
	}

	Describe("using the client", func() {
		var podListErr error

		JustBeforeEach(func() {
			Expect(buildClientErr).NotTo(HaveOccurred())

			podList := &corev1.PodList{}
			podListErr = userClient.List(ctx, podList)
		})

		Context("certificates", func() {
			BeforeEach(func() {
				cert, key := helpers.ObtainClientCert(testEnv, userName)
				authInfo.CertData = helpers.JoinCertAndKey(cert, key)
			})

			It("succeeds and forbids access to the user", func() {
				Expect(buildClientErr).NotTo(HaveOccurred())
				Expect(k8serrors.IsForbidden(podListErr)).To(BeTrue())
			})

			When("a role binding exists", func() {
				BeforeEach(func() {
					allowListingPods(userName)
				})

				FIt("allows listing pods", func() {
					Expect(buildClientErr).NotTo(HaveOccurred())
					Expect(podListErr).NotTo(HaveOccurred())
				})
			})
		})

		Context("tokens", func() {
			BeforeEach(func() {
				token := authProvider.GenerateJWTToken(userName)
				authInfo.Token = token
			})

			It("succeeds and forbids access to the user", func() {
				Expect(buildClientErr).NotTo(HaveOccurred())
				Expect(k8serrors.IsForbidden(podListErr)).To(BeTrue())
			})

			When("a role binding exists", func() {
				BeforeEach(func() {
					allowListingPods(oidcPrefix + userName)
				})

				It("allows listing pods", func() {
					Expect(buildClientErr).NotTo(HaveOccurred())
					Expect(podListErr).NotTo(HaveOccurred())
				})
			})
		})
	})

	Context("isolation", func() {
		When("two clients are created simulaneously", func() {
			var (
				name1     string
				name2     string
				authInfo1 authorization.Info
				authInfo2 authorization.Info
				client1   client.Client
				client2   client.Client
			)

			BeforeEach(func() {
				name1 = uuid.NewString()
				name2 = uuid.NewString()
				cert1, key1 := helpers.ObtainClientCert(testEnv, name1)
				cert2, key2 := helpers.ObtainClientCert(testEnv, name2)
				authInfo1.CertData = helpers.JoinCertAndKey(cert1, key1)
				authInfo2.CertData = helpers.JoinCertAndKey(cert2, key2)
				allowListingPods(name1)
			})

			It("doesn't muddle up their config", func() {
				for i := 0; i < 50; i++ {
					var wg sync.WaitGroup
					wg.Add(2)

					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						var err error
						client1, err = clientFactory.BuildClient(authInfo1)
						Expect(err).NotTo(HaveOccurred(), "iteration: %d", i)
					}()

					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						var err error
						client2, err = clientFactory.BuildClient(authInfo2)
						Expect(err).NotTo(HaveOccurred(), "iteration: %d", i)
					}()

					wg.Wait()

					podList := &corev1.PodList{}
					err := client1.List(ctx, podList)
					Expect(err).ToNot(HaveOccurred(), "expected user: %s, iteration: %d", name1, i)

					client2, err = clientFactory.BuildClient(authInfo2)
					Expect(err).NotTo(HaveOccurred())
					err = client2.List(ctx, podList)
					Expect(err).To(HaveOccurred(), "iteration: %d", i)
				}
			})
		})
	})

	Context("bad auth info content", func() {
		When("auth info is empty", func() {
			BeforeEach(func() {
				authInfo = authorization.Info{}
			})

			It("fails", func() {
				Expect(buildClientErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotAuthenticatedError{}))
			})
		})

		When("the token is not valid", func() {
			BeforeEach(func() {
				authInfo.Token = "xxx"
			})

			It("creates an unusable client", func() {
				Expect(buildClientErr).NotTo(HaveOccurred())
				usageErr := userClient.List(ctx, &corev1.PodList{})
				Expect(k8serrors.IsUnauthorized(usageErr)).To(BeTrue())
			})
		})

		When("the cert is not valid", func() {
			BeforeEach(func() {
				authInfo.CertData = []byte("not a cert")
			})

			It("fails", func() {
				Expect(buildClientErr).To(MatchError(ContainSubstring("failed to decode cert PEM")))
			})
		})

		When("the cert is not valid on this cluster", func() {
			BeforeEach(func() {
				authInfo.CertData = thelpers.CreateCertificatePEM()
			})

			It("creates an unusable client", func() {
				Expect(buildClientErr).NotTo(HaveOccurred())
				usageErr := userClient.List(ctx, &corev1.PodList{})
				Expect(k8serrors.IsUnauthorized(usageErr)).To(BeTrue())
			})
		})
	})
})
