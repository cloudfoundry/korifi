package integration_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/apis"
	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Role", func() {
	var (
		apiHandler *apis.RoleHandler
		org        *v1alpha1.CFOrg
		space      *v1alpha1.CFSpace
	)

	BeforeEach(func() {
		// NOTE: This creates an arbitrary mapping that is not representative of production mapping
		roleMappings := map[string]config.Role{
			"space_developer":      {Name: "cf-space-developer"},
			"organization_manager": {Name: "cf-organization-manager"},
			"cf_user":              {Name: "cf-root-namespace-user"},
		}
		orgRepo := repositories.NewOrgRepo(rootNamespace, k8sClient, clientFactory, nsPermissions, time.Minute)
		spaceRepo := repositories.NewSpaceRepo(namespaceRetriever, orgRepo, clientFactory, nsPermissions, time.Minute)
		roleRepo := repositories.NewRoleRepo(clientFactory, spaceRepo, nsPermissions, rootNamespace, roleMappings)
		decoderValidator, err := apis.NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		apiHandler = apis.NewRoleHandler(*serverURL, roleRepo, decoderValidator)
		apiHandler.RegisterRoutes(router)

		org = createOrgWithCleanup(ctx, generateGUID())
		space = createSpaceWithCleanup(ctx, org.Name, "spacename-"+generateGUID())
	})

	Describe("creation", func() {
		var (
			bodyTemplate  string
			roleName      string
			userGUID      string
			orgSpaceLabel string
			orgSpaceGUID  string
		)

		BeforeEach(func() {
			roleName = ""
			userGUID = "testuser-" + generateGUID()
			orgSpaceLabel = ""
			orgSpaceGUID = ""

			bodyTemplate = `{
              "type": %q,
              "relationships": {
                "user": {"data": {"guid": %q}},
                %q: {"data": {"guid": %q}}
              }
            }`
		})

		createTheRole := func(respRecorder *httptest.ResponseRecorder) {
			requestBody := fmt.Sprintf(bodyTemplate, roleName, userGUID, orgSpaceLabel, orgSpaceGUID)

			createRoleReq, err := http.NewRequestWithContext(ctx, "POST", serverURI("/v3/roles"), strings.NewReader(requestBody))
			Expect(err).NotTo(HaveOccurred())

			createRoleReq.Header.Add("Content-type", "application/json")
			router.ServeHTTP(respRecorder, createRoleReq)
		}

		JustBeforeEach(func() {
			createTheRole(rr)
		})

		Describe("creating an org role", func() {
			BeforeEach(func() {
				roleName = "organization_manager"
				orgSpaceLabel = "organization"
				orgSpaceGUID = org.Name
			})

			It("fails when the user is not allowed to create org roles", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusForbidden))
			})

			When("the user is admin", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
					createRoleBinding(ctx, userName, adminRole.Name, org.Name)
				})

				It("succeeds", func() {
					Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
				})

				When("the role already exists for the user", func() {
					BeforeEach(func() {
						duplicateRr := httptest.NewRecorder()
						createTheRole(duplicateRr)
						Expect(duplicateRr).To(HaveHTTPStatus(http.StatusCreated))
					})

					It("returns unprocessable entity", func() {
						Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
					})
				})
			})
		})

		Describe("creating a space role", func() {
			BeforeEach(func() {
				roleName = "space_developer"
				orgSpaceLabel = "space"
				orgSpaceGUID = space.Name
			})

			It("fails when the user doesn't have a role binding in the space's org", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
			})

			When("the user is an org user", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userGUID, orgUserRole.Name, org.Name)

					createRoleBinding(ctx, userName, orgUserRole.Name, org.Name)
					createRoleBinding(ctx, userName, orgUserRole.Name, space.Name)
				})

				It("fails when the user is not allowed to create space roles", func() {
					Expect(rr).To(HaveHTTPStatus(http.StatusForbidden))
				})

				When("the user is admin", func() {
					BeforeEach(func() {
						createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
						createRoleBinding(ctx, userName, adminRole.Name, org.Name)
						createRoleBinding(ctx, userName, adminRole.Name, space.Name)
					})

					It("succeeds", func() {
						Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
					})

					When("the role already exists for the user", func() {
						BeforeEach(func() {
							duplicateRr := httptest.NewRecorder()
							createTheRole(duplicateRr)
							Expect(duplicateRr).To(HaveHTTPStatus(http.StatusCreated))
						})

						It("returns unprocessable entity", func() {
							Expect(rr).To(HaveHTTPStatus(http.StatusUnprocessableEntity))
						})
					})
				})
			})
		})
	})
})
