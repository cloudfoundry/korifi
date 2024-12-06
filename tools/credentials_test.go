package tools_test

import (
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

type credsType struct {
	Foo string
	Bar struct {
		InBar string
	}
}

var _ = Describe("Credentials", func() {
	var (
		credsObject        any
		secretData         map[string][]byte
		decodedCredentials map[string]any
		err                error
	)

	BeforeEach(func() {
		credsObject = credsType{
			Foo: "foo",
			Bar: struct{ InBar string }{
				InBar: "in-bar",
			},
		}

		secretData, err = tools.ToCredentialsSecretData(credsObject)
	})

	When("ToCredentialsSecretData", func() {
		It("successfully creates credentials secret data", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(secretData).To(MatchAllKeys(Keys{
				tools.CredentialsSecretKey: MatchJSON(`{
					"Foo": "foo",
					"Bar": {
						"InBar": "in-bar"
					}
				}`),
			}))
		})
	})

	When("ToDecodedSecretDataCredentials", func() {
		BeforeEach(func() {
			decodedCredentials, err = tools.ToDecodedSecretDataCredentials(secretData)
			Expect(err).NotTo(HaveOccurred())
		})

		It("successfully decodes credentials from secret data", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(decodedCredentials).To(HaveKeyWithValue("Foo", "foo"))
			Expect(decodedCredentials).To(HaveKey("Bar"))
			Expect(decodedCredentials["Bar"]).To(HaveKeyWithValue("InBar", "in-bar"))
		})
	})
})
