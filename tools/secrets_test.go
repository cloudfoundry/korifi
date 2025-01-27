package tools_test

import (
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

type dataType struct {
	Foo string
	Bar struct {
		InBar string
	}
}

var _ = Describe("Secrets", func() {
	Describe("Credentials", func() {
		var (
			credsObject any
			secretData  map[string][]byte
		)

		BeforeEach(func() {
			credsObject = dataType{
				Foo: "foo",
				Bar: struct{ InBar string }{
					InBar: "in-bar",
				},
			}
		})

		Describe("ToCredentialsSecretData", func() {
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

		Describe("FromCredentialsSecretData", func() {
			var (
				decodedCredentials map[string]any
				decodeErr          error
			)

			BeforeEach(func() {
				var err error
				secretData, err = tools.ToCredentialsSecretData(credsObject)
				Expect(err).NotTo(HaveOccurred())
			})

			JustBeforeEach(func() {
				decodedCredentials, decodeErr = tools.FromCredentialsSecretData(secretData)
			})

			It("successfully decodes credentials from secret data", func() {
				Expect(decodeErr).NotTo(HaveOccurred())

				Expect(decodedCredentials).To(HaveKeyWithValue("Foo", "foo"))
				Expect(decodedCredentials).To(HaveKey("Bar"))
				Expect(decodedCredentials["Bar"]).To(HaveKeyWithValue("InBar", "in-bar"))
			})

			When("the secret data is missing the credentials key", func() {
				BeforeEach(func() {
					secretData = map[string][]byte{
						"foo": {},
					}
				})

				It("returns an error", func() {
					Expect(decodeErr).To(MatchError(ContainSubstring("failed to get credentials")))
				})
			})

			When("the secret data is invalid", func() {
				BeforeEach(func() {
					secretData = map[string][]byte{
						"foo": []byte("invalid-json"),
					}
				})

				It("returns an error", func() {
					Expect(decodeErr).To(MatchError(ContainSubstring("failed to get credentials")))
				})
			})
		})
	})

	Describe("Parameters", func() {
		var (
			paramsObject any
			secretData   map[string][]byte
		)
		BeforeEach(func() {
			paramsObject = dataType{
				Foo: "foo",
				Bar: struct{ InBar string }{
					InBar: "in-bar",
				},
			}
		})

		Describe("ToParametersSecretData", func() {
			JustBeforeEach(func() {
				var err error
				secretData, err = tools.ToParametersSecretData(paramsObject)
				Expect(err).NotTo(HaveOccurred())
			})

			It("creates parameters secret data", func() {
				Expect(secretData).To(MatchAllKeys(Keys{
					tools.ParametersSecretKey: MatchJSON(`{
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
				decodedParameters map[string]any
				decodeErr         error
			)

			BeforeEach(func() {
				var err error
				secretData, err = tools.ToParametersSecretData(paramsObject)
				Expect(err).NotTo(HaveOccurred())
			})

			JustBeforeEach(func() {
				decodedParameters, decodeErr = tools.FromParametersSecretData(secretData)
			})

			It("successfully decodes parameters from secret data", func() {
				Expect(decodeErr).NotTo(HaveOccurred())

				Expect(decodedParameters).To(HaveKeyWithValue("Foo", "foo"))
				Expect(decodedParameters).To(HaveKey("Bar"))
				Expect(decodedParameters["Bar"]).To(HaveKeyWithValue("InBar", "in-bar"))
			})

			When("the secret data is missing the parameters key", func() {
				BeforeEach(func() {
					secretData = map[string][]byte{
						"foo": {},
					}
				})

				It("returns an error", func() {
					Expect(decodeErr).To(MatchError(ContainSubstring("failed to get parameters")))
				})
			})
		})
	})
})
