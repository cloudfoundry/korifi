package e2e_test

import (
	"context"
	"net/http"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Routes", func() {
	var (
		orgGUID    string
		spaceGUID  string
		domainGUID string
		routeGUID  string
		resp       *resty.Response
		errResp    cfErrs
	)

	BeforeEach(func() {
		orgGUID = createOrg(generateGUID("org"))
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)

		spaceGUID = createSpace(generateGUID("space"), orgGUID)

		domainGUID = createDomain("example.org")
		routeGUID = createRoute("my-app", "", spaceGUID, domainGUID)
	})

	AfterEach(func() {
		deleteOrg(orgGUID)
	})

	Describe("Fetch an route", func() {
		var result resource

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetResult(&result).
				SetError(&errResp).
				Get("/v3/routes/" + routeGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is authorized in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
				// Temporarily grant the user the ability to get a domain in the root namespace.
				// This is a workaround to the issue of not having RBAC rules to allow the admin
				// user to get domains in the root "cf" namespace.
				// The commit that adds this should be reverted once we have a mechanism in place
				// to grant the admin user the required RBAC rules to maintain CFDomains.
				roleBinding := v1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: rootNamespace,
					},
					RoleRef: v1.RoleRef{
						Kind: "ClusterRole",
						Name: "cf-k8s-controllers-space-developer",
					},
					Subjects: []v1.Subject{
						{
							Kind: rbacv1.UserKind,
							Name: certUserName,
						},
					},
				}
				err := k8sClient.Create(context.Background(), &roleBinding)
				Expect(err).NotTo(HaveOccurred())
			})

			It("can fetch the route", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(routeGUID))
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a not found error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusNotFound))
				Expect(errResp.Errors).To(ConsistOf(
					cfErr{
						Detail: "Route not found",
						Title:  "CF-ResourceNotFound",
						Code:   10010,
					},
				))
			})
		})
	})
})
