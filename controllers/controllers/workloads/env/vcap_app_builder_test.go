package env_test

import (
	"encoding/json"

	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("VCAP_APPLICATION env value builder", func() {
	var (
		vcapApplicationSecret *corev1.Secret
		builder               *env.VCAPApplicationEnvValueBuilder
	)

	BeforeEach(func() {
		builder = env.NewVCAPApplicationEnvValueBuilder(k8sClient)

		vcapApplicationSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-guid-vcap-application",
				Namespace: cfSpace.Status.GUID,
			},
			Data: map[string][]byte{"VCAP_APPLICATION": []byte(`{"foo":"bar"}`)},
		}
		Expect(k8sClient.Create(ctx, vcapApplicationSecret)).To(Succeed())
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
			vcapAppValue := map[string]string{}
			Expect(json.Unmarshal([]byte(vcapApplication["VCAP_APPLICATION"]), &vcapAppValue)).To(Succeed())
			Expect(vcapAppValue).To(HaveKeyWithValue("application_id", cfApp.Name))
			Expect(vcapAppValue).To(HaveKeyWithValue("application_name", cfApp.Spec.DisplayName))
			Expect(vcapAppValue).To(HaveKeyWithValue("name", cfApp.Spec.DisplayName))
			Expect(vcapAppValue).To(HaveKeyWithValue("cf_api", BeEmpty()))
			Expect(vcapAppValue).To(HaveKeyWithValue("space_id", cfSpace.Name))
			Expect(vcapAppValue).To(HaveKeyWithValue("space_name", cfSpace.Spec.DisplayName))
			Expect(vcapAppValue).To(HaveKeyWithValue("organization_id", cfOrg.Name))
			Expect(vcapAppValue).To(HaveKeyWithValue("organization_name", cfOrg.Spec.DisplayName))
		})
	})
})
