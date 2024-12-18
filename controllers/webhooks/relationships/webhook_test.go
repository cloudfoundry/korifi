package relationships_test

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Setting the space-guid label", func() {
	var testObjects []client.Object

	BeforeEach(func() {
		spaceNamespace := uuid.NewString()
		Expect(adminClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: spaceNamespace,
			},
		})).To(Succeed())

		testObjects = []client.Object{
			&korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: spaceNamespace,
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
			&korifiv1alpha1.CFBuild{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: spaceNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFBuildSpec{
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			},
			&korifiv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: spaceNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFPackageSpec{
					Type: "bits",
				},
			},
			&korifiv1alpha1.CFProcess{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: spaceNamespace,
					Name:      uuid.NewString(),
				},
			},
			&korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: spaceNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					DomainRef: corev1.ObjectReference{
						Name:      uuid.NewString(),
						Namespace: spaceNamespace,
					},
					Host: "myhost",
				},
			},
			&korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: spaceNamespace,
					Name:      uuid.NewString(),
				},
			},
			&korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: spaceNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					Type: "user-provided",
				},
			},
			&korifiv1alpha1.CFTask{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: spaceNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFTaskSpec{
					Command: "foo",
					AppRef: corev1.LocalObjectReference{
						Name: "bob",
					},
				},
			},
		}

		for _, o := range testObjects {
			Expect(adminClient.Create(context.Background(), o)).To(Succeed())
		}
	})

	Describe("create", func() {
		It("sets space-guid label", func() {
			for _, o := range testObjects {
				Expect(o.GetLabels()).To(HaveKeyWithValue(korifiv1alpha1.SpaceGUIDKey, o.GetNamespace()),
					func() string { return fmt.Sprintf("%T does not have the expected space-guid label", o) },
				)
			}
		})
	})

	Describe("updates", func() {
		It("rejects updating space-guid label", func() {
			for _, o := range testObjects {
				o.SetLabels(
					tools.SetMapValue(o.GetLabels(), korifiv1alpha1.SpaceGUIDKey, "new-space-guid"),
				)
				err := adminClient.Update(ctx, o)
				Expect(err).To(MatchError(ContainSubstring("immutable")),
					func() string { return fmt.Sprintf("%T had its space-guid updated", o) },
				)
			}
		})
	})
})
