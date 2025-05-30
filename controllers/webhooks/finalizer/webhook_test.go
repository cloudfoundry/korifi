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
				Expect(adminClient.Create(context.Background(), &korifiv1alpha1.CFOrg{
					ObjectMeta: metav1.ObjectMeta{
						Name:      obj.GetNamespace(),
						Namespace: rootNamespace,
					},
					Spec: korifiv1alpha1.CFOrgSpec{
						DisplayName: obj.GetNamespace(),
					},
				})).To(Succeed())

				Expect(adminClient.Create(context.Background(), &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   obj.GetNamespace(),
						Labels: map[string]string{korifiv1alpha1.CFOrgDisplayNameKey: obj.GetNamespace()},
					},
				})).To(Succeed())
			}

			Expect(adminClient.Create(context.Background(), obj)).To(Succeed())
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
		Entry("managed CF service instance",
			&korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-org-" + uuid.NewString(),
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					Type: korifiv1alpha1.ManagedType,
				},
			},
			korifiv1alpha1.CFServiceInstanceFinalizerName,
		),
		Entry("user-provided CF service instance",
			&korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-org-" + uuid.NewString(),
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					Type: korifiv1alpha1.UserProvidedType,
				},
			},
			korifiv1alpha1.CFServiceInstanceFinalizerName,
		),
		Entry("cfservicebinding",
			&korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-org-" + uuid.NewString(),
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Type: korifiv1alpha1.CFServiceBindingTypeApp,
				},
			},
			korifiv1alpha1.CFServiceBindingFinalizerName,
		),
	)
})
