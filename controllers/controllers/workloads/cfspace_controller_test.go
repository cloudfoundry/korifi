package workloads_test

import (
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/pod-security-admission/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFSpaceReconciler Integration Tests", func() {
	var (
		spaceGUID string
		cfSpace   *korifiv1alpha1.CFSpace
	)

	BeforeEach(func() {
		spaceGUID = PrefixedGUID("cf-space")
		cfSpace = &korifiv1alpha1.CFSpace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      spaceGUID,
				Namespace: testOrg.Status.GUID,
			},
			Spec: korifiv1alpha1.CFSpaceSpec{
				DisplayName: uuid.NewString(),
			},
		}
		Expect(adminClient.Create(ctx, cfSpace)).To(Succeed())
	})

	It("creates a namespace and sets labels", func() {
		Eventually(func(g Gomega) {
			var ns corev1.Namespace
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: spaceGUID}, &ns)).To(Succeed())

			g.Expect(ns.Labels).To(SatisfyAll(
				HaveKeyWithValue(korifiv1alpha1.SpaceNameKey, korifiv1alpha1.OrgSpaceDeprecatedName),
				HaveKeyWithValue(korifiv1alpha1.SpaceGUIDKey, spaceGUID),
				HaveKeyWithValue(api.EnforceLevelLabel, string(api.LevelRestricted)),
			))
			g.Expect(ns.Annotations).To(HaveKeyWithValue(korifiv1alpha1.SpaceNameKey, cfSpace.Spec.DisplayName))
		}).Should(Succeed())
	})

	It("sets the finalizer on cfSpace", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfSpace), cfSpace)).To(Succeed())
			g.Expect(cfSpace.ObjectMeta.Finalizers).To(ConsistOf("cfSpace.korifi.cloudfoundry.org"))
		}).Should(Succeed())
	})

	It("propagates the image-registry-credentials secrets to CFSpace", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Name, Name: imageRegistrySecret.Name}, &corev1.Secret{})).To(Succeed())
		}).Should(Succeed())
	})

	It("sets status on CFSpace", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfSpace), cfSpace)).To(Succeed())

			g.Expect(cfSpace.Status.GUID).To(Equal(cfSpace.Name))
			g.Expect(cfSpace.Status.ObservedGeneration).To(Equal(cfSpace.Generation))
			g.Expect(meta.IsStatusConditionTrue(cfSpace.Status.Conditions, "Ready")).To(BeTrue())
		}).Should(Succeed())
	})

	Describe("service account propagation", func() {
		var serviceAccount *corev1.ServiceAccount

		BeforeEach(func() {
			serviceAccountName := PrefixedGUID("service-account")
			serviceAccount = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: cfRootNamespace,
					Annotations: map[string]string{
						"kapp.k14s.io/baz": "foo",
						"meta.helm.sh/foo": "bar",
					},
				},
				Secrets: []corev1.ObjectReference{
					{Name: serviceAccountName + "-token-someguid"},
					{Name: serviceAccountName + "-dockercfg-someguid"},
					{Name: packageRegistrySecretName},
				},
				ImagePullSecrets: []corev1.LocalObjectReference{
					{Name: serviceAccountName + "-dockercfg-someguid"},
					{Name: packageRegistrySecretName},
				},
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, serviceAccount)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfSpace), cfSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(cfSpace.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())
		})

		It("does not propagate service accounts with missing annotation \"cloudfoundry.org/propagate-service-account\" ", func() {
			Consistently(func(g Gomega) {
				g.Expect(apierrors.IsNotFound(adminClient.Get(
					ctx,
					types.NamespacedName{Namespace: cfSpace.Name, Name: serviceAccount.Name},
					&corev1.ServiceAccount{},
				))).To(BeTrue())
			}, time.Second).Should(Succeed())
		})

		When("the service account is annotated as \"cloudfoundry.org/propagate-service-account\" set to \"false\"", func() {
			BeforeEach(func() {
				serviceAccount.Annotations = map[string]string{
					korifiv1alpha1.PropagateServiceAccountAnnotation: "false",
				}
			})

			It("does not propagate it ", func() {
				Consistently(func(g Gomega) {
					g.Expect(apierrors.IsNotFound(adminClient.Get(
						ctx,
						types.NamespacedName{Namespace: cfSpace.Name, Name: serviceAccount.Name},
						&corev1.ServiceAccount{},
					))).To(BeTrue())
				}, time.Second).Should(Succeed())
			})
		})

		When("the service account is annotated as \"cloudfoundry.org/propagate-service-account\" set to \"true\"", func() {
			BeforeEach(func() {
				serviceAccount.Annotations = map[string]string{
					korifiv1alpha1.PropagateServiceAccountAnnotation: "true",
				}
			})

			It("propagates it", func() {
				var createdServiceAccount corev1.ServiceAccount

				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Name, Name: serviceAccount.Name}, &createdServiceAccount)).To(Succeed())
				}).Should(Succeed())

				By("omitting annotations from deployment tools", func() {
					Expect(maps.Keys(createdServiceAccount.Annotations)).To(ConsistOf("cloudfoundry.org/propagate-service-account"))
					Expect(createdServiceAccount.Annotations["cloudfoundry.org/propagate-service-account"]).To(Equal("true"))
				})
			})

			It("removes all secret references other than the package registry secret from the propagated service account", func() {
				Eventually(func(g Gomega) {
					var createdServiceAccounts corev1.ServiceAccountList
					g.Expect(adminClient.List(ctx, &createdServiceAccounts, client.InNamespace(cfSpace.Name))).To(Succeed())

					g.Expect(createdServiceAccounts.Items).To(ContainElements(
						MatchFields(IgnoreExtras, Fields{
							"ObjectMeta": MatchFields(IgnoreExtras, Fields{
								"Name": Equal(serviceAccount.Name),
							}),
							"Secrets": ConsistOf(
								MatchFields(IgnoreExtras, Fields{"Name": Equal(packageRegistrySecretName)}),
							),
						}),
					))
				}).Should(Succeed())
			})

			When("propagatable service accounts are deleted in the root namespace", func() {
				JustBeforeEach(func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Name, Name: serviceAccount.Name}, &corev1.ServiceAccount{})).To(Succeed())
					}).Should(Succeed())

					Expect(adminClient.Delete(ctx, serviceAccount)).To(Succeed())
				})

				It("deletes the corresponding service account in CFSpace", func() {
					Eventually(func() bool {
						return apierrors.IsNotFound(adminClient.Get(ctx, types.NamespacedName{Name: serviceAccount.Name, Namespace: cfSpace.Name}, new(corev1.ServiceAccount)))
					}).Should(BeTrue(), "timed out waiting for service account to be deleted")
				})

				When("the service account is annotated not to propagate deletions", func() {
					BeforeEach(func() {
						serviceAccount.Annotations[korifiv1alpha1.PropagateDeletionAnnotation] = "false"
					})

					It("doesn't delete the corresponding service account in the CFSpace", func() {
						Consistently(func() bool {
							return apierrors.IsNotFound(adminClient.Get(ctx, types.NamespacedName{Name: serviceAccount.Name, Namespace: cfSpace.Name}, new(corev1.ServiceAccount)))
						}).Should(BeFalse(), "space's copy of service account was deleted and shouldn't have been")
					})
				})
			})
		})
	})

	When("service accounts are updated in the root namespace after CFSpace creation", func() {
		var (
			rootServiceAccount       *corev1.ServiceAccount
			propagatedServiceAccount corev1.ServiceAccount
			tokenSecretName          string
			dockercfgSecretName      string
		)

		BeforeEach(func() {
			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: testOrg.Status.GUID, Name: spaceGUID}, &createdSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			rootServiceAccount = createServiceAccount(ctx, PrefixedGUID("existing-service-account"), cfRootNamespace, map[string]string{"cloudfoundry.org/propagate-service-account": "true"})
			// Ensure that the service account is propagated into the CFSpace namespace
			Eventually(func() error {
				return adminClient.Get(ctx, types.NamespacedName{Name: rootServiceAccount.Name, Namespace: cfSpace.Name}, &propagatedServiceAccount)
			}).Should(Succeed())

			// Simulate k8s adding a token secret to the propagated service account AND the propagated service account having a stale image registry credential secret
			Expect(k8s.PatchResource(ctx, adminClient, &propagatedServiceAccount, func() {
				tokenSecretName = rootServiceAccount.Name + "-token-XYZABC"
				dockercfgSecretName = rootServiceAccount.Name + "-dockercfg-ABCXYZ"
				propagatedServiceAccount.Secrets = []corev1.ObjectReference{{Name: tokenSecretName}, {Name: dockercfgSecretName}, {Name: "out-of-date-registry-credentials"}}
				propagatedServiceAccount.ImagePullSecrets = []corev1.LocalObjectReference{{Name: dockercfgSecretName}, {Name: "out-of-date-registry-credentials"}}
			})).To(Succeed())

			// Modify the root service account to trigger reconciliation
			Expect(k8s.PatchResource(ctx, adminClient, rootServiceAccount, func() {
				rootServiceAccount.Labels = map[string]string{"new-label": "sample-value"}
			})).To(Succeed())
		})

		It("updates the secrets on the propagated service account", func() {
			Eventually(func(g Gomega) {
				var updatedPropagatedServiceAccount corev1.ServiceAccount
				g.Expect(
					adminClient.Get(ctx, client.ObjectKeyFromObject(&propagatedServiceAccount), &updatedPropagatedServiceAccount),
				).To(Succeed())
				g.Expect(updatedPropagatedServiceAccount.Secrets).To(ConsistOf(
					corev1.ObjectReference{Name: tokenSecretName},
					corev1.ObjectReference{Name: dockercfgSecretName},
					corev1.ObjectReference{Name: packageRegistrySecretName},
				))

				g.Expect(updatedPropagatedServiceAccount.ImagePullSecrets).To(ConsistOf(
					corev1.LocalObjectReference{Name: dockercfgSecretName},
					corev1.LocalObjectReference{Name: packageRegistrySecretName},
				))
			}).Should(Succeed())
		})
	})

	When("the package registry secret is added to the root service account", func() {
		var (
			rootServiceAccount       *corev1.ServiceAccount
			propagatedServiceAccount corev1.ServiceAccount
			tokenSecretName          string
			dockercfgSecretName      string
		)

		BeforeEach(func() {
			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: testOrg.Status.GUID, Name: spaceGUID}, &createdSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			rootServiceAccount = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:        PrefixedGUID("existing-service-account"),
					Namespace:   cfRootNamespace,
					Annotations: map[string]string{"cloudfoundry.org/propagate-service-account": "true"},
				},
			}

			Expect(adminClient.Create(ctx, rootServiceAccount)).To(Succeed())

			// Ensure that the service account is propagated into the CFSpace namespace
			Eventually(func() error {
				return adminClient.Get(ctx, types.NamespacedName{Name: rootServiceAccount.Name, Namespace: cfSpace.Name}, &propagatedServiceAccount)
			}).Should(Succeed())

			// Simulate k8s adding a token secret to the propagated service account
			Expect(k8s.PatchResource(ctx, adminClient, &propagatedServiceAccount, func() {
				tokenSecretName = rootServiceAccount.Name + "-token-XYZABC"
				dockercfgSecretName = rootServiceAccount.Name + "-dockercfg-ABCXYZ"
				propagatedServiceAccount.Secrets = []corev1.ObjectReference{{Name: tokenSecretName}, {Name: dockercfgSecretName}}
				propagatedServiceAccount.ImagePullSecrets = []corev1.LocalObjectReference{{Name: dockercfgSecretName}}
			})).To(Succeed())

			// Add the package registry secret to the root service account
			Expect(k8s.PatchResource(ctx, adminClient, rootServiceAccount, func() {
				rootServiceAccount.Secrets = []corev1.ObjectReference{{Name: packageRegistrySecretName}}
				rootServiceAccount.ImagePullSecrets = []corev1.LocalObjectReference{{Name: packageRegistrySecretName}}
			})).To(Succeed())
		})

		It("is also added to the space's copy of the service account", func() {
			Eventually(func(g Gomega) {
				var updatedPropagatedServiceAccount corev1.ServiceAccount
				g.Expect(
					adminClient.Get(ctx, client.ObjectKeyFromObject(&propagatedServiceAccount), &updatedPropagatedServiceAccount),
				).To(Succeed())
				g.Expect(updatedPropagatedServiceAccount.Secrets).To(ConsistOf(
					corev1.ObjectReference{Name: tokenSecretName},
					corev1.ObjectReference{Name: dockercfgSecretName},
					corev1.ObjectReference{Name: packageRegistrySecretName},
				))
				g.Expect(updatedPropagatedServiceAccount.ImagePullSecrets).To(ConsistOf(
					corev1.LocalObjectReference{Name: dockercfgSecretName},
					corev1.LocalObjectReference{Name: packageRegistrySecretName},
				))
			}).Should(Succeed())
		})
	})
})
