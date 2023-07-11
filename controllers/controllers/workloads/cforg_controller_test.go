package workloads_test

import (
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gstruct"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/pod-security-admission/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFOrgReconciler Integration Tests", func() {
	var (
		orgGUID                                      string
		cfOrg                                        korifiv1alpha1.CFOrg
		role                                         *rbacv1.ClusterRole
		username                                     string
		roleBinding                                  *rbacv1.RoleBinding
		roleBindingWithPropagateAnnotationSetToFalse *rbacv1.RoleBinding
		roleBindingWithMissingPropagateAnnotation    *rbacv1.RoleBinding
	)

	BeforeEach(func() {
		rules := []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create"},
				APIGroups: []string{"korifi.cloudfoundry.org"},
				Resources: []string{"cfapps"},
			},
		}
		role = createClusterRole(ctx, adminClient, PrefixedGUID("clusterrole"), rules)

		username = PrefixedGUID("user")
		roleBinding = createRoleBinding(ctx, adminClient, PrefixedGUID("role-binding"), username, role.Name, cfRootNamespace, map[string]string{"cloudfoundry.org/propagate-cf-role": "true"})

		username2 := PrefixedGUID("user2")
		roleBindingWithPropagateAnnotationSetToFalse = createRoleBinding(ctx, adminClient, PrefixedGUID("rb-propagate-annotation-false"), username2, role.Name, cfRootNamespace, map[string]string{"cloudfoundry.org/propagate-cf-role": "false"})

		roleBindingWithMissingPropagateAnnotation = createRoleBinding(ctx, adminClient, PrefixedGUID("rb-missing-propagate-annotation"), username2, role.Name, cfRootNamespace, nil)

		orgGUID = PrefixedGUID("cf-org")
		cfOrg = korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgGUID,
				Namespace: cfRootNamespace,
			},
			Spec: korifiv1alpha1.CFOrgSpec{
				DisplayName: uuid.NewString(),
			},
		}
	})

	When("the CFOrg is created", func() {
		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, &cfOrg)).To(Succeed())
		})

		It("creates an org namespace and sets labels", func() {
			Eventually(func(g Gomega) {
				var orgNamespace v1.Namespace
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: orgGUID}, &orgNamespace)).To(Succeed())

				g.Expect(orgNamespace.Labels).To(SatisfyAll(
					HaveKeyWithValue(korifiv1alpha1.OrgNameKey, korifiv1alpha1.OrgSpaceDeprecatedName),
					HaveKeyWithValue(korifiv1alpha1.OrgGUIDKey, orgGUID),
				))
				g.Expect(orgNamespace.Annotations).To(HaveKeyWithValue(korifiv1alpha1.OrgNameKey, cfOrg.Spec.DisplayName))
			}).Should(Succeed())
		})

		It("sets the finalizer on cfOrg", func() {
			Eventually(func(g Gomega) []string {
				var createdCFOrg korifiv1alpha1.CFOrg
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfRootNamespace, Name: orgGUID}, &createdCFOrg)).To(Succeed())
				return createdCFOrg.ObjectMeta.Finalizers
			}).Should(ConsistOf([]string{
				"cfOrg.korifi.cloudfoundry.org",
			}))
		})

		It("propagates the image-registry-credentials secrets from root-ns to org namespace", func() {
			var createdSecret1, createdSecret2 v1.Secret

			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Name, Name: imageRegistrySecret1.Name}, &createdSecret1)).To(Succeed())
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Name, Name: imageRegistrySecret2.Name}, &createdSecret2)).To(Succeed())
			}).Should(Succeed())

			Expect(createdSecret1.Data).To(Equal(imageRegistrySecret1.Data))
			Expect(createdSecret1.Immutable).To(Equal(imageRegistrySecret1.Immutable))
			Expect(createdSecret1.StringData).To(Equal(imageRegistrySecret1.StringData))
			Expect(createdSecret1.Type).To(Equal(imageRegistrySecret1.Type))

			Expect(createdSecret2.Data).To(Equal(imageRegistrySecret2.Data))
			Expect(createdSecret2.Immutable).To(Equal(imageRegistrySecret2.Immutable))
			Expect(createdSecret2.StringData).To(Equal(imageRegistrySecret2.StringData))
			Expect(createdSecret2.Type).To(Equal(imageRegistrySecret2.Type))
		})

		When("the image-registry-credentials secret does not exist in the root-ns", Serial, func() {
			BeforeEach(func() {
				Expect(adminClient.Delete(ctx, imageRegistrySecret1)).To(Succeed())
			})

			AfterEach(func() {
				imageRegistrySecret1 = createImageRegistrySecret(ctx, adminClient, packageRegistrySecretName, cfRootNamespace)
			})

			It("sets the CFOrg's Ready condition to 'False'", func() {
				Eventually(func(g Gomega) {
					var createdOrg korifiv1alpha1.CFOrg
					g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfRootNamespace, Name: orgGUID}, &createdOrg)).To(Succeed())

					g.Expect(meta.IsStatusConditionTrue(createdOrg.Status.Conditions, "Ready")).To(BeFalse())

					readyCondition := meta.FindStatusCondition(createdOrg.Status.Conditions, "Ready")
					g.Expect(readyCondition).NotTo(BeNil())
					g.Expect(readyCondition.Message).To(ContainSubstring(fmt.Sprintf(
						"error fetching secret %q from namespace %q",
						imageRegistrySecret1.Name,
						imageRegistrySecret1.Namespace,
					)))
					g.Expect(readyCondition.Reason).To(Equal("RegistrySecretPropagation"))
					g.Expect(readyCondition.ObservedGeneration).To(Equal(createdOrg.Generation))
				}, 5*time.Second).Should(Succeed())
			})
		})

		It("propagates the role-bindings with annotation \"cloudfoundry.org/propagate-cf-role\" set to \"true\" from root-ns to org namespace", func() {
			Eventually(func(_ Gomega) error {
				var createdRoleBinding rbacv1.RoleBinding
				return adminClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Name, Name: roleBinding.Name}, &createdRoleBinding)
			}).Should(Succeed())
		})

		It("does not propagate role-bindings with annotation \"cloudfoundry.org/propagate-cf-role\" set to \"false\"", func() {
			Consistently(func(_ Gomega) bool {
				var newRoleBinding rbacv1.RoleBinding
				return apierrors.IsNotFound(adminClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Name, Name: roleBindingWithPropagateAnnotationSetToFalse.Name}, &newRoleBinding))
			}, time.Second).Should(BeTrue())
		})

		It("does not propagate role-bindings with missing annotation \"cloudfoundry.org/propagate-cf-role\"", func() {
			Consistently(func(_ Gomega) bool {
				var newRoleBinding rbacv1.RoleBinding
				return apierrors.IsNotFound(adminClient.Get(ctx, types.NamespacedName{Namespace: cfOrg.Name, Name: roleBindingWithMissingPropagateAnnotation.Name}, &newRoleBinding))
			}, time.Second).Should(BeTrue())
		})

		It("sets the status on the CFOrg", func() {
			Eventually(func(g Gomega) {
				var createdOrg korifiv1alpha1.CFOrg
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfRootNamespace, Name: orgGUID}, &createdOrg)).To(Succeed())

				g.Expect(createdOrg.Status.GUID).To(Equal(orgGUID))
				g.Expect(createdOrg.Status.ObservedGeneration).To(Equal(createdOrg.Generation))
				g.Expect(meta.IsStatusConditionTrue(createdOrg.Status.Conditions, "Ready")).To(BeTrue())
			}).Should(Succeed())
		})

		It("sets restricted pod security labels on the namespace", func() {
			Eventually(func(g Gomega) {
				var ns v1.Namespace
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: orgGUID}, &ns)).To(Succeed())

				g.Expect(ns.Labels).To(HaveKeyWithValue(api.EnforceLevelLabel, string(api.LevelRestricted)))
			}).Should(Succeed())
		})
	})

	When("the CFOrg is updated after namespace modifications", func() {
		var originalOrg *korifiv1alpha1.CFOrg

		BeforeEach(func() {
			Expect(adminClient.Create(ctx, &cfOrg)).To(Succeed())

			originalOrg = cfOrg.DeepCopy()
			var createdNamespace v1.Namespace
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: orgGUID}, &createdNamespace)).To(Succeed())
			}).Should(Succeed())

			updatedNamespace := createdNamespace.DeepCopy()
			updatedNamespace.Labels["foo.com/bar"] = "42"
			updatedNamespace.Annotations["foo.com/bar"] = "43"
			Expect(adminClient.Patch(ctx, updatedNamespace, client.MergeFrom(&createdNamespace))).To(Succeed())

			cfOrg.Spec.DisplayName += "x"
		})

		JustBeforeEach(func() {
			Expect(adminClient.Patch(ctx, &cfOrg, client.MergeFrom(originalOrg))).To(Succeed())
		})

		It("sets the new display name annotation and preserves the added label and annoations", func() {
			Eventually(func(g Gomega) {
				var createdOrg v1.Namespace
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: orgGUID}, &createdOrg)).To(Succeed())

				g.Expect(createdOrg.Annotations).To(HaveKeyWithValue("foo.com/bar", "43"))
				g.Expect(createdOrg.Annotations).To(HaveKeyWithValue(korifiv1alpha1.OrgNameKey, cfOrg.Spec.DisplayName))
				g.Expect(createdOrg.Labels).To(HaveKeyWithValue("foo.com/bar", "42"))
			}).Should(Succeed())
		})
	})

	When("role-bindings are added/updated in root-ns after CFOrg creation", func() {
		var newlyCreatedRoleBinding *rbacv1.RoleBinding
		BeforeEach(func() {
			Expect(adminClient.Create(ctx, &cfOrg)).To(Succeed())

			Eventually(func(g Gomega) {
				var createdOrg korifiv1alpha1.CFOrg
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfRootNamespace, Name: orgGUID}, &createdOrg)).To(Succeed())

				g.Expect(meta.IsStatusConditionTrue(createdOrg.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			newlyCreatedRoleBinding = createRoleBinding(ctx, adminClient, PrefixedGUID("newly-created-role-binding"), PrefixedGUID("new-user"), role.Name, cfRootNamespace, map[string]string{"cloudfoundry.org/propagate-cf-role": "true"})
		})

		It("propagates the new role-binding to org namespace", func() {
			Eventually(func(g Gomega) {
				var createdRoleBindings rbacv1.RoleBindingList
				g.Expect(adminClient.List(ctx, &createdRoleBindings, client.InNamespace(cfOrg.Name))).To(Succeed())

				g.Expect(createdRoleBindings.Items).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(roleBinding.Name),
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

	When("role bindings are deleted in the root-ns after CFOrg creation", func() {
		BeforeEach(func() {
			Expect(adminClient.Create(ctx, &cfOrg)).To(Succeed())

			Eventually(func(g Gomega) {
				var createdOrg korifiv1alpha1.CFOrg
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Namespace: cfRootNamespace, Name: orgGUID}, &createdOrg)).To(Succeed())

				g.Expect(meta.IsStatusConditionTrue(createdOrg.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())
		})

		JustBeforeEach(func() {
			Expect(adminClient.Delete(ctx, roleBinding)).To(Succeed())
		})

		It("deletes the corresponding role binding in CFOrg", func() {
			Eventually(func() bool {
				var deletedRoleBinding rbacv1.RoleBinding
				return apierrors.IsNotFound(adminClient.Get(ctx, types.NamespacedName{Name: roleBinding.Name, Namespace: cfOrg.Name}, &deletedRoleBinding))
			}).Should(BeTrue(), "timed out waiting for role binding to be deleted")
		})

		When("the role binding is annotated not to propagate deletions", func() {
			BeforeEach(func() {
				origRoleBinding := roleBinding.DeepCopy()

				roleBinding.Annotations["cloudfoundry.org/propagate-deletion"] = "false"
				Expect(adminClient.Patch(ctx, roleBinding, client.MergeFrom(origRoleBinding))).To(Succeed())

				Eventually(func(g Gomega) map[string]string {
					var copiedRoleBinding rbacv1.RoleBinding
					g.Expect(
						adminClient.Get(ctx, types.NamespacedName{Name: roleBinding.Name, Namespace: cfOrg.Name}, &copiedRoleBinding),
					).To(Succeed())
					return copiedRoleBinding.Annotations
				}).Should(HaveKeyWithValue("cloudfoundry.org/propagate-deletion", "false"))
			})

			It("doesn't delete the corresponding role binding in the CFOrg", func() {
				Consistently(func() bool {
					return apierrors.IsNotFound(adminClient.Get(ctx, types.NamespacedName{Name: roleBinding.Name, Namespace: cfOrg.Name}, new(rbacv1.RoleBinding)))
				}).Should(BeFalse(), "org's copy of role binding was deleted and shouldn't have been")
			})
		})
	})

	When("the CFOrg is deleted", func() {
		BeforeEach(func() {
			Expect(adminClient.Create(ctx, &cfOrg)).To(Succeed())

			Eventually(func(g Gomega) {
				var orgNamespace v1.Namespace
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: orgGUID}, &orgNamespace)).To(Succeed())
			}).Should(Succeed())

			Expect(adminClient.Delete(ctx, &cfOrg)).To(Succeed())
		})

		It("eventually deletes the namespace", func() {
			// Envtests do not cleanup namespaces. For testing, we check for deletion timestamps on namespace.
			Eventually(func(g Gomega) bool {
				var orgNamespace v1.Namespace
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: orgGUID}, &orgNamespace)).To(Succeed())

				return orgNamespace.GetDeletionTimestamp().IsZero()
			}).Should(BeFalse(), "timed out waiting for deletion timestamps to be set on namespace")
		})

		It("writes some log message from the finalizer helper", func() {
			Eventually(logOutput).WithTimeout(30 * time.Second).Should(gbytes.Say("controllers.CFOrg.finalize"))
		})
	})
})
