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
		secretData  map[string][]byte
	)

	BeforeEach(func() {
		credsObject = credsType{
			Foo: "foo",
			Bar: struct{ InBar string }{
				InBar: "in-bar",
			},
		}
	})

	JustBeforeEach(func() {
		var err error
		secretData, err = tools.ToCredentialsSecretData(credsObject)
		Expect(err).NotTo(HaveOccurred())
	})

	It("creates credentials secret data", func() {
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
