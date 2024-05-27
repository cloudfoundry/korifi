package repositories_test

import (
	"context"
	"errors"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fakeawaiter"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
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

var _ = Describe("OrgRepository", func() {
	var (
		conditionAwaiter *fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFOrg,
			korifiv1alpha1.CFOrgList,
			*korifiv1alpha1.CFOrgList,
		]
		orgRepo *repositories.OrgRepo
	)

	BeforeEach(func() {
		conditionAwaiter = &fakeawaiter.FakeAwaiter[
			*korifiv1alpha1.CFOrg,
			korifiv1alpha1.CFOrgList,
			*korifiv1alpha1.CFOrgList,
		]{}
		orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, userClientFactory, nsPerms, conditionAwaiter)
	})

	Describe("CreateOrg", func() {
		var (
			createErr        error
			orgGUID          string
			orgRecord        repositories.OrgRecord
			conditionStatus  metav1.ConditionStatus
			conditionMessage string
		)

		BeforeEach(func() {
			conditionAwaiter.AwaitConditionStub = func(ctx context.Context, _ client.WithWatch, object client.Object, _ string) (*korifiv1alpha1.CFOrg, error) {
				cfOrg, ok := object.(*korifiv1alpha1.CFOrg)
				Expect(ok).To(BeTrue())

				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   cfOrg.Name,
						Labels: map[string]string{korifiv1alpha1.OrgNameKey: cfOrg.Spec.DisplayName},
					},
				}
				Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

				Expect(k8s.Patch(ctx, k8sClient, cfOrg, func() {
					cfOrg.Status.GUID = cfOrg.Name
					meta.SetStatusCondition(&cfOrg.Status.Conditions, metav1.Condition{
						Type:    korifiv1alpha1.StatusConditionReady,
						Status:  conditionStatus,
						Reason:  "blah",
						Message: conditionMessage,
					})
				})).To(Succeed())

				return cfOrg, nil
			}

			orgGUID = prefixedGUID("org")
			conditionStatus = metav1.ConditionTrue
			conditionMessage = ""
		})

		JustBeforeEach(func() {
			orgRecord, createErr = orgRepo.CreateOrg(ctx, authInfo, repositories.CreateOrgMessage{
				Name: orgGUID,
				Labels: map[string]string{
					"test-label-key": "test-label-val",
				},
				Annotations: map[string]string{
					"test-annotation-key": "test-annotation-val",
				},
			})
		})

		It("fails because forbidden", func() {
			Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user has the admin role", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			It("returns an Org record", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(orgRecord.Name).To(Equal(orgGUID))
				Expect(orgRecord.GUID).To(HavePrefix("cf-org-"))
				Expect(orgRecord.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(orgRecord.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
				Expect(orgRecord.DeletedAt).To(BeNil())
				Expect(orgRecord.Labels).To(Equal(map[string]string{"test-label-key": "test-label-val"}))
				Expect(orgRecord.Annotations).To(Equal(map[string]string{"test-annotation-key": "test-annotation-val"}))
			})

			It("creates a CFOrg resource in the root namespace", func() {
				Expect(createErr).NotTo(HaveOccurred())

				cfOrg := new(korifiv1alpha1.CFOrg)
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: rootNamespace, Name: orgRecord.GUID}, cfOrg)).To(Succeed())

				Expect(cfOrg.Spec.DisplayName).To(Equal(orgGUID))
				Expect(cfOrg.Labels).To(Equal(map[string]string{"test-label-key": "test-label-val"}))
				Expect(cfOrg.Annotations).To(Equal(map[string]string{"test-annotation-key": "test-annotation-val"}))
			})

			It("awaits the ready condition", func() {
				Expect(createErr).NotTo(HaveOccurred())

				cfOrg := new(korifiv1alpha1.CFOrg)
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: rootNamespace, Name: orgRecord.GUID}, cfOrg)).To(Succeed())

				Expect(conditionAwaiter.AwaitConditionCallCount()).To(Equal(1))
				obj, conditionType := conditionAwaiter.AwaitConditionArgsForCall(0)
				Expect(obj.GetName()).To(Equal(cfOrg.Name))
				Expect(obj.GetNamespace()).To(Equal(rootNamespace))
				Expect(conditionType).To(Equal(korifiv1alpha1.StatusConditionReady))
			})

			When("the org does not become ready", func() {
				BeforeEach(func() {
					conditionAwaiter.AwaitConditionReturns(&korifiv1alpha1.CFOrg{}, errors.New("time-out-err"))
				})

				It("errors", func() {
					Expect(createErr).To(MatchError(ContainSubstring("time-out-err")))
				})
			})

			When("the client fails to create the org", func() {
				BeforeEach(func() {
					orgGUID = "this-string-has-illegal-characters-ц"
				})

				It("returns an error", func() {
					Expect(createErr).To(HaveOccurred())
				})
			})
		})
	})

	Describe("ListOrgs", func() {
		var cfOrg1, cfOrg2, cfOrg3 *korifiv1alpha1.CFOrg

		BeforeEach(func() {
			ctx = context.Background()

			cfOrg1 = createOrgWithCleanup(ctx, prefixedGUID("org1"))
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg1.Name)
			cfOrg2 = createOrgWithCleanup(ctx, prefixedGUID("org2"))
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg2.Name)
			cfOrg3 = createOrgWithCleanup(ctx, prefixedGUID("org3"))
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg3.Name)
			createOrgWithCleanup(ctx, prefixedGUID("org4"))
		})

		It("returns the 3 orgs", func() {
			orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{})
			Expect(err).NotTo(HaveOccurred())

			Expect(orgs).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal(cfOrg1.Spec.DisplayName),
					"GUID": Equal(cfOrg1.Name),
				}),
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal(cfOrg2.Spec.DisplayName),
					"GUID": Equal(cfOrg2.Name),
				}),
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal(cfOrg3.Spec.DisplayName),
					"GUID": Equal(cfOrg3.Name),
				}),
			))
		})

		When("the org is not ready", func() {
			BeforeEach(func() {
				meta.SetStatusCondition(&(cfOrg1.Status.Conditions), metav1.Condition{
					Type:    korifiv1alpha1.StatusConditionReady,
					Status:  metav1.ConditionFalse,
					Reason:  "because",
					Message: "because",
				})
				Expect(k8sClient.Status().Update(ctx, cfOrg1)).To(Succeed())

				meta.SetStatusCondition(&(cfOrg2.Status.Conditions), metav1.Condition{
					Type:    korifiv1alpha1.StatusConditionReady,
					Status:  metav1.ConditionUnknown,
					Reason:  "because",
					Message: "because",
				})
				Expect(k8sClient.Status().Update(ctx, cfOrg2)).To(Succeed())
			})

			It("does not list it", func() {
				orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{})
				Expect(err).NotTo(HaveOccurred())

				Expect(orgs).NotTo(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(cfOrg1.Name),
					}),
				))
				Expect(orgs).NotTo(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(cfOrg2.Name),
					}),
				))
				Expect(orgs).To(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(cfOrg3.Name),
					}),
				))
			})
		})

		When("we filter for names org1 and org3", func() {
			It("returns just those", func() {
				orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{Names: []string{cfOrg1.Spec.DisplayName, cfOrg3.Spec.DisplayName}})
				Expect(err).NotTo(HaveOccurred())

				Expect(orgs).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal(cfOrg1.Spec.DisplayName),
						"GUID": Equal(cfOrg1.Name),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal(cfOrg3.Spec.DisplayName),
						"GUID": Equal(cfOrg3.Name),
					}),
				))
			})
		})

		When("we filter for guids org1 and org2", func() {
			It("returns just those", func() {
				orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{GUIDs: []string{cfOrg1.Name, cfOrg2.Name}})
				Expect(err).NotTo(HaveOccurred())

				Expect(orgs).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal(cfOrg1.Spec.DisplayName),
						"GUID": Equal(cfOrg1.Name),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal(cfOrg2.Spec.DisplayName),
						"GUID": Equal(cfOrg2.Name),
					}),
				))
			})
		})

		When("fetching authorized namespaces fails", func() {
			var listErr error

			BeforeEach(func() {
				_, listErr = orgRepo.ListOrgs(ctx, authorization.Info{}, repositories.ListOrgsMessage{Names: []string{cfOrg1.Spec.DisplayName, cfOrg3.Spec.DisplayName}})
			})

			It("returns the error", func() {
				Expect(listErr).To(MatchError(ContainSubstring("failed to get identity")))
			})
		})
	})

	Describe("GetOrg", func() {
		var cfOrg *korifiv1alpha1.CFOrg

		BeforeEach(func() {
			cfOrg = createOrgWithCleanup(ctx, prefixedGUID("the-org"))
			Expect(k8s.PatchResource(ctx, k8sClient, cfOrg, func() {
				cfOrg.Labels = map[string]string{
					"test-label-key": "test-label-val",
				}
				cfOrg.Annotations = map[string]string{
					"test-annotation-key": "test-annotation-val",
				}
			})).To(Succeed())
		})

		When("the user has a role binding in the org", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg.Name)
			})

			It("gets the org", func() {
				orgRecord, err := orgRepo.GetOrg(ctx, authInfo, cfOrg.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(orgRecord.Name).To(Equal(cfOrg.Spec.DisplayName))
				Expect(orgRecord.Labels).To(Equal(map[string]string{"test-label-key": "test-label-val"}))
				Expect(orgRecord.Annotations).To(Equal(map[string]string{"test-annotation-key": "test-annotation-val"}))
			})
		})

		When("the user does not have a role binding in the org", func() {
			It("errors", func() {
				_, err := orgRepo.GetOrg(ctx, authInfo, "the-org")
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

		When("the org isn't found", func() {
			It("errors", func() {
				_, err := orgRepo.GetOrg(ctx, authInfo, "non-existent-org")
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("GetDeletedAt", func() {
		var (
			cfOrg        *korifiv1alpha1.CFOrg
			deletionTime *time.Time
			getErr       error
		)

		BeforeEach(func() {
			cfOrg = createOrgWithCleanup(ctx, prefixedGUID("the-org"))
		})

		JustBeforeEach(func() {
			deletionTime, getErr = orgRepo.GetDeletedAt(ctx, authInfo, cfOrg.Name)
		})

		When("the user has a role binding in the org", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg.Name)
			})

			It("returns nil", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(deletionTime).To(BeNil())
			})

			When("the org is being deleted", func() {
				// This case occurs briefly between the CFOrg starting to delete and the finalizer deleting
				// the roles in the org namespace. Once the finalizer deletes the roles, we'll be in the
				// "the user does not have a role binding in the org" case below
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, k8sClient, cfOrg, func() {
						cfOrg.Finalizers = append(cfOrg.Finalizers, "foo")
					})).To(Succeed())

					Expect(k8sClient.Delete(ctx, cfOrg)).To(Succeed())
				})

				It("returns the deletion time", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(deletionTime).To(PointTo(BeTemporally("~", time.Now(), time.Minute)))
				})
			})
		})

		When("the user does not have a role binding in the org", func() {
			When("the org is not being deleted", func() {
				It("errors", func() {
					Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("the org is being deleted", func() {
				// This case occurs in 2 situations:
				//   1. The user never has access to the Org, but another user deleted it
				//   2. The user had access to the Org, deleted it, but the CFOrg finalizer has already
				//      deleted their role bindings
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, k8sClient, cfOrg, func() {
						cfOrg.Finalizers = append(cfOrg.Finalizers, "foo")
					})).To(Succeed())

					Expect(k8sClient.Delete(ctx, cfOrg)).To(Succeed())
				})

				It("returns the deletion time", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(deletionTime).To(PointTo(BeTemporally("~", time.Now(), time.Minute)))
				})
			})
		})

		When("the org isn't found", func() {
			BeforeEach(func() {
				Expect(k8sClient.Delete(ctx, cfOrg)).To(Succeed())
			})

			It("errors", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("DeleteOrg", func() {
		var cfOrg *korifiv1alpha1.CFOrg

		BeforeEach(func() {
			cfOrg = createOrgWithCleanup(ctx, prefixedGUID("org"))
		})

		When("the user has permission to delete orgs", func() {
			BeforeEach(func() {
				beforeCtx := context.Background()
				createRoleBinding(beforeCtx, userName, adminRole.Name, cfOrg.Namespace)
				// Controllers don't exist in env-test environments, we manually copy role bindings to child ns.
				createRoleBinding(beforeCtx, userName, adminRole.Name, cfOrg.Name)
			})

			When("on the happy path", func() {
				It("deletes the CF Org resource", func() {
					err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
						GUID: cfOrg.Name,
					})
					Expect(err).NotTo(HaveOccurred())

					foundCFOrg := &korifiv1alpha1.CFOrg{}
					err = k8sClient.Get(ctx, client.ObjectKey{Namespace: rootNamespace, Name: cfOrg.Name}, foundCFOrg)
					Expect(err).To(MatchError(ContainSubstring("not found")))
				})
			})

			When("the org doesn't exist", func() {
				It("errors", func() {
					err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
						GUID: "non-existent-org",
					})
					Expect(err).To(MatchError(ContainSubstring("not found")))
				})
			})
		})

		When("the user does not have permission to delete orgs", func() {
			It("errors with forbidden", func() {
				err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
					GUID: cfOrg.Name,
				})
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})

			When("the org doesn't exist", func() {
				It("errors with forbidden", func() {
					err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
						GUID: "non-existent-org",
					})
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})
			})
		})
	})

	Describe("PatchOrgMetadata", func() {
		var (
			orgGUID                       string
			cfOrg                         *korifiv1alpha1.CFOrg
			patchErr                      error
			orgRecord                     repositories.OrgRecord
			labelsPatch, annotationsPatch map[string]*string
		)

		BeforeEach(func() {
			cfOrg = createOrgWithCleanup(ctx, prefixedGUID("org-name"))
			orgGUID = cfOrg.Name
			labelsPatch = nil
			annotationsPatch = nil
		})

		JustBeforeEach(func() {
			patchMsg := repositories.PatchOrgMetadataMessage{
				GUID: orgGUID,
				MetadataPatch: repositories.MetadataPatch{
					Annotations: annotationsPatch,
					Labels:      labelsPatch,
				},
			}

			orgRecord, patchErr = orgRepo.PatchOrgMetadata(ctx, authInfo, patchMsg)
		})

		When("the user is authorized and an org exists", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			When("the org doesn't have any labels or annotations", func() {
				BeforeEach(func() {
					labelsPatch = map[string]*string{
						"key-one": tools.PtrTo("value-one"),
						"key-two": tools.PtrTo("value-two"),
					}
					annotationsPatch = map[string]*string{
						"key-one": tools.PtrTo("value-one"),
						"key-two": tools.PtrTo("value-two"),
					}
					Expect(k8s.PatchResource(ctx, k8sClient, cfOrg, func() {
						cfOrg.Labels = nil
						cfOrg.Annotations = nil
					})).To(Succeed())
				})

				It("returns the updated org record", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					Expect(orgRecord.GUID).To(Equal(orgGUID))
					Expect(orgRecord.Labels).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
					Expect(orgRecord.Annotations).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
				})

				It("sets the k8s CFOrg resource", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					updatedCFOrg := new(korifiv1alpha1.CFOrg)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfOrg), updatedCFOrg)).To(Succeed())
					Expect(updatedCFOrg.Labels).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
					Expect(updatedCFOrg.Annotations).To(Equal(
						map[string]string{
							"key-one": "value-one",
							"key-two": "value-two",
						},
					))
				})
			})

			When("the org already has labels and annotations", func() {
				BeforeEach(func() {
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
					Expect(k8s.PatchResource(ctx, k8sClient, cfOrg, func() {
						cfOrg.Labels = map[string]string{
							"before-key-one": "value-one",
							"before-key-two": "value-two",
							"key-one":        "value-one",
						}
						cfOrg.Annotations = map[string]string{
							"before-key-one": "value-one",
							"before-key-two": "value-two",
							"key-one":        "value-one",
						}
					})).To(Succeed())
				})

				It("returns the updated org record", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					Expect(orgRecord.GUID).To(Equal(cfOrg.Name))
					Expect(orgRecord.Labels).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
					Expect(orgRecord.Annotations).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
				})

				It("sets the k8s CFOrg resource", func() {
					Expect(patchErr).NotTo(HaveOccurred())
					updatedCFOrg := new(korifiv1alpha1.CFOrg)
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfOrg), updatedCFOrg)).To(Succeed())
					Expect(updatedCFOrg.Labels).To(Equal(
						map[string]string{
							"before-key-one": "value-one",
							"key-one":        "value-one-updated",
							"key-two":        "value-two",
						},
					))
					Expect(updatedCFOrg.Annotations).To(Equal(
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

		When("the user is authorized but the Org does not exist", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
				orgGUID = "invalidOrgGUID"
			})

			It("fails to get the Org", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

		When("the user is not authorized", func() {
			It("return a forbidden error", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})
})
