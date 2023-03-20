package payloads_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	rbacv1 "k8s.io/api/rbac/v1"

	"code.cloudfoundry.org/korifi/api/payloads"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("RoleCreate", func() {
	var (
		createPayload payloads.RoleCreate
		roleCreate    *payloads.RoleCreate
		validatorErr  error
	)

	BeforeEach(func() {
		roleCreate = new(payloads.RoleCreate)
		createPayload = payloads.RoleCreate{
			Type: "space_manager",
			Relationships: payloads.RoleRelationships{
				User: &payloads.UserRelationship{
					Data: payloads.UserRelationshipData{
						Username: "cf-service-account",
					},
				},
				Space: &payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "cf-space-guid",
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		body, err := json.Marshal(createPayload)
		Expect(err).NotTo(HaveOccurred())

		req, err := http.NewRequest("", "", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())

		validatorErr = validator.DecodeAndValidateJSONPayload(req, roleCreate)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(roleCreate).To(PointTo(Equal(createPayload)))
	})

	Context("ToMessage()", func() {
		It("converts to repo message correctly", func() {
			msg := roleCreate.ToMessage()
			Expect(msg.Type).To(Equal("space_manager"))
			Expect(msg.Space).To(Equal("cf-space-guid"))
			Expect(msg.User).To(Equal("cf-service-account"))
			Expect(msg.Kind).To(Equal(rbacv1.UserKind))
		})
	})

	When("the service account name is provided", func() {
		BeforeEach(func() {
			createPayload.Relationships.User.Data.Username = "system:serviceaccount:cf-space-guid:cf-service-account"
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(roleCreate).To(PointTo(Equal(createPayload)))
		})

		Context("ToMessage()", func() {
			It("converts to repo message correctly", func() {
				msg := roleCreate.ToMessage()
				Expect(msg.Type).To(Equal("space_manager"))
				Expect(msg.Space).To(Equal("cf-space-guid"))
				Expect(msg.User).To(Equal("cf-service-account"))
				Expect(msg.Kind).To(Equal(rbacv1.ServiceAccountKind))
				Expect(msg.ServiceAccountNamespace).To(Equal("cf-space-guid"))
			})
		})
	})
})
