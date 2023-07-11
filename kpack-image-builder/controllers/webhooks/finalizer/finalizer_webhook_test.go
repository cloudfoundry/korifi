package finalizer_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kpackv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("KpackImageBuilder Finalizers Webhook", func() {
	DescribeTable("Adding finalizers",
		func(obj client.Object, expectedFinalizers []string) {
			Expect(adminClient.Create(context.Background(), obj)).To(Succeed())
			Expect(obj.GetFinalizers()).To(Equal(expectedFinalizers))
		},
		Entry("kpack-image-builder-buildworkload",
			&korifiv1alpha1.BuildWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.BuildWorkloadSpec{
					BuilderName: "kpack-image-builder",
				},
			},
			[]string{korifiv1alpha1.BuildWorkloadFinalizerName},
		),
		Entry("non-kpack-image-builder-buildworkload",
			&korifiv1alpha1.BuildWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.BuildWorkloadSpec{
					BuilderName: "another-image-builder",
				},
			},
			nil,
		),
		Entry("korifi-kpackbuild",
			&kpackv1alpha2.Build{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: rootNamespace,
					Labels: map[string]string{
						controllers.BuildWorkloadLabelKey: "my-build-workload",
					},
				},
			},
			[]string{controllers.KpackBuildFinalizer},
		),
		Entry("non-korifi-kpackbuild",
			&kpackv1alpha2.Build{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: rootNamespace,
				},
			},
			nil,
		),
	)
})
