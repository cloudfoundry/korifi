package integration_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/pod-security-admission/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
)

var _ = Describe("CFOrgReconciler Integration Tests", func() {
	const (
		orgName = "my-org"
	)
	var (
		testCtx                                      context.Context
		rootNamespace                                *v1.Namespace
		orgGUID                                      string
		cfOrg                                        korifiv1alpha1.CFOrg
		imageRegistrySecret                          *v1.Secret
		role                                         *rbacv1.ClusterRole
		rules                                        []rbacv1.PolicyRule
		username                                     string
		roleBinding                                  rbacv1.RoleBinding
		roleBindingWithPropagateAnnotationSetToFalse rbacv1.RoleBinding
		roleBindingWithMissingPropagateAnnotation    rbacv1.RoleBinding
	)

	BeforeEach(func() {
		testCtx = context.Background()
		rootNamespace = createNamespace(testCtx, k8sClient, PrefixedGUID("root-ns"))
		imageRegistrySecret = createSecret(testCtx, k8sClient, packageRegistrySecretName, rootNamespace.Name)
		rules = []rbacv1.PolicyRule{
			{
				Verbs:         []string{"use"},
				APIGroups:     []string{"policy"},
				Resources:     []string{"podsecuritypolicies"},
				ResourceNames: []string{"eirini-workloads-app-psp"},
			},
		}
		role = createClusterRole(testCtx, k8sClient, PrefixedGUID("clusterrole"), rules)

		username = PrefixedGUID("user")
		roleBinding = createRoleBinding(testCtx, k8sClient, PrefixedGUID("role-binding"), username, role.Name, rootNamespace.Name, map[string]string{"cloudfoundry.org/propagate-cf-role": "true"})

		username2 := PrefixedGUID("user2")
		roleBindingWithPropagateAnnotationSetToFalse = createRoleBinding(testCtx, k8sClient, PrefixedGUID("rb-propagate-annotation-false"), username2, role.Name, rootNamespace.Name, map[string]string{"cloudfoundry.org/propagate-cf-role": "false"})

		roleBindingWithMissingPropagateAnnotation = createRoleBinding(testCtx, k8sClient, PrefixedGUID("rb-missing-propagate-annotation"), username2, role.Name, rootNamespace.Name, nil)

		orgGUID = PrefixedGUID("cf-org")
		cfOrg = korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:      orgGUID,
				Namespace: rootNamespace.Name,
			},
			Spec: korifiv1alpha1.CFOrgSpec{
				DisplayName: orgName,
			},
		}
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), rootNamespace)).To(Succeed())
	})

	When("the CFOrg is created", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(testCtx, &cfOrg)).To(Succeed())
		})

		It("creates an org namespace and sets labels", func() {
			Eventually(func(g Gomega) {
				var orgNamespace v1.Namespace
				g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: orgGUID}, &orgNamespace)).To(Succeed())
				g.Expect(orgNamespace.Labels).To(HaveKeyWithValue("cloudfoundry.org/org-name", cfOrg.Spec.DisplayName))
			}).Should(Succeed())
		})

		It("sets the finalizer on cfOrg", func() {
			Eventually(func(g Gomega) []string {
				var createdCFOrg korifiv1alpha1.CFOrg
				g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &createdCFOrg)).To(Succeed())
				return createdCFOrg.ObjectMeta.Finalizers
			}).Should(ConsistOf([]string{
				"cfOrg.korifi.cloudfoundry.org",
			}))
		})

		It("propagates the image-registry-credentials from root-ns to org namespace", func() {
			Eventually(func(g Gomega) {
				var createdSecret v1.Secret
				g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Namespace: cfOrg.Name, Name: imageRegistrySecret.Name}, &createdSecret)).To(Succeed())
				g.Expect(createdSecret.Immutable).To(Equal(imageRegistrySecret.Immutable))
				g.Expect(createdSecret.Data).To(Equal(imageRegistrySecret.Data))
				g.Expect(createdSecret.StringData).To(Equal(imageRegistrySecret.StringData))
				g.Expect(createdSecret.Type).To(Equal(imageRegistrySecret.Type))
			}).Should(Succeed())
		})

		It("propagates the role-bindings with annotation \"cloudfoundry.org/propagate-cf-role\" set to \"true\" from root-ns to org namespace", func() {
			Eventually(func(g Gomega) error {
				var createdRoleBinding rbacv1.RoleBinding
				return k8sClient.Get(testCtx, types.NamespacedName{Namespace: cfOrg.Name, Name: roleBinding.Name}, &createdRoleBinding)
			}).Should(Succeed())
		})

		It("does not propagate role-bindings with annotation \"cloudfoundry.org/propagate-cf-role\" set to \"false\"", func() {
			Consistently(func(g Gomega) bool {
				var newRoleBinding rbacv1.RoleBinding
				return apierrors.IsNotFound(k8sClient.Get(testCtx, types.NamespacedName{Namespace: cfOrg.Name, Name: roleBindingWithPropagateAnnotationSetToFalse.Name}, &newRoleBinding))
			}, time.Second).Should(BeTrue())
		})

		It("does not propagate role-bindings with missing annotation \"cloudfoundry.org/propagate-cf-role\"", func() {
			Consistently(func(g Gomega) bool {
				var newRoleBinding rbacv1.RoleBinding
				return apierrors.IsNotFound(k8sClient.Get(testCtx, types.NamespacedName{Namespace: cfOrg.Name, Name: roleBindingWithMissingPropagateAnnotation.Name}, &newRoleBinding))
			}, time.Second).Should(BeTrue())
		})

		It("sets the CFOrg 'Ready' condition to 'True'", func() {
			Eventually(func(g Gomega) {
				var latestOrg korifiv1alpha1.CFOrg
				err := k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &latestOrg)
				g.Expect(err).To(BeNil())
				g.Expect(meta.IsStatusConditionTrue(latestOrg.Status.Conditions, "Ready")).To(BeTrue())
			}, 5*time.Second).Should(Succeed())
		})

		It("sets status on CFOrg", func() {
			Eventually(func(g Gomega) {
				var createdOrg korifiv1alpha1.CFOrg
				g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &createdOrg)).To(Succeed())
				g.Expect(createdOrg.Status.GUID).To(Equal(orgGUID))
				g.Expect(meta.IsStatusConditionTrue(createdOrg.Status.Conditions, "Ready")).To(BeTrue())
			}).Should(Succeed())
		})

		It("sets restricted pod security labels on the namespace", func() {
			Eventually(func(g Gomega) {
				var ns v1.Namespace
				g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: orgGUID}, &ns)).To(Succeed())
				g.Expect(ns.Labels).To(HaveKeyWithValue(api.EnforceLevelLabel, string(api.LevelRestricted)))
				g.Expect(ns.Labels).To(HaveKeyWithValue(api.EnforceLevelLabel, string(api.LevelRestricted)))
			}).Should(Succeed())
		})
	})

	When("role-bindings are added/updated in root-ns after CFOrg creation", func() {
		var newlyCreatedRoleBinding rbacv1.RoleBinding
		BeforeEach(func() {
			Expect(k8sClient.Create(testCtx, &cfOrg)).To(Succeed())
			Eventually(func(g Gomega) {
				var createdOrg korifiv1alpha1.CFOrg
				g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &createdOrg)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdOrg.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			newlyCreatedRoleBinding = createRoleBinding(testCtx, k8sClient, PrefixedGUID("newly-created-role-binding"), PrefixedGUID("new-user"), role.Name, rootNamespace.Name, map[string]string{"cloudfoundry.org/propagate-cf-role": "true"})
		})

		It("propagates the new role-binding to org namespace", func() {
			Eventually(func(g Gomega) {
				var createdRoleBindings rbacv1.RoleBindingList
				g.Expect(k8sClient.List(testCtx, &createdRoleBindings, client.InNamespace(cfOrg.Name))).To(Succeed())
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
			Expect(k8sClient.Create(testCtx, &cfOrg)).To(Succeed())
			Eventually(func(g Gomega) {
				var createdOrg korifiv1alpha1.CFOrg
				g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Namespace: rootNamespace.Name, Name: orgGUID}, &createdOrg)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdOrg.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			Expect(k8sClient.Delete(testCtx, &roleBinding)).To(Succeed())
		})

		It("deletes the corresponding role binding in CFOrg", func() {
			Eventually(func() bool {
				var deletedRoleBinding rbacv1.RoleBinding
				return apierrors.IsNotFound(k8sClient.Get(testCtx, types.NamespacedName{Name: roleBinding.Name, Namespace: cfOrg.Name}, &deletedRoleBinding))
			}).Should(BeTrue(), "timed out waiting for role binding to be deleted")
		})
	})

	When("the CFOrg is deleted", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(testCtx, &cfOrg)).To(Succeed())
			Eventually(func(g Gomega) {
				var orgNamespace v1.Namespace
				g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: orgGUID}, &orgNamespace)).To(Succeed())
			}).Should(Succeed())

			Expect(k8sClient.Delete(testCtx, &cfOrg)).To(Succeed())
		})

		It("eventually deletes the CFOrg", func() {
			Eventually(func() bool {
				var createdCFOrg korifiv1alpha1.CFOrg
				return apierrors.IsNotFound(k8sClient.Get(context.Background(), types.NamespacedName{Name: orgGUID, Namespace: rootNamespace.Name}, &createdCFOrg))
			}).Should(BeTrue(), "timed out waiting for org to be deleted")
		})

		It("eventually deletes the namespace", func() {
			// Envtests do not cleanup namespaces. For testing, we check for deletion timestamps on namespace.
			Eventually(func(g Gomega) bool {
				var orgNamespace v1.Namespace
				g.Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: orgGUID}, &orgNamespace)).To(Succeed())
				return orgNamespace.GetDeletionTimestamp().IsZero()
			}).Should(BeFalse(), "timed out waiting for deletion timestamps to be set on namespace")
		})
	})
})
