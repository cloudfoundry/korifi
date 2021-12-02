package integration_test

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/tests/integration/helpers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		clientFactory = repositories.NewUnprivilegedClientFactory(k8sConfig)
	})

	JustBeforeEach(func() {
		userClient, buildClientErr = clientFactory.BuildClient(authInfo)
	})

	Describe("using the client", func() {
		var podListErr error

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
				Expect(apierrors.IsForbidden(podListErr)).To(BeTrue())
			})

			When("a role binding exists", func() {
				BeforeEach(func() {
					allowListingPods(userName)
				})

				It("allows listing pods", func() {
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
				Expect(apierrors.IsForbidden(podListErr)).To(BeTrue())
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

	Context("bad auth info content", func() {
		When("auth info is empty", func() {
			BeforeEach(func() {
				authInfo = authorization.Info{}
			})

			It("fails", func() {
				fmt.Printf("buildClientErr = %+v\n", buildClientErr)
				Expect(authorization.IsNotAuthenticated(buildClientErr)).To(BeTrue())
			})
		})

		When("the auth is not valid", func() {
			BeforeEach(func() {
				authInfo.Token = "xxx"
			})

			It("fails", func() {
				Expect(authorization.IsInvalidAuth(buildClientErr)).To(BeTrue())
			})
		})
	})
})
