package descriptors_test

import (
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Client", func() {
	var (
		org   *korifiv1alpha1.CFOrg
		space *korifiv1alpha1.CFSpace

		descrClient          *descriptors.Client
		listResultDescriptor k8sklient.ResultSetDescriptor
	)

	BeforeEach(func() {
		org = createOrg(ctx, uuid.NewString())
		space = createSpace(ctx, org.Name, uuid.NewString())

		descrClient = descriptors.NewClient(restClient, authorization.NewSpaceFilteringOpts(nsPerms))

		Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      fmt.Sprintf("app-%d", 2),
				Labels: map[string]string{
					korifiv1alpha1.SpaceGUIDKey: space.Name,
					"foo":                       "bar",
				},
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  fmt.Sprintf("application-%d", 2),
				DesiredState: korifiv1alpha1.StoppedState,
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "docker",
				},
			},
		})).To(Succeed())

		Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      fmt.Sprintf("app-%d", 1),
				Labels: map[string]string{
					korifiv1alpha1.SpaceGUIDKey: space.Name,
					"foo":                       "bar",
				},
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  fmt.Sprintf("application-%d", 1),
				DesiredState: korifiv1alpha1.StoppedState,
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "docker",
				},
			},
		})).To(Succeed())

		Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      fmt.Sprintf("app-%d", 3),
				Labels: map[string]string{
					korifiv1alpha1.SpaceGUIDKey: space.Name,
					"foo":                       "bar",
				},
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  fmt.Sprintf("application-%d", 3),
				DesiredState: korifiv1alpha1.StoppedState,
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "docker",
				},
			},
		})).To(Succeed())

		Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      "never-listed-app",
				Labels: map[string]string{
					korifiv1alpha1.SpaceGUIDKey: space.Name,
				},
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  "application-never-listed",
				DesiredState: korifiv1alpha1.StoppedState,
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "docker",
				},
			},
		})).To(Succeed())
	})

	Describe("List", func() {
		var listErr error

		JustBeforeEach(func() {
			gvks, _, err := scheme.Scheme.ObjectKinds(&korifiv1alpha1.CFAppList{})
			Expect(err).NotTo(HaveOccurred())
			Expect(gvks).To(HaveLen(1))

			listResultDescriptor, listErr = descrClient.List(ctx, gvks[0], client.MatchingLabels{"foo": "bar"})
		})

		Describe("sorting", func() {
			var (
				sortColumn  string
				desc        bool
				sortedGUIDs []string
				sortErr     error
			)

			BeforeEach(func() {
				sortColumn = "Display Name"
				desc = false
			})

			JustBeforeEach(func() {
				Expect(listErr).NotTo(HaveOccurred())
				sortedGUIDs, sortErr = listResultDescriptor.SortedGUIDs(sortColumn, desc)
			})

			It("returns an empty list", func() {
				Expect(sortErr).NotTo(HaveOccurred())
				Expect(sortedGUIDs).To(BeEmpty())
			})

			When("the user is allowed to list the objects", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, orgUserRole.Name, org.Name)
					createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
				})

				It("sorts the results", func() {
					Expect(sortErr).NotTo(HaveOccurred())
					Expect(sortedGUIDs).To(Equal([]string{"app-1", "app-2", "app-3"}))
				})

				When("sorting descending", func() {
					BeforeEach(func() {
						desc = true
					})

					It("sorts the results in descending order", func() {
						Expect(sortErr).NotTo(HaveOccurred())
						Expect(sortedGUIDs).To(Equal([]string{"app-3", "app-2", "app-1"}))
					})
				})

				When("the sort column does not exist", func() {
					BeforeEach(func() {
						sortColumn = "non-existing-column"
					})

					It("fails", func() {
						Expect(sortErr).To(MatchError(ContainSubstring("not found")))
					})
				})
			})
		})
	})
})
