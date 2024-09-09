package credentials_test

import (
	"fmt"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Credentials", func() {
	var (
		err               error
		credentialsSecret *corev1.Secret
	)

	BeforeEach(func() {
		credentialsSecret = &corev1.Secret{
			Data: map[string][]byte{
				tools.CredentialsSecretKey: []byte(`{"foo":{"bar": "baz"}}`),
			},
		}
	})

	Describe("GetCredentials", func() {
		var creds map[string]any

		JustBeforeEach(func() {
			creds = map[string]any{}
			err = credentials.GetCredentials(credentialsSecret, &creds)
		})

		It("returns the credentials object", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(creds).To(MatchAllKeys(Keys{
				"foo": MatchAllKeys(Keys{
					"bar": Equal("baz"),
				}),
			}))
		})

		When("the credentials cannot be unmarshalled", func() {
			BeforeEach(func() {
				credentialsSecret.Data[tools.CredentialsSecretKey] = []byte("invalid")
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("failed to unmarshal secret data")))
			})
		})

		When("the credentials key is missing from the secret data", func() {
			BeforeEach(func() {
				credentialsSecret.Data = map[string][]byte{
					"foo": {},
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring(
					fmt.Sprintf("does not contain the %q key", tools.CredentialsSecretKey),
				)))
			})
		})
	})

	Describe("GetBindingSecretType", func() {
		var secretType corev1.SecretType

		JustBeforeEach(func() {
			secretType, err = credentials.GetBindingSecretType(credentialsSecret)
		})

		It("returns user-provided type by default", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(secretType).To(BeEquivalentTo(credentials.ServiceBindingSecretTypePrefix + "user-provided"))
		})

		When("the type is specified in the credentials", func() {
			BeforeEach(func() {
				credentialsSecret.Data[tools.CredentialsSecretKey] = []byte(`{"type":"my-type"}`)
			})

			It("returns the speicified type", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(secretType).To(BeEquivalentTo(credentials.ServiceBindingSecretTypePrefix + "my-type"))
			})
		})

		When("the type is not a plain string", func() {
			BeforeEach(func() {
				credentialsSecret.Data[tools.CredentialsSecretKey] = []byte(`{"type":{"my-type":"is-not-a-string"}}`)
			})

			It("returns user-provided type", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(secretType).To(BeEquivalentTo(credentials.ServiceBindingSecretTypePrefix + "user-provided"))
			})
		})
	})

	Describe("GetServiceBindingIOSecretData", func() {
		var bindingSecretData map[string][]byte

		JustBeforeEach(func() {
			bindingSecretData, err = credentials.GetServiceBindingIOSecretData(credentialsSecret)
			Expect(err).NotTo(HaveOccurred())
		})

		It("converts the credentials into a flat strings map", func() {
			Expect(bindingSecretData).To(MatchAllKeys(Keys{
				"type": BeEquivalentTo("user-provided"),
				"foo":  BeEquivalentTo(`{"bar":"baz"}`),
			}))
		})

		When("the credentials secrets has a 'type' attribute specified", func() {
			BeforeEach(func() {
				credentialsSecret.Data = map[string][]byte{
					tools.CredentialsSecretKey: []byte(`{"type": "my-type"}`),
				}
			})

			It("overrides the default type", func() {
				Expect(bindingSecretData).To(MatchAllKeys(Keys{
					"type": BeEquivalentTo("my-type"),
				}))
			})
		})
	})

	Describe("ToCredentialsSecretData", func() {
		var creds any

		BeforeEach(func() {
			creds = struct {
				Str string
				Num int
			}{
				Str: "abc",
				Num: 5,
			}
		})

		It("converts credentials map to a k8s secret data", func() {
			secretData, err := tools.ToCredentialsSecretData(creds)
			Expect(err).NotTo(HaveOccurred())
			Expect(secretData).To(SatisfyAll(
				HaveLen(1),
				HaveKeyWithValue(tools.CredentialsSecretKey, MatchJSON(`{"Str":"abc", "Num":5}`)),
			))
		})
	})
})
