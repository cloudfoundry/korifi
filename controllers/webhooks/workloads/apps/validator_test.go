package apps_test

import (
	"context"
	"fmt"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFAppValidatingWebhook", func() {
	var (
		app       *korifiv1alpha1.CFApp
		createErr error
	)

	BeforeEach(func() {
		app = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  "app-" + uuid.NewString(),
				DesiredState: "STOPPED",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
				},
			},
		}
	})

	JustBeforeEach(func() {
		createErr = adminClient.Create(ctx, app)
	})

	Describe("Create", func() {
		It("should succeed", func() {
			Expect(createErr).NotTo(HaveOccurred())
		})

		When("another CFApp exists with a different name in the same namespace", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &korifiv1alpha1.CFApp{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: testNamespace,
					},
					Spec: korifiv1alpha1.CFAppSpec{
						DisplayName:  uuid.NewString(),
						DesiredState: "STOPPED",
						Lifecycle: korifiv1alpha1.Lifecycle{
							Type: "buildpack",
						},
					},
				})).To(Succeed())
			})

			It("should succeed", func() {
				Expect(createErr).NotTo(HaveOccurred())
			})
		})

		When("another CFApp exists with the same name in a different namespace", func() {
			BeforeEach(func() {
				anotherNamespace := uuid.NewString()
				Expect(adminClient.Create(ctx, &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: anotherNamespace,
					},
				})).To(Succeed())

				Expect(adminClient.Create(ctx, &korifiv1alpha1.CFApp{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: anotherNamespace,
					},
					Spec: korifiv1alpha1.CFAppSpec{
						DisplayName:  app.Spec.DisplayName,
						DesiredState: "STOPPED",
						Lifecycle: korifiv1alpha1.Lifecycle{
							Type: "buildpack",
						},
					},
				})).To(Succeed())
			})

			It("should succeed", func() {
				Expect(createErr).NotTo(HaveOccurred())
			})
		})

		When("another CFApp exists with the same name in the same namespace", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &korifiv1alpha1.CFApp{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: testNamespace,
					},
					Spec: korifiv1alpha1.CFAppSpec{
						DisplayName:  app.Spec.DisplayName,
						DesiredState: "STOPPED",
						Lifecycle: korifiv1alpha1.Lifecycle{
							Type: "buildpack",
						},
					},
				})).To(Succeed())
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring("App with the name '%s' already exists.", app.Spec.DisplayName)))
			})
		})

		When("another CFApp exists with the same name(case insensitive) in the same namespace", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &korifiv1alpha1.CFApp{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: testNamespace,
					},
					Spec: korifiv1alpha1.CFAppSpec{
						DisplayName:  strings.ToUpper(app.Spec.DisplayName),
						DesiredState: "STOPPED",
						Lifecycle: korifiv1alpha1.Lifecycle{
							Type: "buildpack",
						},
					},
				})).To(Succeed())
			})

			It("should fail", func() {
				Expect(createErr).To(MatchError(ContainSubstring(fmt.Sprintf("App with the name '%s' already exists.", app.Spec.DisplayName))))
			})
		})
	})

	Describe("Update", func() {
		var updateErr error

		Describe("changing the name", func() {
			var newName string

			BeforeEach(func() {
				newName = uuid.NewString()
			})

			JustBeforeEach(func() {
				updateErr = k8s.Patch(ctx, adminClient, app, func() {
					app.Spec.DisplayName = newName
				})
			})

			It("should succeed", func() {
				Expect(updateErr).NotTo(HaveOccurred())

				Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(app), app)).To(Succeed())
				Expect(app.Spec.DisplayName).To(Equal(newName))
			})

			When("reusing an old name", func() {
				var oldName string

				BeforeEach(func() {
					oldName = app.Spec.DisplayName
				})

				It("allows creating another app with the old name", func() {
					Expect(updateErr).NotTo(HaveOccurred())

					Expect(adminClient.Create(ctx, &korifiv1alpha1.CFApp{
						ObjectMeta: metav1.ObjectMeta{
							Name:      uuid.NewString(),
							Namespace: testNamespace,
						},
						Spec: korifiv1alpha1.CFAppSpec{
							DisplayName:  oldName,
							DesiredState: "STOPPED",
							Lifecycle: korifiv1alpha1.Lifecycle{
								Type: "buildpack",
							},
						},
					})).To(Succeed())
				})
			})

			When("an app with the new name already exists", func() {
				BeforeEach(func() {
					Expect(adminClient.Create(ctx, &korifiv1alpha1.CFApp{
						ObjectMeta: metav1.ObjectMeta{
							Name:      uuid.NewString(),
							Namespace: testNamespace,
						},
						Spec: korifiv1alpha1.CFAppSpec{
							DisplayName:  newName,
							DesiredState: "STOPPED",
							Lifecycle: korifiv1alpha1.Lifecycle{
								Type: "buildpack",
							},
						},
					})).To(Succeed())
				})

				It("should fail", func() {
					Expect(updateErr).To(MatchError(ContainSubstring(fmt.Sprintf("App with the name '%s' already exists.", newName))))
				})
			})
		})

		Describe("not changing the name", func() {
			JustBeforeEach(func() {
				updateErr = k8s.Patch(ctx, adminClient, app, func() {
					app.Spec.DesiredState = korifiv1alpha1.StartedState
				})
			})

			It("should succeed", func() {
				Expect(updateErr).NotTo(HaveOccurred())

				Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(app), app)).To(Succeed())
				Expect(app.Spec.DesiredState).To(Equal(korifiv1alpha1.StartedState))
			})
		})

		Describe("changing the lifecycle type", func() {
			JustBeforeEach(func() {
				updateErr = k8s.Patch(ctx, adminClient, app, func() {
					app.Spec.Lifecycle.Type = korifiv1alpha1.LifecycleType("docker")
				})
			})

			It("should fail", func() {
				Expect(updateErr).To(MatchError(ContainSubstring("cannot be changed from buildpack to docker")))
			})
		})
	})

	Describe("Delete", func() {
		var deleteErr error

		JustBeforeEach(func() {
			deleteErr = adminNonSyncClient.Delete(ctx, app)
		})

		It("succeeds", func() {
			Expect(deleteErr).NotTo(HaveOccurred())
		})
	})
})
