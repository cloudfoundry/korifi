package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("OrgRepository", func() {
	var orgRepo *repositories.OrgRepo

	BeforeEach(func() {
		orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, userClientFactory, nsPerms, time.Millisecond*2000)
	})

	Describe("CreateOrg", func() {
		var (
			createErr                 error
			orgGUID                   string
			org                       repositories.OrgRecord
			done                      chan bool
			doOrgControllerSimulation bool
		)

		waitForCFOrg := func(anchorNamespace string, orgName string, done chan bool) (*korifiv1alpha1.CFOrg, error) {
			for {
				select {
				case <-done:
					return nil, fmt.Errorf("waitForCFOrg was 'signalled' to stop polling")
				default:
				}

				var orgList korifiv1alpha1.CFOrgList
				err := k8sClient.List(ctx, &orgList, client.InNamespace(anchorNamespace))
				if err != nil {
					return nil, fmt.Errorf("waitForCFOrg failed")
				}

				var matches []korifiv1alpha1.CFOrg
				for _, org := range orgList.Items {
					if org.Spec.DisplayName == orgName {
						matches = append(matches, org)
					}
				}
				if len(matches) > 1 {
					return nil, fmt.Errorf("waitForCFOrg found multiple anchors")
				}
				if len(matches) == 1 {
					return &matches[0], nil
				}

				time.Sleep(time.Millisecond * 100)
			}
		}

		simulateOrgController := func(anchorNamespace string, orgName string, done chan bool) {
			defer GinkgoRecover()

			org, err := waitForCFOrg(anchorNamespace, orgName, done)
			if err != nil {
				return
			}

			createNamespace(ctx, anchorNamespace, org.Name, map[string]string{korifiv1alpha1.OrgNameLabel: org.Spec.DisplayName})

			meta.SetStatusCondition(&(org.Status.Conditions), metav1.Condition{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "blah",
				Message: "blah",
			})
			Expect(
				k8sClient.Status().Update(ctx, org),
			).To(Succeed())
		}

		BeforeEach(func() {
			doOrgControllerSimulation = true
			done = make(chan bool, 1)
			orgGUID = prefixedGUID("org")
		})

		JustBeforeEach(func() {
			if doOrgControllerSimulation {
				go simulateOrgController(rootNamespace, orgGUID, done)
			}
			org, createErr = orgRepo.CreateOrg(ctx, authInfo, repositories.CreateOrgMessage{
				Name: orgGUID,
				Labels: map[string]string{
					"test-label-key": "test-label-val",
				},
				Annotations: map[string]string{
					"test-annotation-key": "test-annotation-val",
				},
			})
		})

		AfterEach(func() {
			done <- true
		})

		When("the user doesn't have the admin role", func() {
			It("fails when creating an org", func() {
				Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user has the admin role", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			It("returns an Org record", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(org.Name).To(Equal(orgGUID))
				Expect(org.GUID).To(HavePrefix("cf-org-"))
				createdAt, err := time.Parse(time.RFC3339, org.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdAt).To(BeTemporally("~", time.Now(), 2*time.Second))
				updatedAt, err := time.Parse(time.RFC3339, org.UpdatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
				Expect(org.Labels).To(Equal(map[string]string{"test-label-key": "test-label-val"}))
				Expect(org.Annotations).To(Equal(map[string]string{"test-annotation-key": "test-annotation-val"}))
			})

			It("creates a CFOrg resource in the root namespace", func() {
				Expect(createErr).NotTo(HaveOccurred())

				cfOrg := new(korifiv1alpha1.CFOrg)
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: rootNamespace, Name: org.GUID}, cfOrg)).To(Succeed())

				Expect(cfOrg.Spec.DisplayName).To(Equal(orgGUID))
				Expect(cfOrg.Labels).To(Equal(map[string]string{"test-label-key": "test-label-val"}))
				Expect(cfOrg.Annotations).To(Equal(map[string]string{"test-annotation-key": "test-annotation-val"}))
			})

			When("the org isn't ready in the timeout", func() {
				BeforeEach(func() {
					doOrgControllerSimulation = false
				})

				It("returns an error", func() {
					Expect(createErr).To(MatchError(ContainSubstring("cf org did not get Condition `Ready`: 'True'")))
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
		})

		It("returns the 3 orgs", func() {
			orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{})
			Expect(err).NotTo(HaveOccurred())

			Expect(orgs).To(ContainElements(
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
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "because",
					Message: "because",
				})
				Expect(k8sClient.Status().Update(ctx, cfOrg1)).To(Succeed())

				meta.SetStatusCondition(&(cfOrg2.Status.Conditions), metav1.Condition{
					Type:    "Ready",
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
			Expect(k8s.PatchSpec(ctx, k8sClient, cfOrg, func() {
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

		When("the org isn't found", func() {
			It("errors", func() {
				_, err := orgRepo.GetOrg(ctx, authInfo, "non-existent-org")
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
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
						"key-one": pointerTo("value-one"),
						"key-two": pointerTo("value-two"),
					}
					annotationsPatch = map[string]*string{
						"key-one": pointerTo("value-one"),
						"key-two": pointerTo("value-two"),
					}
					Expect(k8s.PatchSpec(ctx, k8sClient, cfOrg, func() {
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
						"key-one":        pointerTo("value-one-updated"),
						"key-two":        pointerTo("value-two"),
						"before-key-two": nil,
					}
					annotationsPatch = map[string]*string{
						"key-one":        pointerTo("value-one-updated"),
						"key-two":        pointerTo("value-two"),
						"before-key-two": nil,
					}
					Expect(k8s.PatchSpec(ctx, k8sClient, cfOrg, func() {
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
						"-bad-annotation": pointerTo("stuff"),
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
						"-bad-label": pointerTo("stuff"),
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
