package env_test

import (
	"encoding/json"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/tests/helpers"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("VCAP_APPLICATION env value builder", func() {
	var builder *env.VCAPApplicationEnvValueBuilder

	BeforeEach(func() {
		builder = env.NewVCAPApplicationEnvValueBuilder(controllersClient, nil)
	})

	Describe("BuildEnvValue", func() {
		var (
			vcapApplication                 map[string][]byte
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
			Expect(vcapAppValue).To(HaveKeyWithValue("uris", BeEmpty()))
			Expect(vcapAppValue).To(HaveKeyWithValue("application_uris", BeEmpty()))
		})

		When("extra values are provided", func() {
			BeforeEach(func() {
				builder = env.NewVCAPApplicationEnvValueBuilder(controllersClient, map[string]any{
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

		When("application routes are provided", func() {
			var (
				appRoute *korifiv1alpha1.CFRoute
				routeUri string
			)

			BeforeEach(func() {
				appRoute = &korifiv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: cfApp.Namespace,
						Name:      "vcap-application-my-route",
					},
					Spec: korifiv1alpha1.CFRouteSpec{
						Destinations: []korifiv1alpha1.Destination{{
							GUID: "destination-123-456",
							AppRef: v1.LocalObjectReference{
								Name: cfApp.Name,
							},
						}},
					},
				}

				helpers.EnsureCreate(controllersClient, appRoute)

				routeUri = cfApp.Name + ".mydomain.platform.com"

				helpers.EnsurePatch(controllersClient, appRoute, func(appRoute *korifiv1alpha1.CFRoute) {
					appRoute.Status.URI = routeUri
				})
			})

			It("includes application uris", func() {
				Expect(buildVCAPApplicationEnvValueErr).ToNot(HaveOccurred())
				Expect(vcapApplication).To(HaveKey("VCAP_APPLICATION"))
				vcapAppValue := map[string]any{}
				Expect(json.Unmarshal([]byte(vcapApplication["VCAP_APPLICATION"]), &vcapAppValue)).To(Succeed())
				Expect(vcapAppValue).To(HaveKeyWithValue("application_id", cfApp.Name))
				Expect(vcapAppValue).To(HaveKeyWithValue("application_uris", ConsistOf(routeUri)))
				Expect(vcapAppValue).To(HaveKeyWithValue("uris", ConsistOf(routeUri)))
			})
		})
	})
})
