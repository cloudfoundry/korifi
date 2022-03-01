package e2e_test

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	hncv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("Routes", func() {
	var (
		client     *resty.Client
		domainGUID string
		domainName string
		orgGUID    string
		spaceGUID  string
		host       string
		path       string

		cfDomainRoleName    string // temp hack
		cfDomainBindingName string // temp hack
	)

	BeforeEach(func() {
		host = generateGUID("myapp")
		path = generateGUID("/some-path")
		orgGUID = createOrg(generateGUID("org"))
		spaceGUID = createSpace(generateGUID("space"), orgGUID)
		client = certClient

		// temp hack for cfdomain permission problem in root CF namespace
		cfDomainRoleName = generateGUID("cfdomain-role")
		Expect(k8sClient.Create(context.Background(), &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: cfDomainRoleName,
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"get", "list"},
					APIGroups: []string{"networking.cloudfoundry.org"},
					Resources: []string{"cfdomains"},
				},
			},
		})).To(Succeed())

		cfDomainBindingName = generateGUID("cfdomain-binding")
		Expect(k8sClient.Create(context.Background(), &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfDomainBindingName,
				Namespace: rootNamespace,
				Annotations: map[string]string{
					hncv1alpha2.AnnotationNoneSelector: "true",
				},
			},
			Subjects: []rbacv1.Subject{
				{Kind: rbacv1.UserKind, Name: certUserName},
			},
			RoleRef: rbacv1.RoleRef{
				Kind: "ClusterRole",
				Name: cfDomainRoleName,
			},
		})).To(Succeed())
		// end of hack

		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)
		createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)

		domainName = generateGUID("domain-name")
		domainGUID = createDomain(domainName)
	})

	AfterEach(func() {
		deleteOrg(orgGUID)
		deleteDomain(domainGUID)

		// temp hack for cfdomain
		Expect(k8sClient.Delete(context.Background(), &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: cfDomainBindingName, Namespace: rootNamespace},
		})).To(Succeed())
		Expect(k8sClient.Delete(context.Background(), &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: cfDomainRoleName},
		})).To(Succeed())
		// end of hack
	})

	Describe("Fetch a route", func() {
		var (
			result    resource
			resp      *resty.Response
			errResp   cfErrs
			routeGUID string
		)

		BeforeEach(func() {
			routeGUID = createRoute(host, path, spaceGUID, domainGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetResult(&result).
				SetError(&errResp).
				Get("/v3/routes/" + routeGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user is authorized in the space", func() {
			It("can fetch the route", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(result.GUID).To(Equal(routeGUID))
			})
		})

		When("the user is not authorized in the space", func() {
			BeforeEach(func() {
				client = tokenClient
			})

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

	Describe("creation", func() {
		var (
			resp      *resty.Response
			createErr cfErrs
			route     routeResource
		)

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetBody(routeResource{
					resource: resource{
						Relationships: map[string]relationship{
							"domain": {Data: resource{GUID: domainGUID}},
							"space":  {Data: resource{GUID: spaceGUID}},
						},
					},
					Host: host,
					Path: path,
				}).
				SetResult(&route).
				SetError(&createErr).
				Post("/v3/routes")
			Expect(err).NotTo(HaveOccurred())
		})

		It("can create a route", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(route.URL).To(SatisfyAll(
				HavePrefix(host),
				HaveSuffix(path),
			))
		})

		When("the route already exists", func() {
			BeforeEach(func() {
				createRoute(host, path, spaceGUID, domainGUID)
			})

			It("fails with a duplicate error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(createErr.Errors).To(ConsistOf(cfErr{
					Detail: fmt.Sprintf("Route already exists with host '%s' and path '%s' for domain '%s'.", host, path, domainName),
					Title:  "CF-UnprocessableEntity",
					Code:   10008,
				}))
			})
		})

		When("the route already exists in another space", func() {
			BeforeEach(func() {
				anotherSpaceGUID := createSpace(generateGUID("another-space"), orgGUID)
				createRoute(host, path, anotherSpaceGUID, domainGUID)
			})

			It("fails with a duplicate error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(createErr.Errors).To(ConsistOf(cfErr{
					Detail: fmt.Sprintf("Route already exists with host '%s' and path '%s' for domain '%s'.", host, path, domainName),
					Title:  "CF-UnprocessableEntity",
					Code:   10008,
				}))
			})
		})

		When("there is no context path", func() {
			BeforeEach(func() {
				path = ""
				createRoute(host, path, spaceGUID, domainGUID)
			})

			It("fails with a duplicate error", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(createErr.Errors).To(ConsistOf(cfErr{
					Detail: fmt.Sprintf("Route already exists with host '%s' for domain '%s'.", host, domainName),
					Title:  "CF-UnprocessableEntity",
					Code:   10008,
				}))
			})
		})
	})

	Describe("update destinations", func() {
		var (
			routeGUID string
			appGUID   string
			resp      *resty.Response
			result    destinationsResource
		)

		BeforeEach(func() {
			routeGUID = createRoute(host, path, spaceGUID, domainGUID)
			appGUID = generateGUID("app")
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				SetBody(destinationsResource{
					Destinations: []destination{
						{App: bareResource{GUID: appGUID}},
					},
				}).
				SetResult(&result).
				Post("/v3/routes/" + routeGUID + "/destinations")
			Expect(err).NotTo(HaveOccurred())
		})

		It("can modify a route", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Destinations).To(HaveLen(1))
			Expect(result.Destinations[0].App.GUID).To(Equal(appGUID))
		})
	})

	Describe("delete", func() {
		var (
			routeGUID string
			resp      *resty.Response
		)

		BeforeEach(func() {
			routeGUID = createRoute(host, path, spaceGUID, domainGUID)
		})

		JustBeforeEach(func() {
			var err error
			resp, err = client.R().
				Delete("/v3/routes/" + routeGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("can delete a route", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusAccepted))
			Expect(resp).To(HaveRestyHeaderWithValue("Location", SatisfyAll(
				HavePrefix(apiServerRoot),
				ContainSubstring("/v3/jobs/route.delete-"),
			)))
		})

		It("frees up the deleted route's name for reuse", func() {
			createRoute(host, path, spaceGUID, domainGUID)
		})
	})
})
