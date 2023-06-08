package finalizer_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Controllers Finalizers Webhook", func() {
	DescribeTable("Adding finalizers",
		func(obj client.Object, expectedFinalizers ...string) {
			if obj.GetNamespace() != rootNamespace {
				Expect(k8sClient.Create(context.Background(), &korifiv1alpha1.CFOrg{
					ObjectMeta: metav1.ObjectMeta{
						Name:      obj.GetNamespace(),
						Namespace: rootNamespace,
					},
					Spec: korifiv1alpha1.CFOrgSpec{
						DisplayName: obj.GetNamespace(),
					},
				})).To(Succeed())

				Expect(k8sClient.Create(context.Background(), &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   obj.GetNamespace(),
						Labels: map[string]string{korifiv1alpha1.OrgNameKey: obj.GetNamespace()},
					},
				})).To(Succeed())
			}

			Expect(k8sClient.Create(context.Background(), obj)).To(Succeed())
			Expect(obj.GetFinalizers()).To(ConsistOf(expectedFinalizers))
		},
		Entry("cfapp",
			&korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-org-" + uuid.NewString(),
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFAppSpec{
					DisplayName:  "cfapp",
					DesiredState: "STOPPED",
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			},
			korifiv1alpha1.CFAppFinalizerName,
		),
		Entry("cfspace",
			&korifiv1alpha1.CFSpace{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-org-" + uuid.NewString(),
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFSpaceSpec{
					DisplayName: "asdf",
				},
			},
			korifiv1alpha1.CFSpaceFinalizerName,
		),
		Entry("cfpackage",
			&korifiv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-org-" + uuid.NewString(),
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFPackageSpec{
					Type: "bits",
				},
			},
			korifiv1alpha1.CFPackageFinalizerName,
		),
		Entry("cforg",
			&korifiv1alpha1.CFOrg{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      "test-org-" + uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFOrgSpec{
					DisplayName: "test-org-" + uuid.NewString(),
				},
			},
			korifiv1alpha1.CFOrgFinalizerName,
		),
		Entry("cfroute",
			&korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-org-" + uuid.NewString(),
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					DomainRef: corev1.ObjectReference{
						Name:      defaultDomainName,
						Namespace: rootNamespace,
					},
					Host: "myhost",
				},
			},
			korifiv1alpha1.CFRouteFinalizerName,
		),
		Entry("cfdomain",
			&korifiv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFDomainSpec{
					Name: uuid.NewString() + ".foo",
				},
			},
			korifiv1alpha1.CFDomainFinalizerName,
		),
		Entry("builderinfo (no finalizer is added)",
			&korifiv1alpha1.BuilderInfo{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
			},
		),
	)
})
