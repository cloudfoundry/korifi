package integration_test

import (
	"context"
	"fmt"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFAppValidatingWebhook", func() {
	var (
		ctx        context.Context
		namespace1 string
		namespace2 string
		app1Guid   string
		app2Guid   string
		app1Name   string
		app2Name   string
		app1       *korifiv1alpha1.CFApp
	)

	BeforeEach(func() {
		ctx = context.Background()

		namespace1 = "ns-1-" + uuid.NewString()
		namespace2 = "ns-2-" + uuid.NewString()

		Expect(k8sClient.Create(ctx, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace1,
			},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace2,
			},
		})).To(Succeed())

		app1Guid = "guid-1-" + uuid.NewString()
		app2Guid = "guid-2-" + uuid.NewString()
		app1Name = "name-1-" + uuid.NewString()
		app2Name = "name-2-" + uuid.NewString()
	})

	Describe("Create", func() {
		var createErr error

		BeforeEach(func() {
			app1 = makeCFApp(app1Guid, namespace1, app1Name)
		})

		JustBeforeEach(func() {
			createErr = k8sClient.Create(ctx, app1)
		})

		It("should succeed", func() {
			Expect(createErr).NotTo(HaveOccurred())
		})

		When("another CFApp exists with a different name in the same namespace", func() {
			BeforeEach(func() {
				app2 := makeCFApp(app2Guid, namespace1, app2Name)
				Expect(k8sClient.Create(ctx, app2)).To(Succeed())
			})

			It("should succeed", func() {
				Expect(createErr).NotTo(HaveOccurred())
			})
		})

		When("another CFApp exists with the same name in a different namespace", func() {
			BeforeEach(func() {
				app2 := makeCFApp(app2Guid, namespace2, app1Name)
				Expect(k8sClient.Create(ctx, app2)).To(Succeed())
			})

			It("should succeed", func() {
				Expect(createErr).NotTo(HaveOccurred())
			})
		})

		When("another CFApp exists with the same name in the same namespace", func() {
			BeforeEach(func() {
				app2 := makeCFApp(app2Guid, namespace1, app1Name)
				Expect(k8sClient.Create(ctx, app2)).To(Succeed())
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring("App with the name '%s' already exists.", app1Name)))
			})
		})

		When("another CFApp exists with the same name(case insensitive) in the same namespace", func() {
			BeforeEach(func() {
				app2 := makeCFApp(app2Guid, namespace1, strings.ToUpper(app1Name))
				Expect(
					k8sClient.Create(ctx, app2),
				).To(Succeed())
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring(fmt.Sprintf("App with the name '%s' already exists.", app1Name))))
			})
		})
	})

	Describe("Update", func() {
		var updateErr error

		BeforeEach(func() {
			app1 = makeCFApp(app1Guid, namespace1, app1Name)
			Expect(k8sClient.Create(ctx, app1)).To(Succeed())
		})

		JustBeforeEach(func() {
			updateErr = k8sClient.Update(context.Background(), app1)
		})

		When("changing the name", func() {
			var newName string

			BeforeEach(func() {
				newName = uuid.NewString()
				app1.Spec.DisplayName = newName
			})

			It("should succeed", func() {
				Expect(updateErr).NotTo(HaveOccurred())

				app1Actual := korifiv1alpha1.CFApp{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(app1), &app1Actual)).To(Succeed())
				Expect(app1Actual.Spec.DisplayName).To(Equal(newName))
			})

			When("reusing an old name", func() {
				It("allows creating another app with the old name", func() {
					Expect(updateErr).NotTo(HaveOccurred())

					reuseOldNameApp := makeCFApp(uuid.NewString(), namespace1, app1Name)
					Expect(k8sClient.Create(ctx, reuseOldNameApp)).To(Succeed())
				})
			})
		})

		When("not changing the name", func() {
			BeforeEach(func() {
				app1.Spec.DesiredState = korifiv1alpha1.StartedState
			})

			It("should succeed", func() {
				Expect(updateErr).NotTo(HaveOccurred())

				app1Actual := korifiv1alpha1.CFApp{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(app1), &app1Actual)).To(Succeed())
				Expect(app1Actual.Spec.DesiredState).To(Equal(korifiv1alpha1.StartedState))
			})
		})

		When("modifying spec.DisplayName to match another CFApp spec.DisplayName", func() {
			BeforeEach(func() {
				app2 := makeCFApp(app2Guid, namespace1, app2Name)
				app1.Spec.DisplayName = app2Name
				Expect(k8sClient.Create(ctx, app2)).To(Succeed())
			})

			It("should fail", func() {
				Expect(updateErr).To(MatchError(ContainSubstring(fmt.Sprintf("App with the name '%s' already exists.", app2Name))))
			})
		})
	})

	Describe("Delete", func() {
		var deleteErr error

		BeforeEach(func() {
			app1 = makeCFApp(app1Guid, namespace1, app1Name)
			Expect(k8sClient.Create(ctx, app1)).To(Succeed())
		})

		JustBeforeEach(func() {
			deleteErr = k8sClient.Delete(ctx, app1)
		})

		It("succeeds", func() {
			Expect(deleteErr).NotTo(HaveOccurred())
		})
	})
})

func makeCFApp(cfAppGUID string, namespace string, name string) *korifiv1alpha1.CFApp {
	return &korifiv1alpha1.CFApp{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CFApp",
			APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfAppGUID,
			Namespace: namespace,
		},
		Spec: korifiv1alpha1.CFAppSpec{
			DisplayName:  name,
			DesiredState: "STOPPED",
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}
}
