package descriptors_test

import (
	"fmt"
	"slices"

	"github.com/BooleanCat/go-functional/v2/it"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Mapper", func() {
	var (
		mapper      *descriptors.ObjectListMapper
		objectGUIDs []string
		objectList  client.ObjectList
		mapErr      error
	)

	BeforeEach(func() {
		objectGUIDs = []string{"obj-3", "obj-0", "obj-1"}

		for i := range 5 {
			Expect(k8sClient.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("obj-%d", i),
					Namespace: testNamespace,
					Labels: map[string]string{
						"korifi.cloudfoundry.org/guid": fmt.Sprintf("obj-%d", i),
					},
				},
			})).To(Succeed())
		}

		mapper = descriptors.NewObjectListMapper(k8sClient)
	})

	JustBeforeEach(func() {
		gvk := schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "SecretList",
		}

		objectList, mapErr = mapper.GUIDsToObjectList(ctx, gvk, objectGUIDs)
	})

	FIt("returns a list of objects ordered in the specified order", func() {
		Expect(mapErr).NotTo(HaveOccurred())
		Expect(objectList).To(BeAssignableToTypeOf(&corev1.SecretList{}))
		object := objectList.(*corev1.SecretList)
		Expect(object.Items).To(HaveLen(3))

		resultGUIDs := slices.Collect(it.Map(slices.Values(object.Items), func(item corev1.Secret) string {
			return item.Name
		}))
		Expect(resultGUIDs).To(Equal(objectGUIDs))
	})
})
