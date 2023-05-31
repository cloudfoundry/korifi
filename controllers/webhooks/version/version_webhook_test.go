package version_test

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"code.cloudfoundry.org/korifi/version"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultMemoryMB    = 128
	defaultDiskQuotaMB = 256
	defaultTimeout     = 60
)

var _ = Describe("Setting the version annotation", func() {
	Describe("create", func() {
		var testObjects []client.Object
		createObject := func(obj client.Object) client.Object {
			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
			return obj
		}

		BeforeEach(func() {
			orgNamespace := "test-org-" + uuid.NewString()
			Expect(k8sClient.Create(context.Background(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   orgNamespace,
					Labels: map[string]string{korifiv1alpha1.OrgNameKey: orgNamespace},
				},
			})).To(Succeed())

			domainName := uuid.NewString()
			testObjects = []client.Object{
				createObject(&korifiv1alpha1.CFOrg{
					ObjectMeta: metav1.ObjectMeta{
						Name:      orgNamespace,
						Namespace: rootNamespace,
					},
					Spec: korifiv1alpha1.CFOrgSpec{
						DisplayName: orgNamespace,
					},
				}),
				createObject(&korifiv1alpha1.CFSpace{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
					Spec: korifiv1alpha1.CFSpaceSpec{
						DisplayName: "asdf",
					},
				}),
				createObject(&korifiv1alpha1.BuilderInfo{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      uuid.NewString(),
					},
				}),
				createObject(&korifiv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      domainName,
					},
					Spec: korifiv1alpha1.CFDomainSpec{
						Name: "my.domain",
					},
				}),
				createObject(&korifiv1alpha1.CFServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
					Spec: korifiv1alpha1.CFServiceInstanceSpec{
						Type: "user-provided",
					},
				}),
				createObject(&korifiv1alpha1.CFApp{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
					Spec: korifiv1alpha1.CFAppSpec{
						DisplayName:  "cfapp",
						DesiredState: "STOPPED",
						Lifecycle: korifiv1alpha1.Lifecycle{
							Type: "buildpack",
						},
					},
				}),
				createObject(&korifiv1alpha1.CFPackage{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
					Spec: korifiv1alpha1.CFPackageSpec{
						Type: "bits",
					},
				}),
				createObject(&korifiv1alpha1.CFTask{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
					Spec: korifiv1alpha1.CFTaskSpec{
						Command: "foo",
						AppRef: corev1.LocalObjectReference{
							Name: "bob",
						},
					},
				}),
				createObject(&korifiv1alpha1.CFProcess{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
					Spec: korifiv1alpha1.CFProcessSpec{
						Ports: []int32{80},
					},
				}),
				createObject(&korifiv1alpha1.CFBuild{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
					Spec: korifiv1alpha1.CFBuildSpec{
						Lifecycle: korifiv1alpha1.Lifecycle{
							Type: "buildpack",
						},
					},
				}),
				createObject(&korifiv1alpha1.CFRoute{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
					Spec: korifiv1alpha1.CFRouteSpec{
						DomainRef: corev1.ObjectReference{
							Name:      domainName,
							Namespace: rootNamespace,
						},
						Host: "myhost",
					},
				}),
				createObject(&korifiv1alpha1.CFServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
				}),
				createObject(&korifiv1alpha1.TaskWorkload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
					Spec: korifiv1alpha1.TaskWorkloadSpec{
						Command: []string{"foo"},
					},
				}),
				createObject(&korifiv1alpha1.AppWorkload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
				}),
				createObject(&korifiv1alpha1.BuildWorkload{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: orgNamespace,
						Name:      uuid.NewString(),
					},
				}),
			}
		})

		It("sets the version annotation", func() {
			for _, o := range testObjects {
				Expect(o.GetAnnotations()).To(HaveKeyWithValue(version.KorifiCreationVersionKey, "some-version"),
					func() string { return fmt.Sprintf("%T does not have the expected version", o) },
				)
			}
		})
	})

	Describe("updates", func() {
		var taskWorkload *korifiv1alpha1.TaskWorkload

		BeforeEach(func() {
			taskWorkload = &korifiv1alpha1.TaskWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.TaskWorkloadSpec{Command: []string{"foo"}},
			}
			Expect(k8sClient.Create(context.Background(), taskWorkload)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(taskWorkload), taskWorkload)).To(Succeed())
				g.Expect(taskWorkload.Annotations).To(HaveKeyWithValue(version.KorifiCreationVersionKey, "some-version"))
			}).Should(Succeed())
		})

		When("unsetting the version", func() {
			JustBeforeEach(func() {
				Expect(k8s.Patch(context.Background(), k8sClient, taskWorkload, func() {
					delete(taskWorkload.Annotations, version.KorifiCreationVersionKey)
					taskWorkload.Spec.Command = []string{"bar"}
				})).To(Succeed())
			})

			It("restores it", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(taskWorkload), taskWorkload)).To(Succeed())
					g.Expect(taskWorkload.Annotations).To(HaveKeyWithValue(version.KorifiCreationVersionKey, "some-version"))
					g.Expect(taskWorkload.Spec.Command).To(ConsistOf("bar"))
				}).Should(Succeed())
			})
		})

		When("updating the version", func() {
			JustBeforeEach(func() {
				Expect(k8s.Patch(context.Background(), k8sClient, taskWorkload, func() {
					taskWorkload.Annotations[version.KorifiCreationVersionKey] = "foo"
				})).To(Succeed())
			})

			It("leaves the new version", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(taskWorkload), taskWorkload)).To(Succeed())
					g.Expect(taskWorkload.Annotations).To(HaveKeyWithValue(version.KorifiCreationVersionKey, "foo"))
				}).Should(Succeed())
			})
		})
	})
})
