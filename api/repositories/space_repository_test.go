package repositories_test

import (
	"context"
	"errors"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("SpaceRepository", func() {
	var (
		orgRepo          *repositories.OrgRepo
		conditionAwaiter *FakeAwaiter[
			*korifiv1alpha1.CFSpace,
			korifiv1alpha1.CFSpaceList,
			*korifiv1alpha1.CFSpaceList,
		]
		spaceRepo *repositories.SpaceRepo
	)

	BeforeEach(func() {
		orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, userClientFactory, nsPerms, &FakeAwaiter[
			*korifiv1alpha1.CFOrg,
			korifiv1alpha1.CFOrgList,
			*korifiv1alpha1.CFOrgList,
		]{})

		conditionAwaiter = &FakeAwaiter[
			*korifiv1alpha1.CFSpace,
			korifiv1alpha1.CFSpaceList,
			*korifiv1alpha1.CFSpaceList,
		]{}
		spaceRepo = repositories.NewSpaceRepo(namespaceRetriever, orgRepo, userClientFactory, nsPerms, conditionAwaiter)
	})

	Describe("CreateSpace", func() {
		var (
			createErr        error
			orgGUID          string
			spaceName        string
			spaceRecord      repositories.SpaceRecord
			conditionStatus  metav1.ConditionStatus
			conditionMessage string
		)

		BeforeEach(func() {
			conditionAwaiter.AwaitConditionStub = func(ctx context.Context, _ client.WithWatch, object client.Object, _ string) (*korifiv1alpha1.CFSpace, error) {
				cfSpace, ok := object.(*korifiv1alpha1.CFSpace)
				Expect(ok).To(BeTrue())

				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   cfSpace.Name,
						Labels: map[string]string{korifiv1alpha1.SpaceNameKey: cfSpace.Spec.DisplayName},
					},
				}
				Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

				Expect(k8s.Patch(ctx, k8sClient, cfSpace, func() {
					cfSpace.Status.GUID = cfSpace.Name
					meta.SetStatusCondition(&cfSpace.Status.Conditions, metav1.Condition{
						Type:    "Ready",
						Status:  conditionStatus,
						Reason:  "blah",
						Message: conditionMessage,
					})
				})).To(Succeed())

				return cfSpace, nil
			}

			spaceName = prefixedGUID("space-name")
			org := createOrgWithCleanup(ctx, prefixedGUID("org"))
			orgGUID = org.Name
			conditionStatus = metav1.ConditionTrue
			conditionMessage = ""
		})

		JustBeforeEach(func() {
			spaceRecord, createErr = spaceRepo.CreateSpace(ctx, authInfo, repositories.CreateSpaceMessage{
				Name:             spaceName,
				OrganizationGUID: orgGUID,
			})
		})

		When("the user doesn't have the admin role", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, orgUserRole.Name, orgGUID)
			})

			It("fails when creating a space", func() {
				Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user has the admin role", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, orgGUID)
			})

			It("awaits the ready condition", func() {
				Expect(createErr).NotTo(HaveOccurred())

				cfSpace := new(korifiv1alpha1.CFSpace)
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: orgGUID, Name: spaceRecord.GUID}, cfSpace)).To(Succeed())

				Expect(conditionAwaiter.AwaitConditionCallCount()).To(Equal(1))
				obj, conditionType := conditionAwaiter.AwaitConditionArgsForCall(0)
				Expect(obj.GetName()).To(Equal(cfSpace.Name))
				Expect(obj.GetNamespace()).To(Equal(orgGUID))
				Expect(conditionType).To(Equal(shared.StatusConditionReady))
			})

			It("creates a CFSpace resource in the org namespace", func() {
				Expect(createErr).NotTo(HaveOccurred())

				spaceCR := new(korifiv1alpha1.CFSpace)
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: orgGUID, Name: spaceRecord.GUID}, spaceCR)).To(Succeed())

				Expect(spaceRecord.Name).To(Equal(spaceName))
				Expect(spaceRecord.GUID).To(HavePrefix("cf-space-"))
				Expect(spaceRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(spaceRecord.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
				Expect(spaceRecord.DeletedAt).To(BeNil())
			})

			When("the space does not become ready", func() {
				BeforeEach(func() {
					conditionAwaiter.AwaitConditionReturns(&korifiv1alpha1.CFSpace{}, errors.New("time-out-err"))
				})

				It("errors", func() {
					Expect(createErr).To(MatchError(ContainSubstring("time-out-err")))
				})
			})

			When("the org does not exist", func() {
				BeforeEach(func() {
					orgGUID = "does-not-exist"
				})

				It("returns an error", func() {
					Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("the client fails to create the space", func() {
				BeforeEach(func() {
					spaceName = "this-string-has-illegal-characters-ц"
				})

				It("fails", func() {
					Expect(createErr).To(HaveOccurred())
				})
			})
		})
	})

	Describe("ListSpaces", func() {
		var cfOrg1, cfOrg2 *korifiv1alpha1.CFOrg
		var space11, space12, space21, space22 *korifiv1alpha1.CFSpace

		BeforeEach(func() {
			ctx = context.Background()

			cfOrg1 = createOrgWithCleanup(ctx, prefixedGUID("org1"))
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg1.Name)
			cfOrg2 = createOrgWithCleanup(ctx, prefixedGUID("org2"))
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg2.Name)

			space11 = createSpaceWithCleanup(ctx, cfOrg1.Name, "space1")
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space11.Name)
			space12 = createSpaceWithCleanup(ctx, cfOrg1.Name, "space2")
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space12.Name)

			space21 = createSpaceWithCleanup(ctx, cfOrg2.Name, "space1")
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space21.Name)
			space22 = createSpaceWithCleanup(ctx, cfOrg2.Name, "space3")
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space22.Name)

			createSpaceWithCleanup(ctx, cfOrg2.Name, "space3")
		})

		It("returns the spaces the user has role bindings in", func() {
			spaces, err := spaceRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{})
			Expect(err).NotTo(HaveOccurred())

			Expect(spaces).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"Name":             Equal("space1"),
					"GUID":             Equal(space11.Name),
					"OrganizationGUID": Equal(cfOrg1.Name),
				}),
				MatchFields(IgnoreExtras, Fields{
					"Name":             Equal("space2"),
					"GUID":             Equal(space12.Name),
					"OrganizationGUID": Equal(cfOrg1.Name),
				}),
				MatchFields(IgnoreExtras, Fields{
					"Name":             Equal("space1"),
					"GUID":             Equal(space21.Name),
					"OrganizationGUID": Equal(cfOrg2.Name),
				}),
				MatchFields(IgnoreExtras, Fields{
					"Name":             Equal("space3"),
					"GUID":             Equal(space22.Name),
					"OrganizationGUID": Equal(cfOrg2.Name),
				}),
			))
		})

		When("the space anchor is not ready", func() {
			BeforeEach(func() {
				meta.SetStatusCondition(&(space11.Status.Conditions), metav1.Condition{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "cus",
					Message: "cus",
				})
				Expect(k8sClient.Status().Update(ctx, space11)).To(Succeed())
			})

			It("does not list it", func() {
				spaces, err := spaceRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{})
				Expect(err).NotTo(HaveOccurred())

				Expect(spaces).NotTo(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"Name":             Equal("space1"),
						"GUID":             Equal(space11.Name),
						"OrganizationGUID": Equal(cfOrg1.Name),
					}),
				))
			})
		})

		When("filtering by org guids", func() {
			It("only returns the spaces belonging to the specified org guids", func() {
				spaces, err := spaceRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
					OrganizationGUIDs: []string{cfOrg1.Name, "does-not-exist"},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(spaces).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name":             Equal("space1"),
						"OrganizationGUID": Equal(cfOrg1.Name),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":             Equal("space2"),
						"OrganizationGUID": Equal(cfOrg1.Name),
					}),
				))
			})
		})

		When("filtering by space names", func() {
			It("only returns the spaces matching the specified names", func() {
				spaces, err := spaceRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
					Names: []string{"space1", "space3", "does-not-exist"},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(spaces).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name":             Equal("space1"),
						"OrganizationGUID": Equal(cfOrg1.Name),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":             Equal("space1"),
						"OrganizationGUID": Equal(cfOrg2.Name),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":             Equal("space3"),
						"OrganizationGUID": Equal(cfOrg2.Name),
					}),
				))
			})
		})

		When("filtering by space guids", func() {
			It("only returns the spaces matching the specified guids", func() {
				spaces, err := spaceRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
					GUIDs: []string{space11.Name, space21.Name, "does-not-exist"},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(spaces).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name":             Equal("space1"),
						"OrganizationGUID": Equal(cfOrg1.Name),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":             Equal("space1"),
						"OrganizationGUID": Equal(cfOrg2.Name),
					}),
				))
			})
		})

		When("filtering by org guids, space names and space guids", func() {
			It("only returns the spaces matching the specified names", func() {
				spaces, err := spaceRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
					OrganizationGUIDs: []string{cfOrg1.Name, cfOrg2.Name},
					Names:             []string{"space1", "space2", "space4"},
					GUIDs:             []string{space11.Name, space21.Name},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(spaces).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name":             Equal("space1"),
						"OrganizationGUID": Equal(cfOrg1.Name),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":             Equal("space1"),
						"OrganizationGUID": Equal(cfOrg2.Name),
					}),
				))
			})
		})

		When("filtering by space names that don't exist", func() {
			It("only returns the spaces matching the specified names", func() {
				spaces, err := spaceRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
					Names: []string{"does-not-exist", "still-does-not-exist"},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(spaces).To(BeEmpty())
			})
		})

		When("filtering by org guids that don't exist", func() {
			It("only returns the spaces matching the specified names", func() {
				spaces, err := spaceRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
					OrganizationGUIDs: []string{"does-not-exist", "still-does-not-exist"},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(spaces).To(BeEmpty())
			})
		})

		When("an org exists with a rolebinding for the user, but without permission to list spaces", func() {
			var org *korifiv1alpha1.CFOrg

			BeforeEach(func() {
				org = createOrgWithCleanup(ctx, "org-without-list-space-perm")
				createRoleBinding(ctx, userName, rootNamespaceUserRole.Name, org.Name)
			})

			It("returns the 4 spaces", func() {
				spaces, err := spaceRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{})
				Expect(err).NotTo(HaveOccurred())

				Expect(spaces).To(HaveLen(4))
			})
		})
	})

	Describe("GetSpace", func() {
		var (
			cfOrg   *korifiv1alpha1.CFOrg
			cfSpace *korifiv1alpha1.CFSpace
		)

		BeforeEach(func() {
			cfOrg = createOrgWithCleanup(ctx, "the-org")
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg.Name)
			cfSpace = createSpaceWithCleanup(ctx, cfOrg.Name, "the-space")
		})

		When("the user has a role binding in the space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("gets the space resource", func() {
				spaceRecord, err := spaceRepo.GetSpace(ctx, authInfo, cfSpace.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(spaceRecord.Name).To(Equal("the-space"))
				Expect(spaceRecord.OrganizationGUID).To(Equal(cfOrg.Name))
			})
		})

		When("the user does not have a role binding in the space", func() {
			It("errors", func() {
				_, err := spaceRepo.GetSpace(ctx, authInfo, "the-space")
				Expect(err).To(MatchError(ContainSubstring("not found")))
			})
		})

		When("the space doesn't exist", func() {
			It("errors", func() {
				_, err := spaceRepo.GetSpace(ctx, authInfo, "non-existent-space")
				Expect(err).To(MatchError(ContainSubstring("not found")))
			})
		})
	})

	Describe("DeleteSpace", func() {
		var (
			cfOrg   *korifiv1alpha1.CFOrg
			cfSpace *korifiv1alpha1.CFSpace
		)

		BeforeEach(func() {
			cfOrg = createOrgWithCleanup(ctx, prefixedGUID("org"))
			cfSpace = createSpaceWithCleanup(ctx, cfOrg.Name, "the-space")
		})

		When("the user has permission to delete spaces", func() {
			BeforeEach(func() {
				beforeCtx := context.Background()
				createRoleBinding(beforeCtx, userName, adminRole.Name, cfSpace.Namespace)
			})

			It("deletes the space resource", func() {
				err := spaceRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
					GUID:             cfSpace.Name,
					OrganizationGUID: cfOrg.Name,
				})
				Expect(err).NotTo(HaveOccurred())

				foundCFSpace := &korifiv1alpha1.CFSpace{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: cfOrg.Name, Name: cfSpace.Name}, foundCFSpace)
				Expect(err).To(MatchError(ContainSubstring("not found")))
			})

			When("the space doesn't exist", func() {
				It("errors", func() {
					err := spaceRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
						GUID:             "non-existent-space",
						OrganizationGUID: cfOrg.Name,
					})
					Expect(err).To(MatchError(ContainSubstring("not found")))
				})
			})
		})

		When("the user does not have permission to delete spaces", func() {
			It("errors with forbidden", func() {
				err := spaceRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
					GUID:             cfSpace.Name,
					OrganizationGUID: cfOrg.Name,
				})
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})

			When("the space doesn't exist", func() {
				It("errors with forbidden", func() {
					err := spaceRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
						GUID:             "non-existent-space",
						OrganizationGUID: cfOrg.Name,
					})
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})
			})
		})
	})

	Describe("PatchSpaceMetadata", func() {
		var (
			spaceGUID                     string
			orgGUID                       string
			cfSpace                       *korifiv1alpha1.CFSpace
			cfOrg                         *korifiv1alpha1.CFOrg
			labelsPatch, annotationsPatch map[string]*string
			patchErr                      error
			spaceRecord                   repositories.SpaceRecord
		)

		BeforeEach(func() {
			cfOrg = createOrgWithCleanup(ctx, prefixedGUID("org"))
			orgGUID = cfOrg.Name
			cfSpace = createSpaceWithCleanup(ctx, cfOrg.Name, "the-space")
			spaceGUID = cfSpace.Name
			labelsPatch = nil
			annotationsPatch = nil
		})

		JustBeforeEach(func() {
			patchMsg := repositories.PatchSpaceMetadataMessage{
				GUID:    spaceGUID,
				OrgGUID: orgGUID,
				MetadataPatch: repositories.MetadataPatch{
					Annotations: annotationsPatch,
					Labels:      labelsPatch,
				},
			}

			spaceRecord, patchErr = spaceRepo.PatchSpaceMetadata(ctx, authInfo, patchMsg)
		})

		When("the user is authorized and the space exists", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, orgGUID)
			})

			When("the space doesn't have any labels or annotations", func() {
				BeforeEach(func() {
					labelsPatch = map[string]*string{
						"key-one": tools.PtrTo("value-one"),
						"key-two": tools.PtrTo("value-two"),
					}
					annotationsPatch = map[string]*string{
						"key-one": tools.PtrTo("value-one"),
						"key-two": tools.PtrTo("value-two"),
					}
					Expect(k8s.PatchResource(ctx, k8sClient, cfSpace, func() {
						cfSpace.Labels = nil
						cfSpace.Annotations = nil
					})).To(Succeed())
				})

				It("returns the updated org record", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					Expect(spaceRecord.GUID).To(Equal(spaceGUID))
					Expect(spaceRecord.OrganizationGUID).To(Equal(orgGUID))
					Expect(spaceRecord.Labels).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
					Expect(spaceRecord.Annotations).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
				})

				It("sets the k8s CFSpace resource", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					updatedCFSpace := new(korifiv1alpha1.CFSpace)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfSpace), updatedCFSpace)).To(Succeed())
					Expect(updatedCFSpace.Labels).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
					Expect(updatedCFSpace.Annotations).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
				})
			})

			When("the space already has labels and annotations", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, k8sClient, cfSpace, func() {
						cfSpace.Labels = map[string]string{
							"before-key-one": "value-one",
							"before-key-two": "value-two",
							"key-one":        "value-one",
						}
						cfSpace.Annotations = map[string]string{
							"before-key-one": "value-one",
							"before-key-two": "value-two",
							"key-one":        "value-one",
						}
					})).To(Succeed())

					labelsPatch = map[string]*string{
						"key-one":        tools.PtrTo("value-one-updated"),
						"key-two":        tools.PtrTo("value-two"),
						"before-key-two": nil,
					}
					annotationsPatch = map[string]*string{
						"key-one":        tools.PtrTo("value-one-updated"),
						"key-two":        tools.PtrTo("value-two"),
						"before-key-two": nil,
					}
				})

				It("returns the updated org record", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					Expect(spaceRecord.GUID).To(Equal(spaceGUID))
					Expect(spaceRecord.OrganizationGUID).To(Equal(orgGUID))
					Expect(spaceRecord.Labels).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
					Expect(spaceRecord.Annotations).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
				})

				It("sets the k8s CFSpace resource", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					updatedCFSpace := new(korifiv1alpha1.CFSpace)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfSpace), updatedCFSpace)).To(Succeed())
					Expect(updatedCFSpace.Labels).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
					Expect(updatedCFSpace.Annotations).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
				})
			})

			When("an annotation is invalid", func() {
				BeforeEach(func() {
					annotationsPatch = map[string]*string{
						"-bad-annotation": tools.PtrTo("stuff"),
					}
				})

				It("returns an UnprocessableEntityError", func() {
					var unprocessableEntityError apierrors.UnprocessableEntityError
					Expect(errors.As(patchErr, &unprocessableEntityError)).To(BeTrue())
					Expect(unprocessableEntityError.Detail()).To(SatisfyAll(
						ContainSubstring("metadata.annotations is invalid"),
						ContainSubstring(`"-bad-annotation"`),
						ContainSubstring("alphanumeric"),
					))
				})
			})

			When("a label is invalid", func() {
				BeforeEach(func() {
					labelsPatch = map[string]*string{
						"-bad-label": tools.PtrTo("stuff"),
					}
				})

				It("returns an UnprocessableEntityError", func() {
					var unprocessableEntityError apierrors.UnprocessableEntityError
					Expect(errors.As(patchErr, &unprocessableEntityError)).To(BeTrue())
					Expect(unprocessableEntityError.Detail()).To(SatisfyAll(
						ContainSubstring("metadata.labels is invalid"),
						ContainSubstring(`"-bad-label"`),
						ContainSubstring("alphanumeric"),
					))
				})
			})
		})

		When("the user is authorized but the Space does not exist", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, orgGUID)
				spaceGUID = "invalidSpaceGUID"
			})

			It("fails to get the Space", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

		When("the user is not authorized", func() {
			It("return a forbidden error", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("GetDeletedAt", func() {
		var (
			cfSpace      *korifiv1alpha1.CFSpace
			deletionTime *time.Time
			getErr       error
		)

		BeforeEach(func() {
			cfOrg := createOrgWithCleanup(ctx, "the-org")
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg.Name)
			cfSpace = createSpaceWithCleanup(ctx, cfOrg.Name, "the-space")
		})

		JustBeforeEach(func() {
			deletionTime, getErr = spaceRepo.GetDeletedAt(ctx, authInfo, cfSpace.Name)
		})

		It("returns nil", func() {
			Expect(getErr).NotTo(HaveOccurred())
			Expect(deletionTime).To(BeNil())
		})

		When("the space is being deleted", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, k8sClient, cfSpace, func() {
					cfSpace.Finalizers = append(cfSpace.Finalizers, "foo")
				})).To(Succeed())

				Expect(k8sClient.Delete(ctx, cfSpace)).To(Succeed())
			})

			It("returns the deletion time", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(deletionTime).To(PointTo(BeTemporally("~", time.Now(), time.Minute)))
			})
		})

		When("the space isn't found", func() {
			BeforeEach(func() {
				Expect(k8sClient.Delete(ctx, cfSpace)).To(Succeed())
			})

			It("errors", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})
})
