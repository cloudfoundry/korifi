package k8sns_test

import (
	"context"
	"errors"
	"maps"
	"slices"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/k8sns"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mockMetadataCompiler[T any, NS k8sns.NamespaceObject[T]] struct {
	processedObjects map[NS]any
	labels           map[string]string
	annotations      map[string]string
}

func (c *mockMetadataCompiler[T, NS]) CompileLabels(obj NS) map[string]string {
	c.processedObjects[obj] = struct{}{}
	return c.labels
}

func (c *mockMetadataCompiler[T, NS]) CompileAnnotations(obj NS) map[string]string {
	c.processedObjects[obj] = struct{}{}
	return c.annotations
}

var _ = Describe("K8S NS Reconciler Integration Tests", func() {
	var (
		orgGUID string
		nsObj   *korifiv1alpha1.CFOrg

		reconciler       *k8sns.Reconciler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg]
		finalizer        *mockFinalizer[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg]
		metadataCompiler *mockMetadataCompiler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg]

		result       ctrl.Result
		reconcileErr error
	)

	BeforeEach(func() {
		createNamespace(rootNamespace)

		finalizer = &mockFinalizer[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg]{}
		metadataCompiler = &mockMetadataCompiler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg]{
			processedObjects: map[*korifiv1alpha1.CFOrg]any{},
			labels: map[string]string{
				"org-label": "org-label-value",
			},
			annotations: map[string]string{
				"org-annotation": "org-annotation-value",
			},
		}
		reconciler = k8sns.NewReconciler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg](controllersClient, finalizer, metadataCompiler, []string{})

		orgGUID = uuid.NewString()
		nsObj = &korifiv1alpha1.CFOrg{
			ObjectMeta: metav1.ObjectMeta{
				Name:       orgGUID,
				Namespace:  rootNamespace,
				Generation: 3,
			},
			Spec: korifiv1alpha1.CFOrgSpec{
				DisplayName: uuid.NewString(),
			},
		}
	})

	JustBeforeEach(func() {
		result, reconcileErr = reconciler.ReconcileResource(ctx, nsObj)
	})

	expectToHaveSucceeded := func() {
		GinkgoHelper()

		Expect(reconcileErr).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	}

	It("sets the status on the NSObj", func() {
		expectToHaveSucceeded()

		Expect(nsObj.Status.GUID).To(Equal(orgGUID))
		Expect(nsObj.Status.ObservedGeneration).To(Equal(nsObj.Generation))
	})

	Describe("the underlying namespace", func() {
		var underlyingNamespace *corev1.Namespace

		JustBeforeEach(func() {
			expectToHaveSucceeded()

			underlyingNamespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsObj.Name,
				},
			}
			Expect(controllersClient.Get(ctx, client.ObjectKeyFromObject(underlyingNamespace), underlyingNamespace)).To(Succeed())
		})

		It("sets labels and annotations", func() {
			Expect(metadataCompiler.processedObjects).To(SatisfyAll(HaveLen(1), HaveKey(nsObj)))
			Expect(underlyingNamespace.Labels).To(HaveKeyWithValue("org-label", "org-label-value"))
			Expect(underlyingNamespace.Annotations).To(HaveKeyWithValue("org-annotation", "org-annotation-value"))
		})

		When("the underlying namespace already exists", func() {
			BeforeEach(func() {
				ns := createNamespace(nsObj.Name)
				Expect(k8s.PatchResource(ctx, controllersClient, ns, func() {
					ns.Labels = map[string]string{
						"foo.com/bar": "42",
					}
					ns.Annotations = map[string]string{
						"foo.com/bar": "43",
					}
				})).To(Succeed())
			})

			It("merges original labels and annotations", func() {
				ns := getNamespace(nsObj.Name)
				Expect(ns.Labels).To(SatisfyAll(
					HaveKeyWithValue("foo.com/bar", "42"),
					HaveKeyWithValue("org-label", "org-label-value"),
				))
				Expect(ns.Annotations).To(SatisfyAll(
					HaveKeyWithValue("foo.com/bar", "43"),
					HaveKeyWithValue("org-annotation", "org-annotation-value"),
				))
			})
		})
	})

	Describe("image registry secrets propagation", func() {
		var imageRegistrySecret *corev1.Secret

		BeforeEach(func() {
			imageRegistrySecret = createImageRegistrySecret(ctx, "container-registry-secret", rootNamespace)
			reconciler = k8sns.NewReconciler[korifiv1alpha1.CFOrg, *korifiv1alpha1.CFOrg](controllersClient, finalizer, metadataCompiler, []string{imageRegistrySecret.Name})
		})

		It("propagates the image-registry-credentials secrets from root-ns to the underlying namespace", func() {
			expectToHaveSucceeded()

			createdSecret := &corev1.Secret{}
			Expect(controllersClient.Get(ctx, types.NamespacedName{Namespace: nsObj.Name, Name: imageRegistrySecret.Name}, createdSecret)).To(Succeed())

			Expect(createdSecret.Data).To(Equal(imageRegistrySecret.Data))
			Expect(createdSecret.Immutable).To(Equal(imageRegistrySecret.Immutable))
			Expect(createdSecret.Type).To(Equal(imageRegistrySecret.Type))

			By("omitting annotations from deployment tools", func() {
				Expect(slices.Collect(maps.Keys(createdSecret.Annotations))).NotTo(ContainElements("kapp.k14s.io/foo", "meta.helm.sh/baz"))
			})
		})

		When("the image-registry-credentials secret does not exist in the root-ns", Serial, func() {
			BeforeEach(func() {
				reconciler = k8sns.NewReconciler(controllersClient, finalizer, metadataCompiler, []string{"i-do-not-exist"})
			})

			It("returns an error", func() {
				Expect(reconcileErr).To(MatchError(ContainSubstring("error fetching secret")))
			})
		})
	})

	Describe("role bindings propagation", func() {
		var roleBinding *rbacv1.RoleBinding

		BeforeEach(func() {
			rules := []rbacv1.PolicyRule{
				{
					Verbs:     []string{"create"},
					APIGroups: []string{"korifi.cloudfoundry.org"},
					Resources: []string{"cfapps"},
				},
			}
			role := createClusterRole(ctx, rules)
			roleBinding = createRoleBinding(ctx, role.Name, rootNamespace)
		})

		JustBeforeEach(func() {
			expectToHaveSucceeded()
		})

		It("does not propagate role-bindings with missing annotation \"cloudfoundry.org/propagate-cf-role\"", func() {
			Expect(apierrors.IsNotFound(controllersClient.Get(
				ctx,
				types.NamespacedName{Namespace: nsObj.Name, Name: roleBinding.Name},
				&rbacv1.RoleBinding{},
			))).To(BeTrue())
		})

		When("a role binding has annotation \"cloudfoundry.org/propagate-cf-role\" set to \"false\"", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, controllersClient, roleBinding, func() {
					roleBinding.Annotations = map[string]string{
						korifiv1alpha1.PropagateRoleBindingAnnotation: "false",
					}
				})).To(Succeed())
			})

			It("does not propagate the role binding", func() {
				Expect(apierrors.IsNotFound(controllersClient.Get(
					ctx,
					types.NamespacedName{Namespace: nsObj.Name, Name: roleBinding.Name},
					&rbacv1.RoleBinding{},
				))).To(BeTrue())
			})
		})

		When("a role binding has annotation \"cloudfoundry.org/propagate-cf-role\" set to \"true\"", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, controllersClient, roleBinding, func() {
					roleBinding.Annotations = map[string]string{
						korifiv1alpha1.PropagateRoleBindingAnnotation: "true",
					}
				})).To(Succeed())
			})

			It("propagates the role binding", func() {
				Expect(controllersClient.Get(
					ctx,
					types.NamespacedName{Namespace: nsObj.Name, Name: roleBinding.Name},
					&rbacv1.RoleBinding{},
				)).To(Succeed())
			})
		})

		When("a matching role binding has been deleted in the root namespace", func() {
			var underlyingRoleBinding *rbacv1.RoleBinding

			BeforeEach(func() {
				createNamespace(nsObj.Name)
				underlyingRoleBinding = &rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: nsObj.Name,
						Name:      roleBinding.Name,
					},
					Subjects: roleBinding.Subjects,
					RoleRef:  roleBinding.RoleRef,
				}
				Expect(controllersClient.Create(ctx, underlyingRoleBinding)).To(Succeed())
				Expect(controllersClient.Delete(ctx, roleBinding)).To(Succeed())
			})

			It("preserves the role binding in the underlying namespace", func() {
				Expect(controllersClient.Get(ctx, client.ObjectKeyFromObject(underlyingRoleBinding), underlyingRoleBinding)).To(Succeed())
			})

			When("the role binding in the underlying namespace is annotated to be propagated", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, controllersClient, underlyingRoleBinding, func() {
						underlyingRoleBinding.Labels = map[string]string{
							korifiv1alpha1.PropagatedFromLabel: rootNamespace,
						}
						underlyingRoleBinding.Annotations = map[string]string{
							korifiv1alpha1.PropagateDeletionAnnotation: "true",
						}
					})).To(Succeed())
				})

				It("deletes the role binding in the underlying namespace", func() {
					rbList := &rbacv1.RoleBindingList{}
					Expect(controllersClient.List(ctx, rbList, client.InNamespace(nsObj.Name))).To(Succeed())
					Expect(rbList.Items).To(BeEmpty())
				})
			})
		})
	})

	Describe("deletion", func() {
		BeforeEach(func() {
			nsObj.DeletionTimestamp = tools.PtrTo(metav1.Now())
		})

		It("finalizes the org", func() {
			expectToHaveSucceeded()
			Expect(finalizer.finalizedObjects).To(ConsistOf(nsObj))
		})

		When("the finalizer requeues", func() {
			BeforeEach(func() {
				finalizer.result = ctrl.Result{RequeueAfter: 1234}
			})

			It("requeues", func() {
				Expect(reconcileErr).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{RequeueAfter: 1234}))
			})
		})

		When("the finalizer fails", func() {
			BeforeEach(func() {
				finalizer.finalizeErr = errors.New("finalize-err")
			})

			It("returns the error", func() {
				Expect(reconcileErr).To(MatchError("finalize-err"))
			})
		})
	})
})

func createClusterRole(ctx context.Context, rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: uuid.NewString(),
		},
		Rules: rules,
	}
	Expect(adminClient.Create(ctx, role)).To(Succeed())
	return role
}

func createRoleBinding(ctx context.Context, clusterRole, namespace string) *rbacv1.RoleBinding {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: namespace,
		},
		Subjects: []rbacv1.Subject{{
			Kind: rbacv1.UserKind,
			Name: uuid.NewString(),
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: clusterRole,
		},
	}
	Expect(adminClient.Create(ctx, roleBinding)).To(Succeed())
	return roleBinding
}

func createImageRegistrySecret(ctx context.Context, name string, namespace string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"kapp.k14s.io/foo": "bar",
				"meta.helm.sh/baz": "foo",
				"bar":              "baz",
			},
		},
		StringData: map[string]string{
			"foo": "bar",
		},
		Type: "Docker",
	}
	Expect(adminClient.Create(ctx, secret)).To(Succeed())
	return secret
}
