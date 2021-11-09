package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Roles", func() {
	var (
		ctx      context.Context
		userName string
	)

	createRole := func(roleName, userName, spaceGUID string) (*http.Response, error) {
		rolesURL := apiServerRoot + "/v3/roles"
		body := fmt.Sprintf(`{
            "type": "%s",
            "relationships": {
                "user": {
                    "data": {
                        "guid": "%s"
                    }
                },
                "space": {
                    "data": {
                        "guid": "%s"
                    }
                }
            }
        }`, roleName, userName, spaceGUID)
		req, err := http.NewRequest(http.MethodPost, rolesURL, strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())

		return http.DefaultClient.Do(req)
	}

	createBinding := func(namespace, userName, roleName string) *rbacv1.RoleBinding {
		binding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind: rbacv1.UserKind,
					Name: userName,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind: "ClusterRole",
				Name: roleName,
			},
		}

		Expect(k8sClient.Create(ctx, binding)).To(Succeed())

		return binding
	}

	BeforeEach(func() {
		ctx = context.Background()
		userName = uuid.NewString()
	})

	Describe("creating a space role", func() {
		var (
			org   presenter.OrgResponse
			space presenter.SpaceResponse
		)

		BeforeEach(func() {
			org = createOrg(uuid.NewString())
			space = createSpace(uuid.NewString(), org.GUID)
			createBinding(org.GUID, userName, "basic-user")
		})

		AfterEach(func() {
			deleteSubnamespace(org.GUID, space.GUID)
			deleteSubnamespace(rootNamespace, org.GUID)
		})

		It("creates a role binding", func() {
			response, err := createRole("space_developer", userName, space.GUID)
			Expect(err).NotTo(HaveOccurred())

			Expect(response).To(HaveHTTPStatus(http.StatusCreated))

			defer response.Body.Close()

			responseMap := map[string]interface{}{}
			Expect(json.NewDecoder(response.Body).Decode(&responseMap)).To(Succeed())

			Expect(responseMap).To(HaveKeyWithValue("type", "space_developer"))

			roleBindingList := &rbacv1.RoleBindingList{}
			Eventually(func() ([]rbacv1.RoleBinding, error) {
				err := k8sClient.List(ctx, roleBindingList,
					client.InNamespace(space.GUID),
					client.MatchingLabels{
						repositories.RoleTypeLabel: "space_developer",
					},
				)
				if err != nil {
					return nil, err
				}
				return roleBindingList.Items, nil
			}).Should(HaveLen(1))

			binding := roleBindingList.Items[0]
			Expect(responseMap).To(HaveKeyWithValue("guid", binding.Labels[repositories.RoleGuidLabel]))
			Expect(binding.RoleRef.Name).To(Equal("cf-k8s-controllers-space-developer"))
			Expect(binding.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(binding.Subjects).To(HaveLen(1))
			subject := binding.Subjects[0]
			Expect(subject.Name).To(Equal(userName))
			Expect(subject.Kind).To(Equal(rbacv1.UserKind))
		})
	})
})
