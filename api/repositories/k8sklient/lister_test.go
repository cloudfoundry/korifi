package k8sklient_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	descfake "code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors/fake"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("Lister", func() {
	var (
		err              error
		descriptorClient *fake.DescriptorClient
		objectListMapper *fake.ObjectListMapper
		fakeDescriptor   *descfake.ResultSetDescriptor
		listOpts         []repositories.ListOption

		lister    k8sklient.Lister
		listItems client.ObjectList
		pageInfo  descriptors.PageInfo
	)

	BeforeEach(func() {
		descriptorClient = new(fake.DescriptorClient)
		objectListMapper = new(fake.ObjectListMapper)

		fakeDescriptor = new(descfake.ResultSetDescriptor)
		fakeDescriptor.GUIDsReturns([]string{"guid-1", "guid-2"}, nil)
		descriptorClient.ListReturns(fakeDescriptor, nil)

		appsList := &korifiv1alpha1.CFAppList{
			Items: []korifiv1alpha1.CFApp{
				{ObjectMeta: metav1.ObjectMeta{Name: "guid-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "guid-2"}},
			},
		}
		objectListMapper.GUIDsToObjectListReturns(appsList, nil)

		listOpts = []repositories.ListOption{
			repositories.InNamespace("ns"),
			repositories.WithLabel("my-label", "my-value"),
		}

		lister = k8sklient.NewDescriptorsBasedLister(descriptorClient, objectListMapper)
	})

	JustBeforeEach(func() {
		listOpt := repositories.ListOptions{}
		for _, opt := range listOpts {
			Expect(opt.ApplyToList(&listOpt)).To(Succeed())
		}

		appListGVK := schema.GroupVersionKind{
			Group:   "korifi.cloudfoundry.org",
			Version: "v1alpha1",
			Kind:    "CFAppList",
		}
		listItems, pageInfo, err = lister.List(ctx, appListGVK, listOpt)
	})

	It("returns a list of objects", func() {
		Expect(err).NotTo(HaveOccurred())

		Expect(listItems).To(BeAssignableToTypeOf(&korifiv1alpha1.CFAppList{}))
		appList := listItems.(*korifiv1alpha1.CFAppList)
		Expect(appList.Items).To(Equal([]korifiv1alpha1.CFApp{
			{ObjectMeta: metav1.ObjectMeta{Name: "guid-1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "guid-2"}},
		}))
	})

	It("returns a single page info", func() {
		Expect(pageInfo).To(Equal(descriptors.PageInfo{
			TotalResults: 2,
			TotalPages:   1,
			PageNumber:   1,
			PageSize:     2,
		}))
	})

	It("lists object descriptors with user supplied filltering opts", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(descriptorClient.ListCallCount()).To(Equal(1))
		_, gvk, actualListOpts := descriptorClient.ListArgsForCall(0)
		Expect(gvk).To(Equal(schema.GroupVersionKind{
			Group:   "korifi.cloudfoundry.org",
			Version: "v1alpha1",
			Kind:    "CFAppList",
		}))

		Expect(actualListOpts).To(ConsistOf(&client.ListOptions{
			LabelSelector: parseLabelSelector("my-label=my-value"),
			Namespace:     "ns",
		}))
	})

	When("the descriptor client fails", func() {
		BeforeEach(func() {
			descriptorClient.ListReturns(nil, errors.New("list-err"))
		})

		It("returns the error", func() {
			Expect(err).To(MatchError(ContainSubstring("list-err")))
		})
	})

	It("gets the guids from the descriptor", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeDescriptor.GUIDsCallCount()).To(Equal(1))
	})

	When("getting the guids fails", func() {
		BeforeEach(func() {
			fakeDescriptor.GUIDsReturns(nil, errors.New("guids-err"))
		})

		It("returns the error", func() {
			Expect(err).To(MatchError(ContainSubstring("guids-err")))
		})
	})

	It("maps guids to objects", func() {
		Expect(err).NotTo(HaveOccurred())

		Expect(objectListMapper.GUIDsToObjectListCallCount()).To(Equal(1))
		_, actualGVK, actualGUIDs := objectListMapper.GUIDsToObjectListArgsForCall(0)
		Expect(actualGVK).To(Equal(schema.GroupVersionKind{
			Group:   "korifi.cloudfoundry.org",
			Version: "v1alpha1",
			Kind:    "CFAppList",
		}))
		Expect(actualGUIDs).To(Equal([]string{"guid-1", "guid-2"}))
	})

	When("mapping the guids to objects fails", func() {
		BeforeEach(func() {
			objectListMapper.GUIDsToObjectListReturns(nil, errors.New("map-err"))
		})

		It("returns the error", func() {
			Expect(err).To(MatchError(ContainSubstring("map-err")))
		})
	})

	When("mapping the guids to object fails to resolve guids", func() {
		var appList *korifiv1alpha1.CFAppList
		BeforeEach(func() {
			appList = &korifiv1alpha1.CFAppList{
				Items: []korifiv1alpha1.CFApp{
					{ObjectMeta: metav1.ObjectMeta{Name: "guid-1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "guid-2"}},
				},
			}

			objectListMapper.GUIDsToObjectListReturnsOnCall(0, nil, descriptors.ObjectResolutionError{})
			objectListMapper.GUIDsToObjectListReturns(appList, nil)
		})

		It("retries the list and succeeds", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(objectListMapper.GUIDsToObjectListCallCount()).To(Equal(2))

			Expect(listItems).To(BeAssignableToTypeOf(&korifiv1alpha1.CFAppList{}))
			appList := listItems.(*korifiv1alpha1.CFAppList)
			Expect(appList.Items).To(Equal([]korifiv1alpha1.CFApp{
				{ObjectMeta: metav1.ObjectMeta{Name: "guid-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "guid-2"}},
			}))
		})
	})

	When("paging is requested", func() {
		BeforeEach(func() {
			listOpts = append(listOpts, repositories.WithPaging(repositories.Pagination{
				PerPage: 1,
				Page:    2,
			}))
		})

		It("only maps paged guids to objects", func() {
			Expect(err).NotTo(HaveOccurred())

			Expect(objectListMapper.GUIDsToObjectListCallCount()).To(Equal(1))
			_, actualGVK, actualGUIDs := objectListMapper.GUIDsToObjectListArgsForCall(0)
			Expect(actualGVK).To(Equal(schema.GroupVersionKind{
				Group:   "korifi.cloudfoundry.org",
				Version: "v1alpha1",
				Kind:    "CFAppList",
			}))
			Expect(actualGUIDs).To(Equal([]string{"guid-2"}))
		})

		It("returns paged page info", func() {
			Expect(pageInfo).To(Equal(descriptors.PageInfo{
				TotalResults: 2,
				TotalPages:   2,
				PageNumber:   2,
				PageSize:     1,
			}))
		})
	})

	When("sorting is requested", func() {
		BeforeEach(func() {
			listOpts = append(listOpts, repositories.SortOpt{By: "foo", Desc: true})
		})

		It("uses the descriptor client and the object mapper", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(descriptorClient.ListCallCount()).To(Equal(1))
			Expect(fakeDescriptor.GUIDsCallCount()).To(Equal(1))
			Expect(objectListMapper.GUIDsToObjectListCallCount()).To(Equal(1))
		})

		It("sorts the objects in the requested order", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeDescriptor.SortCallCount()).To(Equal(1))
			by, descending := fakeDescriptor.SortArgsForCall(0)
			Expect(by).To(Equal("foo"))
			Expect(descending).To(BeTrue())
			Expect(fakeDescriptor.GUIDsCallCount()).To(Equal(1))
		})

		It("returns a single page info", func() {
			Expect(pageInfo).To(Equal(descriptors.PageInfo{
				TotalResults: 2,
				TotalPages:   1,
				PageNumber:   1,
				PageSize:     2,
			}))
		})

		When("sorting fails", func() {
			BeforeEach(func() {
				fakeDescriptor.SortReturns(errors.New("sort-err"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(ContainSubstring("sort-err")))
			})
		})
	})
})
