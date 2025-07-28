package k8sklient_test

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/onsi/gomega/gstruct"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authfake "code.cloudfoundry.org/korifi/api/authorization/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("Klient", func() {
	var (
		obj    client.Object
		err    error
		klient *k8sklient.K8sKlient

		userClient            *fake.WithWatch
		userClientFactory     *authfake.UserClientFactory
		nsRetriever           *fake.NamespaceRetriever
		descriptorBasedLister *fake.Lister
	)

	BeforeEach(func() {
		obj = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: uuid.NewString(),
			},
		}

		nsRetriever = new(fake.NamespaceRetriever)
		descriptorBasedLister = new(fake.Lister)

		userClient = new(fake.WithWatch)
		userClientFactory = new(authfake.UserClientFactory)
		userClientFactory.BuildClientReturns(userClient, nil)

		klient = k8sklient.NewK8sKlient(
			nsRetriever,
			userClientFactory,
			descriptorBasedLister,
			scheme.Scheme,
		)
	})

	Describe("Get", func() {
		JustBeforeEach(func() {
			err = klient.Get(ctx, obj)
		})

		It("delegates to the user client", func() {
			Expect(err).NotTo(HaveOccurred())

			Expect(userClientFactory.BuildClientCallCount()).To(Equal(1))
			actualAuthInfo := userClientFactory.BuildClientArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(userClient.GetCallCount()).To(Equal(1))
			_, actualObjectKey, actualObject, actualOpts := userClient.GetArgsForCall(0)
			Expect(actualObjectKey).To(Equal(client.ObjectKeyFromObject(obj)))
			Expect(actualObject).To(Equal(obj))
			Expect(actualOpts).To(BeEmpty())
		})

		When("the user client fails", func() {
			BeforeEach(func() {
				userClient.GetReturns(errors.New("get-err"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(ContainSubstring("get-err")))
			})
		})

		When("the object has no namespace", func() {
			BeforeEach(func() {
				obj = &korifiv1alpha1.CFApp{
					ObjectMeta: metav1.ObjectMeta{
						Name: uuid.NewString(),
					},
				}

				nsRetriever.NamespaceForReturns("test-namespace", nil)
			})

			It("resolves the namespace", func() {
				Expect(nsRetriever.NamespaceForCallCount()).To(Equal(1))
				_, actualGuid, actualResourceType := nsRetriever.NamespaceForArgsForCall(0)
				Expect(actualGuid).To(Equal(obj.GetName()))
				Expect(actualResourceType).To(Equal(repositories.AppResourceType))

				Expect(userClient.GetCallCount()).To(Equal(1))
				_, actualObjectKey, actualObject, _ := userClient.GetArgsForCall(0)
				Expect(actualObjectKey.Namespace).To(Equal("test-namespace"))
				Expect(actualObjectKey.Name).To(Equal(obj.GetName()))
				Expect(actualObject).To(Equal(obj))
			})

			When("resolving the namespace fails", func() {
				BeforeEach(func() {
					nsRetriever.NamespaceForReturns("", errors.New("ns-resolving-err"))
				})

				It("returns the error", func() {
					Expect(err).To(MatchError(ContainSubstring("ns-resolving-err")))
				})
			})

			When("the object is not a Korifi resource", func() {
				BeforeEach(func() {
					obj = &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name: uuid.NewString(),
						},
					}
				})

				It("returns the error", func() {
					Expect(err).To(MatchError(ContainSubstring("unsupported resource type")))
				})
			})
		})

		When("creating the user client fails", func() {
			BeforeEach(func() {
				userClientFactory.BuildClientReturns(nil, errors.New("err-build-client"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(ContainSubstring("err-build-client")))
			})
		})
	})

	Describe("Create", func() {
		JustBeforeEach(func() {
			err = klient.Create(ctx, obj)
		})

		It("delegates to the user client", func() {
			Expect(err).NotTo(HaveOccurred())

			Expect(userClientFactory.BuildClientCallCount()).To(Equal(1))
			actualAuthInfo := userClientFactory.BuildClientArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(userClient.CreateCallCount()).To(Equal(1))
			_, actualObject, actualOpts := userClient.CreateArgsForCall(0)
			Expect(actualObject).To(Equal(obj))
			Expect(actualOpts).To(BeEmpty())
		})

		When("creating the user client fails", func() {
			BeforeEach(func() {
				userClientFactory.BuildClientReturns(nil, errors.New("err-build-client"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(ContainSubstring("err-build-client")))
			})
		})

		When("the user client fails", func() {
			BeforeEach(func() {
				userClient.CreateReturns(errors.New("create-err"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(ContainSubstring("create-err")))
			})
		})
	})

	Describe("Patch", func() {
		var modify func() error

		BeforeEach(func() {
			modify = func() error {
				obj.SetLabels(map[string]string{"foo": "bar"})
				return nil
			}
		})

		JustBeforeEach(func() {
			err = klient.Patch(ctx, obj, modify)
		})

		It("delegates to the user client", func() {
			Expect(err).NotTo(HaveOccurred())

			Expect(userClientFactory.BuildClientCallCount()).To(Equal(1))
			actualAuthInfo := userClientFactory.BuildClientArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(userClient.PatchCallCount()).To(Equal(1))
			_, actualObject, actualPatch, actualOpts := userClient.PatchArgsForCall(0)
			Expect(actualObject).To(Equal(&korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obj.GetName(),
					Namespace: obj.GetNamespace(),
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			}))
			Expect(actualOpts).To(BeEmpty())

			var actualPatchData []byte
			actualPatchData, err = actualPatch.Data(&korifiv1alpha1.CFApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      obj.GetName(),
					Namespace: obj.GetNamespace(),
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(actualPatchData)).To(Equal(`{"metadata":{"labels":{"foo":"bar"}}}`))
		})

		When("the modify function fails", func() {
			BeforeEach(func() {
				modify = func() error {
					return errors.New("modify-err")
				}
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(ContainSubstring("modify-err")))
			})

			It("does not patch the object", func() {
				Expect(userClient.PatchCallCount()).To(Equal(0))
			})
		})

		When("creating the user client fails", func() {
			BeforeEach(func() {
				userClientFactory.BuildClientReturns(nil, errors.New("err-build-client"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(ContainSubstring("err-build-client")))
			})
		})

		When("the user client fails", func() {
			BeforeEach(func() {
				userClient.PatchReturns(errors.New("patch-err"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(ContainSubstring("patch-err")))
			})
		})
	})

	Describe("List", func() {
		var (
			objectList *korifiv1alpha1.CFAppList
			listOpts   []repositories.ListOption
			pageInfo   descriptors.PageInfo
		)

		BeforeEach(func() {
			objectList = &korifiv1alpha1.CFAppList{
				Items: []korifiv1alpha1.CFApp{
					{ObjectMeta: metav1.ObjectMeta{Name: "guid-1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "guid-2"}},
				},
			}
			pageInfo = descriptors.PageInfo{
				TotalResults: 2,
				TotalPages:   2,
				PageNumber:   2,
				PageSize:     1,
			}
			descriptorBasedLister.ListReturns(objectList, pageInfo, nil)

			listOpts = []repositories.ListOption{
				repositories.InNamespace("ns"),
				repositories.WithLabel("my-label", "my-value"),
			}
		})

		JustBeforeEach(func() {
			pageInfo, err = klient.List(ctx, objectList, listOpts...)
		})

		It("delegates to the user client", func() {
			Expect(err).NotTo(HaveOccurred())

			Expect(userClientFactory.BuildClientCallCount()).To(Equal(1))
			actualAuthInfo := userClientFactory.BuildClientArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(userClient.ListCallCount()).To(Equal(1))
			_, actualObjectList, actualOpts := userClient.ListArgsForCall(0)
			Expect(actualObjectList).To(Equal(objectList))

			Expect(actualOpts).To(ConsistOf(&client.ListOptions{
				LabelSelector: parseLabelSelector("my-label=my-value"),
				Namespace:     "ns",
			}))
		})

		When("paging is requested", func() {
			BeforeEach(func() {
				listOpts = append(listOpts, repositories.WithPaging(repositories.Pagination{
					PerPage: 1,
					Page:    2,
				}))
			})

			It("delegates to the descriptor based lister", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(descriptorBasedLister.ListCallCount()).To(Equal(1))
				_, _, actualListOptions := descriptorBasedLister.ListArgsForCall(0)
				Expect(actualListOptions.Paging).To(Equal(&repositories.PagingOpt{
					PageSize:   1,
					PageNumber: 2,
				}))
			})
		})

		When("sorting is requested", func() {
			BeforeEach(func() {
				listOpts = append(listOpts, repositories.WithOrdering("created_at"))
			})

			It("delegates to the descriptor based lister", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(descriptorBasedLister.ListCallCount()).To(Equal(1))
				_, _, actualListOptions := descriptorBasedLister.ListArgsForCall(0)
				Expect(actualListOptions.Sort).To(Equal(&repositories.SortOpt{
					By:   "Created At",
					Desc: false,
				}))
			})
		})

		When("a list option errors", func() {
			BeforeEach(func() {
				listOpts = []repositories.ListOption{repositories.ErroringListOption("list-opt-err")}
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(&k8serrors.StatusError{
					ErrStatus: metav1.Status{
						Message: "list-opt-err",
						Status:  metav1.StatusFailure,
						Code:    http.StatusUnprocessableEntity,
						Reason:  metav1.StatusReasonInvalid,
					},
				}))
			})

			It("does not delegate to the user client", func() {
				Expect(userClient.ListCallCount()).To(Equal(0))
			})
		})
	})

	Describe("Watch", func() {
		var (
			objectWatch *fake.WatchInterface
			actualWatch watch.Interface
			objectList  client.ObjectList
			listOpts    []repositories.ListOption
		)

		BeforeEach(func() {
			objectList = &korifiv1alpha1.CFAppList{}
			listOpts = []repositories.ListOption{}

			objectWatch = new(fake.WatchInterface)
			userClient.WatchReturns(objectWatch, nil)
		})

		JustBeforeEach(func() {
			actualWatch, err = klient.Watch(ctx, objectList, listOpts...)
		})

		It("delegates to the user client", func() {
			Expect(err).NotTo(HaveOccurred())

			Expect(userClientFactory.BuildClientCallCount()).To(Equal(1))
			actualAuthInfo := userClientFactory.BuildClientArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(userClient.WatchCallCount()).To(Equal(1))
			_, actualObjectList, actualOpts := userClient.WatchArgsForCall(0)
			Expect(actualObjectList).To(Equal(objectList))
			Expect(actualOpts).To(ConsistOf(PointTo(BeZero())))

			Expect(actualWatch).To(Equal(objectWatch))
		})

		When("watching with options", func() {
			var (
				spaceGuidsReqs        []labels.Requirement
				expectedFieldSelector fields.Selector
			)

			BeforeEach(func() {
				listOpts = []repositories.ListOption{
					repositories.WithLabelIn(korifiv1alpha1.SpaceGUIDLabelKey, []string{"s1", "s2"}),
					repositories.WithLabelSelector("foo==bar"),
					repositories.InNamespace("in-ns"),
					repositories.MatchingFields{"field": "fieldValue"},
				}
			})

			It("uses a label selector", func() {
				Expect(err).NotTo(HaveOccurred())

				Expect(userClient.WatchCallCount()).To(Equal(1))
				_, _, actualOpts := userClient.WatchArgsForCall(0)

				expectedSelector := parseLabelSelector("foo==bar")

				spaceGuidsReqs, err = labels.ParseToRequirements(fmt.Sprintf("%s in (s1,s2)", korifiv1alpha1.SpaceGUIDLabelKey))
				Expect(err).NotTo(HaveOccurred())
				expectedSelector = expectedSelector.Add(spaceGuidsReqs...)

				expectedFieldSelector, err = fields.ParseSelector("field=fieldValue")
				Expect(err).NotTo(HaveOccurred())
				Expect(actualOpts).To(ConsistOf(PointTo(Equal(client.ListOptions{
					LabelSelector: expectedSelector,
					Namespace:     "in-ns",
					FieldSelector: expectedFieldSelector,
				}))))
			})
		})

		When("creating the user client fails", func() {
			BeforeEach(func() {
				userClientFactory.BuildClientReturns(nil, errors.New("err-build-client"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(ContainSubstring("err-build-client")))
			})
		})

		When("the user client fails", func() {
			BeforeEach(func() {
				userClient.WatchReturns(nil, errors.New("watch-err"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(ContainSubstring("watch-err")))
			})
		})
	})
})

func parseLabelSelector(selectorString string) labels.Selector {
	GinkgoHelper()

	selector, err := labels.Parse(selectorString)
	Expect(err).NotTo(HaveOccurred())
	return selector
}
