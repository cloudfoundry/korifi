package integration_test

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BuildUserClient", func() {
	var (
		userClient     client.Client
		buildClientErr error
		authHeader     string
		ctx            context.Context
		userName       string
	)

	BeforeEach(func() {
		authHeader = ""
		ctx = context.Background()
		userName = uuid.NewString()
	})

	JustBeforeEach(func() {
		userClient, buildClientErr = repositories.BuildUserClient(k8sConfig, authHeader)
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
				cert, key := obtainClientCert(userName)
				authHeader = "clientcert " + encodeCertAndKey(cert, key)
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
				authHeader = "bearer " + token
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

	Context("bad auth header content", func() {
		When("no auth header is passed", func() {
			BeforeEach(func() {
				authHeader = ""
			})
			It("fails", func() {
				Expect(authorization.IsNotAuthenticated(buildClientErr)).To(BeTrue())
			})
		})

		When("auth header is not two values", func() {
			BeforeEach(func() {
				authHeader = "bearer"
			})

			It("fails", func() {
				Expect(buildClientErr).To(HaveOccurred())
			})
		})

		When("auth header scheme is not recognised", func() {
			BeforeEach(func() {
				authHeader = "superSecure xxx"
			})

			It("fails", func() {
				Expect(buildClientErr).To(HaveOccurred())
			})
		})

		When("the auth is not valid", func() {
			BeforeEach(func() {
				authHeader = "bearer xxx"
			})

			It("fails", func() {
				Expect(authorization.IsInvalidAuth(buildClientErr)).To(BeTrue())
			})
		})
	})
})
