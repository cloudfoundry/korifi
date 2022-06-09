package utils_test

import (
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
)

var _ = Describe("EnvVar", func() {
	Describe("Map to Kuberentes EnvVar", func() {
		var (
			env     map[string]string
			envVars []v1.EnvVar
		)

		BeforeEach(func() {
			env = map[string]string{
				"foo":  "bar",
				"dora": "fedora",
			}
		})

		JustBeforeEach(func() {
			envVars = utils.MapToEnvVar(env)
		})

		It("translates key-values to EnvVars", func() {
			Expect(envVars).To(ConsistOf(v1.EnvVar{Name: "foo", Value: "bar"}, v1.EnvVar{Name: "dora", Value: "fedora"}))
		})

		Context("when env map is empty", func() {
			BeforeEach(func() {
				env = map[string]string{}
			})

			It("should return an empty slice", func() {
				Expect(envVars).To(BeEmpty())
			})
		})
	})
})
