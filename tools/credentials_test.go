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
		credsObject any
		err         error
	)

	BeforeEach(func() {
		credsObject = credsType{
			Foo: "foo",
			Bar: struct{ InBar string }{
				InBar: "in-bar",
			},
		}
	})

	Describe("ToCredentialsSecretData", func() {
		var secretData map[string][]byte

		BeforeEach(func() {
			secretData, err = tools.ToCredentialsSecretData(credsObject)
		})

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

	Describe("FromCredentialsSecretData", func() {
		var (
			decodedCredentials map[string]any
			secretData         map[string][]byte
		)
		BeforeEach(func() {
			secretData, err = tools.ToCredentialsSecretData(credsObject)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			decodedCredentials, err = tools.FromCredentialsSecretData(secretData)
			Expect(err).NotTo(HaveOccurred())
		})

		It("successfully decodes credentials from secret data", func() {
			Expect(decodedCredentials).To(HaveKeyWithValue("Foo", "foo"))
			Expect(decodedCredentials).To(HaveKey("Bar"))
			Expect(decodedCredentials["Bar"]).To(HaveKeyWithValue("InBar", "in-bar"))
		})
	})
})
