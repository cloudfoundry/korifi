package descriptors_test

import (
	"fmt"
	"slices"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

var _ = Describe("Mapper", func() {
	var (
		org   *korifiv1alpha1.CFOrg
		space *korifiv1alpha1.CFSpace

		mapper      *descriptors.ObjectListMapper
		objectGUIDs []string
		objectList  client.ObjectList
	)

	BeforeEach(func() {
		org = createOrg(ctx, uuid.NewString())
		space = createSpace(ctx, org.Name, uuid.NewString())

		objectGUIDs = []string{"obj-3", "obj-0", "obj-1"}

		for i := range 5 {
			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: space.Name,
					Name:      fmt.Sprintf("obj-%d", i),
					Labels: map[string]string{
						korifiv1alpha1.SpaceGUIDLabelKey: space.Name,
						korifiv1alpha1.GUIDLabelKey:      fmt.Sprintf("obj-%d", i),
					},
				},
				Spec: korifiv1alpha1.CFAppSpec{
					DisplayName:  fmt.Sprintf("obj-%d", i),
					DesiredState: "STOPPED",
					Lifecycle: korifiv1alpha1.Lifecycle{
						Type: "docker",
					},
				},
			})).To(Succeed())
		}

		mapper = descriptors.NewObjectListMapper(userClientFactory)
	})

	JustBeforeEach(func() {
		gvk := schema.GroupVersionKind{
			Group:   "korifi.cloudfoundry.org",
			Version: "v1alpha1",
			Kind:    "CFAppList",
		}

		var err error
		objectList, err = mapper.GUIDsToObjectList(ctx, gvk, objectGUIDs)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns an empty list", func() {
		Expect(objectList).To(BeAssignableToTypeOf(&korifiv1alpha1.CFAppList{}))
		object := objectList.(*korifiv1alpha1.CFAppList)
		Expect(object.Items).To(BeEmpty())
	})

	When("the user is allowed to list the objects", func() {
		BeforeEach(func() {
			createRoleBinding(ctx, userName, orgUserRole.Name, org.Name)
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
		})

		It("returns a list of objects ordered in the specified order", func() {
			Expect(objectList).To(BeAssignableToTypeOf(&korifiv1alpha1.CFAppList{}))
			object := objectList.(*korifiv1alpha1.CFAppList)
			Expect(object.Items).To(HaveLen(3))

			resultGUIDs := slices.Collect(it.Map(slices.Values(object.Items), func(item korifiv1alpha1.CFApp) string {
				return item.Name
			}))
			Expect(resultGUIDs).To(Equal(objectGUIDs))
		})
	})

	When("the ordered guids slice is empty", func() {
		BeforeEach(func() {
			objectGUIDs = []string{}
		})

		It("returns an empty list", func() {
			Expect(objectList).To(BeAssignableToTypeOf(&korifiv1alpha1.CFAppList{}))
			object := objectList.(*korifiv1alpha1.CFAppList)
			Expect(object.Items).To(BeEmpty())
		})
	})
})
