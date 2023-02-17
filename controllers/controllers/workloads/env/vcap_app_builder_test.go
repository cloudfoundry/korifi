package env_test

import (
	"encoding/json"

	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("VCAP_APPLICATION env value builder", func() {
	var builder *env.VCAPApplicationEnvValueBuilder

	BeforeEach(func() {
		builder = env.NewVCAPApplicationEnvValueBuilder(k8sClient, nil)
	})

	Describe("BuildEnvValue", func() {
		var (
			vcapApplication                 map[string]string
			buildVCAPApplicationEnvValueErr error
		)

		JustBeforeEach(func() {
			vcapApplication, buildVCAPApplicationEnvValueErr = builder.BuildEnvValue(ctx, cfApp)
		})

		It("sets the basic fields", func() {
			Expect(buildVCAPApplicationEnvValueErr).ToNot(HaveOccurred())
			Expect(vcapApplication).To(HaveKey("VCAP_APPLICATION"))
			vcapAppValue := map[string]any{}
			Expect(json.Unmarshal([]byte(vcapApplication["VCAP_APPLICATION"]), &vcapAppValue)).To(Succeed())
			Expect(vcapAppValue).To(HaveKeyWithValue("application_id", cfApp.Name))
			Expect(vcapAppValue).To(HaveKeyWithValue("application_name", cfApp.Spec.DisplayName))
			Expect(vcapAppValue).To(HaveKeyWithValue("name", cfApp.Spec.DisplayName))
			Expect(vcapAppValue).To(HaveKeyWithValue("space_id", cfSpace.Name))
			Expect(vcapAppValue).To(HaveKeyWithValue("space_name", cfSpace.Spec.DisplayName))
			Expect(vcapAppValue).To(HaveKeyWithValue("organization_id", cfOrg.Name))
			Expect(vcapAppValue).To(HaveKeyWithValue("organization_name", cfOrg.Spec.DisplayName))
		})

		When("extra values are provided", func() {
			BeforeEach(func() {
				builder = env.NewVCAPApplicationEnvValueBuilder(k8sClient, map[string]any{
					"application_id": "not-the-application-id",
					"foo":            "bar",
					"answer":         42,
					"innit":          true,
					"x":              map[string]any{"y": "z"},
				})
			})

			It("includes them", func() {
				Expect(buildVCAPApplicationEnvValueErr).ToNot(HaveOccurred())
				Expect(vcapApplication).To(HaveKey("VCAP_APPLICATION"))
				vcapAppValue := map[string]any{}
				Expect(json.Unmarshal([]byte(vcapApplication["VCAP_APPLICATION"]), &vcapAppValue)).To(Succeed())
				Expect(vcapAppValue).To(HaveKeyWithValue("application_id", cfApp.Name))
				Expect(vcapAppValue).To(HaveKeyWithValue("foo", "bar"))
				Expect(vcapAppValue).To(HaveKeyWithValue("answer", 42.0))
				Expect(vcapAppValue).To(HaveKeyWithValue("innit", true))
				Expect(vcapAppValue).To(HaveKeyWithValue("x", HaveKeyWithValue("y", "z")))
			})
		})
	})
})
