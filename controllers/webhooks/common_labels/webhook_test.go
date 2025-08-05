package common_labels_test

import (
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CommonLabelsWebhook", func() {
	Describe("CFApp", test(&korifiv1alpha1.CFApp{
		Spec: korifiv1alpha1.CFAppSpec{
			DisplayName:  "cfapp",
			DesiredState: "STOPPED",
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}))

	Describe("CFBuild", test(&korifiv1alpha1.CFBuild{
		Spec: korifiv1alpha1.CFBuildSpec{
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}))

	Describe("CFDomain", test(&korifiv1alpha1.CFDomain{
		Spec: korifiv1alpha1.CFDomainSpec{
			Name: "example.com",
		},
	}))

	Describe("CFOrg", test(&korifiv1alpha1.CFOrg{
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: "example-org",
		},
	}))

	Describe("CFPackage", test(&korifiv1alpha1.CFPackage{
		Spec: korifiv1alpha1.CFPackageSpec{
			Type: "bits",
		},
	}))

	Describe("CFProcess", test(&korifiv1alpha1.CFProcess{}))

	Describe("CFRoute", test(&korifiv1alpha1.CFRoute{
		Spec: korifiv1alpha1.CFRouteSpec{
			Host: "example",
			Path: "/example",
			DomainRef: corev1.ObjectReference{
				Name: "example.com",
			},
		},
	}))

	Describe("CFSecurityGroup", test(&korifiv1alpha1.CFSecurityGroup{
		Spec: korifiv1alpha1.CFSecurityGroupSpec{
			Rules: []korifiv1alpha1.SecurityGroupRule{},
		},
	}))

	Describe("CFServiceBinding", test(&korifiv1alpha1.CFServiceBinding{
		Spec: korifiv1alpha1.CFServiceBindingSpec{
			Type: "key",
		},
	}))

	Describe("CFServiceBroker", test(&korifiv1alpha1.CFServiceBroker{}))

	Describe("CFServiceInstance", test(&korifiv1alpha1.CFServiceInstance{
		Spec: korifiv1alpha1.CFServiceInstanceSpec{
			Type: "user-provided",
		},
	}))

	Describe("CFServiceOffering", test(&korifiv1alpha1.CFServiceOffering{}))

	Describe("CFServicePlan", test(&korifiv1alpha1.CFServicePlan{
		Spec: korifiv1alpha1.CFServicePlanSpec{
			Visibility: korifiv1alpha1.ServicePlanVisibility{
				Type: korifiv1alpha1.PublicServicePlanVisibilityType,
			},
		},
	}))

	Describe("CFSpace", test(&korifiv1alpha1.CFSpace{
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: "asdf",
		},
	}))

	Describe("CFTask", test(&korifiv1alpha1.CFTask{}))
})

func test(testObj client.Object) func() {
	GinkgoHelper()

	return func() {
		var obj client.Object

		BeforeEach(func() {
			obj = testObj.DeepCopyObject().(client.Object)
			namespace := uuid.NewString()
			Expect(adminClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			})).To(Succeed())

			obj.SetName(uuid.NewString())
			obj.SetNamespace(namespace)
			Expect(adminClient.Create(ctx, obj)).To(Succeed())
		})

		It("sets the created_at label", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
				g.Expect(obj.GetLabels()).To(HaveKey(korifiv1alpha1.CreatedAtLabelKey))
				g.Expect(parseTime(obj.GetLabels()[korifiv1alpha1.CreatedAtLabelKey])).To(
					BeTemporally("~", obj.GetCreationTimestamp().Time.UTC(), 2*time.Second),
				)
			}).Should(Succeed())
		})

		It("sets the guid label", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
				g.Expect(obj.GetLabels()).To(HaveKeyWithValue(korifiv1alpha1.GUIDLabelKey, obj.GetName()))
			}).Should(Succeed())
		})

		It("does not set the updated_at label", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
				g.Expect(obj.GetLabels()).NotTo(HaveKey(korifiv1alpha1.UpdatedAtLabelKey))
			}).Should(Succeed())
		})

		When("the object is updated", func() {
			BeforeEach(func() {
				time.Sleep(1100 * time.Millisecond)
				Expect(k8s.PatchResource(ctx, adminClient, obj, func() {
					obj.GetLabels()["foo"] = "bar"
				})).To(Succeed())
			})

			It("sets the updated_at label", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())

					g.Expect(obj.GetLabels()).To(HaveKey(korifiv1alpha1.UpdatedAtLabelKey))
					g.Expect(parseTime(obj.GetLabels()[korifiv1alpha1.UpdatedAtLabelKey])).To(
						BeTemporally(">", obj.GetCreationTimestamp().Time.UTC(), 2*time.Second),
					)
				}).Should(Succeed())
			})

			When("the object has no created_at label (it has been created before this webhook has been applied)", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, adminClient, obj, func() {
						delete(obj.GetLabels(), korifiv1alpha1.CreatedAtLabelKey)
					})).To(Succeed())
				})

				It("sets the created_at label", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
						g.Expect(obj.GetLabels()).To(HaveKey(korifiv1alpha1.CreatedAtLabelKey))
						g.Expect(parseTime(obj.GetLabels()[korifiv1alpha1.CreatedAtLabelKey])).To(
							BeTemporally("~", obj.GetCreationTimestamp().Time.UTC(), 2*time.Second),
						)
					}).Should(Succeed())
				})
			})
		})
	}
}

func parseTime(s string) time.Time {
	t, err := time.Parse(korifiv1alpha1.LabelDateFormat, s)
	Expect(err).NotTo(HaveOccurred())
	return t
}
