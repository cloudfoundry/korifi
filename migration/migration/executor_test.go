package migration_test

import (
	"crypto/sha1"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	bindingswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/services/bindings"
	"code.cloudfoundry.org/korifi/migration/migration"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Executor", func() {
	Describe("CFServiceBinding Lease", func() {
		var (
			serviceBinding       *korifiv1alpha1.CFServiceBinding
			migrator             *migration.Migrator
			existingLease        *coordinationv1.Lease
			bindingsNameRegistry coordination.NameRegistry
		)

		BeforeEach(func() {
			bindingsNameRegistry = coordination.NewNameRegistry(k8sClient, bindingswebhook.ServiceBindingEntityType)
			migrator = migration.New(k8sClient, "test-version", 1, bindingsNameRegistry)

			serviceBinding = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Type: "app",
					Service: corev1.ObjectReference{
						Kind: "CFServiceInstance",
						Name: uuid.NewString(),
					},
					AppRef: corev1.LocalObjectReference{
						Name: uuid.NewString(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, serviceBinding)).To(Succeed())

			existingLeaseName := fmt.Sprintf("servicebinding::sb::%s::%s::%s", serviceBinding.Spec.AppRef.Name, serviceBinding.Spec.Service.Namespace, serviceBinding.Spec.Service.Name)
			existingLease = &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      fmt.Sprintf("n-%x", sha1.Sum([]byte(existingLeaseName))),
				},
			}
			Expect(k8sClient.Create(ctx, existingLease)).To(Succeed())
		})

		JustBeforeEach(func() {
			err := migrator.Run(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("sets the migrated-by label", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceBinding), serviceBinding)).To(Succeed())
				g.Expect(serviceBinding.Labels).To(HaveKeyWithValue(migration.MigratedByLabelKey, "test-version"))
			}).Should(Succeed())
		})

		It("deletes the existing lease", func() {
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(existingLease), existingLease)
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})

		It("creates a new lease in the new format by using the service name", func() {
			leaseName := fmt.Sprintf("servicebinding::sb::%s::%s::%s::%s", serviceBinding.Spec.AppRef.Name, serviceBinding.Namespace, serviceBinding.Spec.Service.Name, serviceBinding.Spec.Service.Name)
			newLease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      fmt.Sprintf("n-%x", sha1.Sum([]byte(leaseName))),
				},
			}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(newLease), newLease)).To(Succeed())
				g.Expect(newLease.Annotations).To(SatisfyAll(
					HaveKeyWithValue(coordination.EntityTypeAnnotation, bindingswebhook.ServiceBindingEntityType),
					HaveKeyWithValue(coordination.NameAnnotation, fmt.Sprintf("sb::%s::%s::%s::%s", serviceBinding.Spec.AppRef.Name, serviceBinding.Namespace, serviceBinding.Spec.Service.Name, serviceBinding.Spec.Service.Name)),
					HaveKeyWithValue(coordination.NamespaceAnnotation, serviceBinding.Namespace),
					HaveKeyWithValue(coordination.OwnerNamespaceAnnotation, serviceBinding.Namespace),
					HaveKeyWithValue(coordination.OwnerNameAnnotation, serviceBinding.Name),
				))
			}).Should(Succeed())
		})

		When("the service binding has a display name", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, serviceBinding, func() {
					serviceBinding.Spec.DisplayName = tools.PtrTo(uuid.NewString())
				})).To(Succeed())
			})

			It("creates a new lease in the new format by using the binding display name", func() {
				leaseName := fmt.Sprintf("servicebinding::sb::%s::%s::%s::%s", serviceBinding.Spec.AppRef.Name, serviceBinding.Namespace, serviceBinding.Spec.Service.Name, *serviceBinding.Spec.DisplayName)
				newLease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      fmt.Sprintf("n-%x", sha1.Sum([]byte(leaseName))),
					},
				}
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(newLease), newLease)).To(Succeed())
					g.Expect(newLease.Annotations).To(SatisfyAll(
						HaveKeyWithValue(coordination.EntityTypeAnnotation, bindingswebhook.ServiceBindingEntityType),
						HaveKeyWithValue(coordination.NameAnnotation, fmt.Sprintf("sb::%s::%s::%s::%s", serviceBinding.Spec.AppRef.Name, serviceBinding.Namespace, serviceBinding.Spec.Service.Name, *serviceBinding.Spec.DisplayName)),
						HaveKeyWithValue(coordination.NamespaceAnnotation, serviceBinding.Namespace),
						HaveKeyWithValue(coordination.OwnerNamespaceAnnotation, serviceBinding.Namespace),
						HaveKeyWithValue(coordination.OwnerNameAnnotation, serviceBinding.Name),
					))
				}).Should(Succeed())
			})
		})

		When("the binding unique name has been already registered for that binding", func() {
			BeforeEach(func() {
				Expect(bindingsNameRegistry.RegisterName(ctx, serviceBinding.Namespace, serviceBinding.UniqueName(), serviceBinding.Namespace, serviceBinding.Name)).To(Succeed())
			})

			It("preserves the new unique name lease", func() {
				leaseName := fmt.Sprintf("servicebinding::sb::%s::%s::%s::%s", serviceBinding.Spec.AppRef.Name, serviceBinding.Namespace, serviceBinding.Spec.Service.Name, serviceBinding.Spec.Service.Name)
				newLease := &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      fmt.Sprintf("n-%x", sha1.Sum([]byte(leaseName))),
					},
				}
				Consistently(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(newLease), newLease)).To(Succeed())
				}).Should(Succeed())
			})

			It("deletes the existing lease", func() {
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(existingLease), existingLease)
					g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})
		})

		When("the binding has been already migrated by the current version", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, serviceBinding, func() {
					serviceBinding.Labels = map[string]string{
						migration.MigratedByLabelKey: "test-version",
					}
				})).To(Succeed())
			})

			It("does not migrate again", func() {
				Consistently(func(g Gomega) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(existingLease), existingLease)
					g.Expect(err).NotTo(HaveOccurred())
				}).Should(Succeed())
			})
		})

		When("the binding has been migrated by another version", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, serviceBinding, func() {
					serviceBinding.Labels = map[string]string{
						migration.MigratedByLabelKey: "another-version",
					}
				})).To(Succeed())
			})

			It("migrates the binding", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceBinding), serviceBinding)).To(Succeed())
					g.Expect(serviceBinding.Labels).To(HaveKeyWithValue(migration.MigratedByLabelKey, "test-version"))
				}).Should(Succeed())
			})
		})
	})
})
