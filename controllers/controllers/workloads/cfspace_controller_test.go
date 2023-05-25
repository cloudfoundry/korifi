package workloads_test

import (
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/pod-security-admission/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFSpaceReconciler Integration Tests", func() {
	const (
		spaceName = "my-space"
	)

	var (
		spaceGUID                                       string
		cfSpace                                         *korifiv1alpha1.CFSpace
		role                                            *rbacv1.ClusterRole
		username                                        string
		roleBinding                                     *rbacv1.RoleBinding
		roleBindingWithPropagateAnnotationSetToFalse    *rbacv1.RoleBinding
		roleBindingWithMissingPropagateAnnotation       *rbacv1.RoleBinding
		serviceAccount                                  *corev1.ServiceAccount
		serviceAccountWithPropagateAnnotationSetToFalse *corev1.ServiceAccount
		serviceAccountWithMissingPropagateAnnotation    *corev1.ServiceAccount
	)

	BeforeEach(func() {
		rules := []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create"},
				APIGroups: []string{"korifi.cloudfoundry.org"},
				Resources: []string{"cfapps"},
			},
		}
		role = createClusterRole(ctx, k8sClient, PrefixedGUID("clusterrole"), rules)

		username = PrefixedGUID("user")
		roleBinding = createRoleBinding(ctx, k8sClient, PrefixedGUID("role-binding"), username, role.Name, cfOrg.Status.GUID, map[string]string{"cloudfoundry.org/propagate-cf-role": "true"})
		roleBindingWithPropagateAnnotationSetToFalse = createRoleBinding(ctx, k8sClient, PrefixedGUID("rb-propagate-annotation-false"), username, role.Name, cfOrg.Status.GUID, map[string]string{"cloudfoundry.org/propagate-cf-role": "false"})
		roleBindingWithMissingPropagateAnnotation = createRoleBinding(ctx, k8sClient, PrefixedGUID("rb-missing-propagate-annotation"), username, role.Name, cfOrg.Status.GUID, nil)

		serviceAccount = createServiceAccount(ctx, k8sClient, PrefixedGUID("service-account"), cfRootNamespace, map[string]string{"cloudfoundry.org/propagate-service-account": "true"})
		serviceAccountWithPropagateAnnotationSetToFalse = createServiceAccount(ctx, k8sClient, PrefixedGUID("service-account-false-propagate"), cfRootNamespace, map[string]string{"cloudfoundry.org/propagate-service-account": "false"})
		serviceAccountWithMissingPropagateAnnotation = createServiceAccount(ctx, k8sClient, PrefixedGUID("service-account-no-propagate"), cfRootNamespace, nil)

		spaceGUID = PrefixedGUID("cf-space")
		cfSpace = &korifiv1alpha1.CFSpace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      spaceGUID,
				Namespace: cfOrg.Status.GUID,
			},
			Spec: korifiv1alpha1.CFSpaceSpec{
				DisplayName: spaceName,
			},
		}
	})

	When("the CFSpace is created", func() {
		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())
		})

		It("creates a namespace and sets labels", func() {
			Eventually(func(g Gomega) {
				var createdSpace corev1.Namespace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: spaceGUID}, &createdSpace)).To(Succeed())

				g.Expect(createdSpace.Labels).To(SatisfyAll(
					HaveKeyWithValue(korifiv1alpha1.SpaceNameKey, korifiv1alpha1.OrgSpaceDeprecatedName),
					HaveKeyWithValue(korifiv1alpha1.SpaceGUIDKey, spaceGUID),
				))
				g.Expect(createdSpace.Annotations).To(HaveKeyWithValue(korifiv1alpha1.SpaceNameKey, cfSpace.Spec.DisplayName))
			}).Should(Succeed())
		})

		It("sets the finalizer on cfSpace", func() {
			Eventually(func(g Gomega) []string {
				var createdCFSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Status.GUID, Name: spaceGUID}, &createdCFSpace)).To(Succeed())
				return createdCFSpace.ObjectMeta.Finalizers
			}).Should(ConsistOf([]string{
				"cfSpace.korifi.cloudfoundry.org",
			}))
		})

		It("propagates the image-registry-credentials to CFSpace", func() {
			Eventually(func(g Gomega) {
				var createdSecret corev1.Secret
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Name, Name: imageRegistrySecret.Name}, &createdSecret)).To(Succeed())

				g.Expect(createdSecret.Data).To(Equal(imageRegistrySecret.Data))
				g.Expect(createdSecret.Immutable).To(Equal(imageRegistrySecret.Immutable))
				g.Expect(createdSecret.StringData).To(Equal(imageRegistrySecret.StringData))
				g.Expect(createdSecret.Type).To(Equal(imageRegistrySecret.Type))
			}).Should(Succeed())
		})

		When("the image-registry-credentials secret does not exist in the org namespace", Serial, func() {
			BeforeEach(func() {
				Expect(k8sClient.Delete(ctx, imageRegistrySecret)).To(Succeed())

				var orgSecret corev1.Secret
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Name, Name: imageRegistrySecret.Name}, &orgSecret)).To(Succeed())
				Expect(k8sClient.Delete(ctx, &orgSecret)).To(Succeed())
			})

			AfterEach(func() {
				imageRegistrySecret = createSecret(ctx, k8sClient, packageRegistrySecretName, cfRootNamespace)

				Eventually(func(g Gomega) {
					var createdSecret corev1.Secret
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Name, Name: imageRegistrySecret.Name}, &createdSecret)).To(Succeed())
				}).Should(Succeed())
			})

			It("sets the CFSpace's Ready condition to 'False'", func() {
				Eventually(func(g Gomega) {
					var createdCFSpace korifiv1alpha1.CFSpace
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Status.GUID, Name: spaceGUID}, &createdCFSpace)).To(Succeed())

					g.Expect(meta.IsStatusConditionTrue(createdCFSpace.Status.Conditions, "Ready")).To(BeFalse())

					readyCondition := meta.FindStatusCondition(createdCFSpace.Status.Conditions, "Ready")
					g.Expect(readyCondition).NotTo(BeNil())
					g.Expect(readyCondition.Message).To(ContainSubstring(fmt.Sprintf(
						"error fetching secret %q from namespace %q",
						imageRegistrySecret.Name,
						cfOrg.Name,
					)))
					g.Expect(readyCondition.Reason).To(Equal("RegistrySecretPropagation"))
					g.Expect(readyCondition.ObservedGeneration).To(Equal(createdCFSpace.Generation))
				}, 5*time.Second).Should(Succeed())
			})
		})

		It("propagates the role-bindings with annotation \"cloudfoundry.org/propagate-cf-role\" set to \"true\" to CFSpace", func() {
			Eventually(func(g Gomega) {
				var createdRoleBindings rbacv1.RoleBindingList
				g.Expect(k8sClient.List(ctx, &createdRoleBindings, client.InNamespace(cfSpace.Name))).To(Succeed())

				g.Expect(createdRoleBindings.Items).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(roleBinding.Name),
						}),
					}),
				))
			}).Should(Succeed())
		})

		It("does not propagate role-bindings with annotation \"cloudfoundry.org/propagate-cf-role\" set to \"false\" ", func() {
			Consistently(func(g Gomega) bool {
				var newRoleBinding rbacv1.RoleBinding
				return apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Name, Name: roleBindingWithPropagateAnnotationSetToFalse.Name}, &newRoleBinding))
			}, time.Second).Should(BeTrue())
		})

		It("does not propagate role-bindings with missing annotation \"cloudfoundry.org/propagate-cf-role\" ", func() {
			Consistently(func(g Gomega) bool {
				var newRoleBinding rbacv1.RoleBinding
				return apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Name, Name: roleBindingWithMissingPropagateAnnotation.Name}, &newRoleBinding))
			}, time.Second).Should(BeTrue())
		})

		It("propagates the service accounts with annotation \"cloudfoundry.org/propagate-service-account\" set to \"true\" to CFSpace", func() {
			Eventually(func(g Gomega) {
				var createdServiceAccounts corev1.ServiceAccountList
				g.Expect(k8sClient.List(ctx, &createdServiceAccounts, client.InNamespace(cfSpace.Name))).To(Succeed())

				g.Expect(createdServiceAccounts.Items).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(serviceAccount.Name),
						}),
					}),
				))
			}).Should(Succeed())
		})

		It("removes all secret references other than the package registry secret from the propagated service account", func() {
			Eventually(func(g Gomega) {
				var createdServiceAccounts corev1.ServiceAccountList
				g.Expect(k8sClient.List(ctx, &createdServiceAccounts, client.InNamespace(cfSpace.Name))).To(Succeed())

				g.Expect(createdServiceAccounts.Items).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(serviceAccount.Name),
						}),
						"Secrets": ConsistOf(MatchFields(IgnoreExtras, Fields{"Name": Equal(packageRegistrySecretName)})),
					}),
				))
			}).Should(Succeed())
		})

		It("does not propagate service accounts with annotation \"cloudfoundry.org/propagate-service-account\" set to \"false\" ", func() {
			Consistently(func(g Gomega) bool {
				var newServiceAccount corev1.ServiceAccount
				return apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Name, Name: serviceAccountWithPropagateAnnotationSetToFalse.Name}, &newServiceAccount))
			}, time.Second).Should(BeTrue())
		})

		It("does not propagate service accounts with missing annotation \"cloudfoundry.org/propagate-service-account\" ", func() {
			Consistently(func(g Gomega) bool {
				var newServiceAccount corev1.ServiceAccount
				return apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Name, Name: serviceAccountWithMissingPropagateAnnotation.Name}, &newServiceAccount))
			}, time.Second).Should(BeTrue())
		})

		It("sets status on CFSpace", func() {
			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfSpace), &createdSpace)).To(Succeed())

				g.Expect(createdSpace.Status.GUID).To(Equal(cfSpace.Name))
				g.Expect(createdSpace.Status.ObservedGeneration).To(Equal(createdSpace.Generation))
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}).Should(Succeed())
		})

		It("sets restricted pod security labels on the namespace", func() {
			Eventually(func(g Gomega) {
				var ns corev1.Namespace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cfSpace.Name}, &ns)).To(Succeed())

				g.Expect(ns.Labels).To(HaveKeyWithValue(api.EnforceLevelLabel, string(api.LevelRestricted)))
			}).Should(Succeed())
		})
	})

	When("the CFSpace is updated after namespace modifications", func() {
		var originalSpace *korifiv1alpha1.CFSpace

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())
			originalSpace = cfSpace.DeepCopy()
			var createdNamespace corev1.Namespace
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: spaceGUID}, &createdNamespace)).To(Succeed())
			}).Should(Succeed())

			updatedNamespace := createdNamespace.DeepCopy()
			updatedNamespace.Labels["foo.com/bar"] = "42"
			updatedNamespace.Annotations["foo.com/bar"] = "43"
			Expect(k8sClient.Patch(ctx, updatedNamespace, client.MergeFrom(&createdNamespace))).To(Succeed())

			cfSpace.Spec.DisplayName += "x"
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Patch(ctx, cfSpace, client.MergeFrom(originalSpace))).To(Succeed())
		})

		It("sets the new display name annotation and preserves the added label and annoations", func() {
			Eventually(func(g Gomega) {
				var createdSpace corev1.Namespace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: spaceGUID}, &createdSpace)).To(Succeed())

				g.Expect(createdSpace.Annotations).To(HaveKeyWithValue(korifiv1alpha1.SpaceNameKey, cfSpace.Spec.DisplayName))
				g.Expect(createdSpace.Labels).To(HaveKeyWithValue("foo.com/bar", "42"))
				g.Expect(createdSpace.Annotations).To(HaveKeyWithValue("foo.com/bar", "43"))
			}).Should(Succeed())
		})
	})

	When("role-bindings are added/updated in CFOrg namespace after CFSpace creation", func() {
		var newlyCreatedRoleBinding *rbacv1.RoleBinding
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())

			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Status.GUID, Name: spaceGUID}, &createdSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			newlyCreatedRoleBinding = createRoleBinding(ctx, k8sClient, PrefixedGUID("newly-created-role-binding"), username, role.Name, cfOrg.Status.GUID, map[string]string{"cloudfoundry.org/propagate-cf-role": "true"})

			Expect(k8s.Patch(ctx, k8sClient, roleBinding, func() {
				roleBinding.SetLabels(map[string]string{"foo": "bar"})
			})).To(Succeed())
		})

		It("propagates the new role-binding to CFSpace namespace", func() {
			Eventually(func(g Gomega) {
				var createdRoleBindings rbacv1.RoleBindingList
				g.Expect(k8sClient.List(ctx, &createdRoleBindings, client.InNamespace(cfSpace.Name))).To(Succeed())

				g.Expect(createdRoleBindings.Items).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name":   Equal(roleBinding.Name),
							"Labels": HaveKeyWithValue("foo", "bar"),
						}),
					}),
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(newlyCreatedRoleBinding.Name),
						}),
					}),
				))
			}).Should(Succeed())
		})
	})

	When("service accounts are added in the root namespace after CFSpace creation", func() {
		var newlyCreatedServiceAccount *corev1.ServiceAccount
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())

			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Status.GUID, Name: spaceGUID}, &createdSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			newlyCreatedServiceAccount = createServiceAccount(ctx, k8sClient, PrefixedGUID("newly-created-service-account"), cfRootNamespace, map[string]string{"cloudfoundry.org/propagate-service-account": "true"})
			Expect(k8s.Patch(ctx, k8sClient, serviceAccount, func() {
				serviceAccount.SetLabels(map[string]string{"foo": "bar"})
			})).To(Succeed())
		})

		It("propagates the new service account to CFSpace namespace", func() {
			Eventually(func(g Gomega) {
				var createdServiceAccounts corev1.ServiceAccountList
				g.Expect(k8sClient.List(ctx, &createdServiceAccounts, client.InNamespace(cfSpace.Name))).To(Succeed())

				g.Expect(createdServiceAccounts.Items).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name":   Equal(serviceAccount.Name),
							"Labels": HaveKeyWithValue("foo", "bar"),
						}),
					}),
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(newlyCreatedServiceAccount.Name),
						}),
					}),
				))
			}).Should(Succeed())
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
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())

			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Status.GUID, Name: spaceGUID}, &createdSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			rootServiceAccount = createServiceAccount(ctx, k8sClient, PrefixedGUID("existing-service-account"), cfRootNamespace, map[string]string{"cloudfoundry.org/propagate-service-account": "true"})
			// Ensure that the service account is propagated into the CFSpace namespace
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: rootServiceAccount.Name, Namespace: cfSpace.Name}, &propagatedServiceAccount)
			}).Should(Succeed())

			// Simulate k8s adding a token secret to the propagated service account AND the propagated service account having a stale image registry credential secret
			Expect(k8s.PatchResource(ctx, k8sClient, &propagatedServiceAccount, func() {
				tokenSecretName = rootServiceAccount.Name + "-token-XYZABC"
				dockercfgSecretName = rootServiceAccount.Name + "-dockercfg-ABCXYZ"
				propagatedServiceAccount.Secrets = []corev1.ObjectReference{{Name: tokenSecretName}, {Name: dockercfgSecretName}, {Name: "out-of-date-registry-credentials"}}
				propagatedServiceAccount.ImagePullSecrets = []corev1.LocalObjectReference{{Name: dockercfgSecretName}, {Name: "out-of-date-registry-credentials"}}
			})).To(Succeed())

			// Modify the root service account to trigger reconciliation
			Expect(k8s.PatchResource(ctx, k8sClient, rootServiceAccount, func() {
				rootServiceAccount.Labels = map[string]string{"new-label": "sample-value"}
			})).To(Succeed())
		})

		It("updates the secrets on the propagated service account", func() {
			Eventually(func(g Gomega) {
				var updatedPropagatedServiceAccount corev1.ServiceAccount
				g.Expect(
					k8sClient.Get(ctx, client.ObjectKeyFromObject(&propagatedServiceAccount), &updatedPropagatedServiceAccount),
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
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())

			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Status.GUID, Name: spaceGUID}, &createdSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			rootServiceAccount = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:        PrefixedGUID("existing-service-account"),
					Namespace:   cfRootNamespace,
					Annotations: map[string]string{"cloudfoundry.org/propagate-service-account": "true"},
				},
			}

			Expect(k8sClient.Create(ctx, rootServiceAccount)).To(Succeed())

			// Ensure that the service account is propagated into the CFSpace namespace
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: rootServiceAccount.Name, Namespace: cfSpace.Name}, &propagatedServiceAccount)
			}).Should(Succeed())

			// Simulate k8s adding a token secret to the propagated service account
			Expect(k8s.PatchResource(ctx, k8sClient, &propagatedServiceAccount, func() {
				tokenSecretName = rootServiceAccount.Name + "-token-XYZABC"
				dockercfgSecretName = rootServiceAccount.Name + "-dockercfg-ABCXYZ"
				propagatedServiceAccount.Secrets = []corev1.ObjectReference{{Name: tokenSecretName}, {Name: dockercfgSecretName}}
				propagatedServiceAccount.ImagePullSecrets = []corev1.LocalObjectReference{{Name: dockercfgSecretName}}
			})).To(Succeed())

			// Add the package registry secret to the root service account
			Expect(k8s.PatchResource(ctx, k8sClient, rootServiceAccount, func() {
				rootServiceAccount.Secrets = []corev1.ObjectReference{{Name: packageRegistrySecretName}}
				rootServiceAccount.ImagePullSecrets = []corev1.LocalObjectReference{{Name: packageRegistrySecretName}}
			})).To(Succeed())
		})

		It("is also added to the space's copy of the service account", func() {
			Eventually(func(g Gomega) {
				var updatedPropagatedServiceAccount corev1.ServiceAccount
				g.Expect(
					k8sClient.Get(ctx, client.ObjectKeyFromObject(&propagatedServiceAccount), &updatedPropagatedServiceAccount),
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

	When("role bindings are deleted in the CFOrg namespace after CFSpace creation", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())

			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Status.GUID, Name: spaceGUID}, &createdSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			Expect(k8sClient.Delete(ctx, roleBinding)).To(Succeed())
		})

		It("deletes the corresponding role binding in CFSpace", func() {
			Eventually(func() bool {
				var deletedRoleBinding rbacv1.RoleBinding
				return apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: roleBinding.Name, Namespace: cfSpace.Name}, &deletedRoleBinding))
			}).Should(BeTrue(), "timed out waiting for role binding to be deleted")
		})
	})

	When("service accounts are deleted in the root namespace after CFSpace creation", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())

			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Status.GUID, Name: spaceGUID}, &createdSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			Expect(k8sClient.Delete(ctx, serviceAccount)).To(Succeed())
		})

		It("deletes the corresponding service account in CFSpace", func() {
			Eventually(func() bool {
				var deletedServiceAccount corev1.ServiceAccount
				return apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: serviceAccount.Name, Namespace: cfSpace.Name}, &deletedServiceAccount))
			}).Should(BeTrue(), "timed out waiting for service account to be deleted")
		})
	})

	When("the CFSpace is deleted", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())

			Eventually(func(g Gomega) {
				var spaceNamespace corev1.Namespace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: spaceGUID}, &spaceNamespace)).To(Succeed())
			}).Should(Succeed())

			Expect(k8sClient.Delete(ctx, cfSpace)).To(Succeed())
		})

		It("eventually deletes the namespace", func() {
			// Envtests do not cleanup namespaces. For testing, we check for deletion timestamps on namespace.
			Eventually(func(g Gomega) bool {
				var spaceNamespace corev1.Namespace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: spaceGUID}, &spaceNamespace)).To(Succeed())

				return spaceNamespace.GetDeletionTimestamp().IsZero()
			}).Should(BeFalse(), "timed out waiting for deletion timestamps to be set on namespace")
		})
	})
})
