package repositories_test

import (
	"context"
	"fmt"
	"sort"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	workloads "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	hncv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("OrgRepository", func() {
	var (
		ctx     context.Context
		orgRepo *repositories.OrgRepo
	)

	const (
		orgLevel   = "1"
		spaceLevel = "2"
	)

	BeforeEach(func() {
		ctx = context.Background()
		orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, userClientFactory, nsPerms, time.Millisecond*2000)
	})

	Describe("Create", func() {
		var createErr error

		Describe("Org", func() {
			var (
				orgName                   string
				org                       repositories.OrgRecord
				createRoleBindings        bool
				done                      chan bool
				doOrgControllerSimulation bool
			)

			waitForCFOrg := func(anchorNamespace string, orgName string, done chan bool) (*workloads.CFOrg, error) {
				for {
					select {
					case <-done:
						return nil, fmt.Errorf("waitForCFOrg was 'signalled' to stop polling")
					default:
					}

					var orgList workloads.CFOrgList
					err := k8sClient.List(ctx, &orgList, client.InNamespace(anchorNamespace))
					if err != nil {
						return nil, fmt.Errorf("waitForCFOrg failed")
					}

					var matches []workloads.CFOrg
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

			simulateOrgController := func(anchorNamespace string, orgName string, createRoleBindings bool, depthLevel string, done chan bool) {
				defer GinkgoRecover()

				org, err := waitForCFOrg(anchorNamespace, orgName, done)
				if err != nil {
					return
				}

				namespaceLabels := map[string]string{
					rootNamespace + hncv1alpha2.LabelTreeDepthSuffix: depthLevel,
				}
				createNamespace(ctx, anchorNamespace, org.Name, namespaceLabels)

				if createRoleBindings {
					createRoleBinding(ctx, userName, adminRole.Name, org.Name)
				}

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
				orgName = prefixedGUID("org-name")
				createRoleBindings = true
			})

			JustBeforeEach(func() {
				if doOrgControllerSimulation {
					go simulateOrgController(rootNamespace, orgName, createRoleBindings, orgLevel, done)
				}
				org, createErr = orgRepo.CreateOrg(ctx, authInfo, repositories.CreateOrgMessage{
					Name: orgName,
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

				It("creates a CF Org resource in the root namespace", func() {
					Expect(createErr).NotTo(HaveOccurred())

					orgCR := new(workloads.CFOrg)
					Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: rootNamespace, Name: org.GUID}, orgCR)).To(Succeed())

					Expect(org.Name).To(Equal(orgName))
					Expect(org.GUID).To(HavePrefix("cf-org-"))
					Expect(org.CreatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
					Expect(org.UpdatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
				})

				When("CFOrgController fails to propagate the role bindings in the timeout", func() {
					BeforeEach(func() {
						createRoleBindings = false
					})

					It("returns an error", func() {
						Expect(createErr).To(MatchError(ContainSubstring("failed establishing permissions in new namespace")))
					})
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
						orgName = "this-string-has-illegal-characters-ц"
					})

					It("returns an error", func() {
						Expect(createErr).To(HaveOccurred())
					})
				})
			})
		})

		Describe("Space", func() {
			var (
				orgGUID                   string
				spaceName                 string
				space                     repositories.SpaceRecord
				imageRegistryCredentials  string
				doHNCControllerSimulation bool
			)

			waitForAnchor := func(anchorNamespace string, anchorLabel map[string]string, done chan bool) (*hncv1alpha2.SubnamespaceAnchor, error) {
				for {
					select {
					case <-done:
						return nil, fmt.Errorf("waitForAnchor was 'signalled' to stop polling")
					default:
					}

					var subspaceAnchorList hncv1alpha2.SubnamespaceAnchorList
					err := k8sClient.List(ctx, &subspaceAnchorList, client.InNamespace(anchorNamespace), client.MatchingLabels(anchorLabel))
					if err != nil {
						return nil, fmt.Errorf("waitForAnchor failed")
					}
					if len(subspaceAnchorList.Items) > 1 {
						return nil, fmt.Errorf("waitForAnchor found multiple anchors")
					}
					if len(subspaceAnchorList.Items) == 1 {
						return &subspaceAnchorList.Items[0], nil
					}

					time.Sleep(time.Millisecond * 100)
				}
			}

			// simulateHNC waits for the subnamespaceanchor to appear, and then
			// creates the namespace, creates a cf-admin rolebinding for the user
			// in that namespace, creates the hierarchyconfiguration in the
			// namespace and sets the status on the subnamespaceanchor to Ok. It
			// will stop waiting for the subnamespaceanchor to appear if anything
			// is written to the done channel
			simulateHNC := func(anchorNamespace string, anchorLabel map[string]string, createRoleBindings bool, depthLevel string, done chan bool) {
				defer GinkgoRecover()

				anchor, err := waitForAnchor(anchorNamespace, anchorLabel, done)
				if err != nil {
					return
				}

				namespaceLabels := map[string]string{
					rootNamespace + hncv1alpha2.LabelTreeDepthSuffix: depthLevel,
				}
				createNamespace(ctx, anchorNamespace, anchor.Name, namespaceLabels)

				Expect(k8sClient.Create(ctx, &hncv1alpha2.HierarchyConfiguration{
					ObjectMeta: metav1.ObjectMeta{Namespace: anchor.Name, Name: "hierarchy"},
				})).To(Succeed())

				newAnchor := anchor.DeepCopy()

				if createRoleBindings {
					createRoleBinding(ctx, userName, adminRole.Name, anchor.Name)
				}
				newAnchor.Status.State = hncv1alpha2.Ok
				Expect(k8sClient.Patch(ctx, newAnchor, client.MergeFrom(anchor))).To(Succeed())
			}

			BeforeEach(func() {
				spaceName = prefixedGUID("space-name")
				imageRegistryCredentials = "imageRegistryCredentials"
				org := createOrgWithCleanup(ctx, prefixedGUID("org"))
				orgGUID = org.Name
				doHNCControllerSimulation = true
			})

			JustBeforeEach(func() {
				if doHNCControllerSimulation {
					done := make(chan bool, 1)
					defer func(done chan bool) { done <- true }(done)

					go simulateHNC(orgGUID, map[string]string{repositories.SpaceNameLabel: spaceName}, true, spaceLevel, done)
				}
				space, createErr = orgRepo.CreateSpace(ctx, authInfo, repositories.CreateSpaceMessage{
					Name:                     spaceName,
					OrganizationGUID:         orgGUID,
					ImageRegistryCredentials: "imageRegistryCredentials",
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

				It("creates a Space", func() {
					Expect(createErr).NotTo(HaveOccurred())

					By("Creating a SubnamespaceAnchor in the Org namespace", func() {
						var namesRequirement *labels.Requirement
						var err error
						namesRequirement, err = labels.NewRequirement(repositories.SpaceNameLabel, selection.Equals, []string{spaceName})
						Expect(err).NotTo(HaveOccurred())
						anchorList := hncv1alpha2.SubnamespaceAnchorList{}
						err = k8sClient.List(ctx, &anchorList, client.InNamespace(orgGUID), client.MatchingLabelsSelector{
							Selector: labels.NewSelector().Add(*namesRequirement),
						})
						Expect(err).NotTo(HaveOccurred())
						Expect(anchorList.Items).To(HaveLen(1))

						Expect(space.Name).To(Equal(spaceName))
						Expect(space.GUID).To(HavePrefix("cf-space-"))
						Expect(space.CreatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
						Expect(space.UpdatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
					})

					By("Creating ServiceAccounts in the Space namespace", func() {
						serviceAccountList := corev1.ServiceAccountList{}
						Expect(k8sClient.List(ctx, &serviceAccountList, client.InNamespace(space.GUID))).To(Succeed())
						Expect(serviceAccountList.Items).To(HaveLen(2))

						sort.Slice(serviceAccountList.Items, func(i, j int) bool {
							return serviceAccountList.Items[i].Name < serviceAccountList.Items[j].Name
						})
						serviceAccount := serviceAccountList.Items[0]
						Expect(serviceAccount.Name).To(Equal("eirini"))
						serviceAccount = serviceAccountList.Items[1]
						Expect(serviceAccount.Name).To(Equal("kpack-service-account"))
						Expect(serviceAccount.ImagePullSecrets).To(ConsistOf(corev1.LocalObjectReference{Name: imageRegistryCredentials}))
						Expect(serviceAccount.Secrets).To(ConsistOf(corev1.ObjectReference{Name: imageRegistryCredentials}))
					})
				})

				When("the space isn't ready in the timeout", func() {
					BeforeEach(func() {
						doHNCControllerSimulation = false
					})

					It("returns an error", func() {
						Expect(createErr).To(MatchError(ContainSubstring("did not get state 'ok'")))
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

				When("the org does not exist", func() {
					BeforeEach(func() {
						orgGUID = "does-not-exist"
					})

					It("returns an error", func() {
						Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
					})
				})

				When("the user doesn't have permission to get the org", func() {
					BeforeEach(func() {
						otherOrg := createOrgWithCleanup(ctx, "org")
						orgGUID = otherOrg.Name
					})

					It("returns an error", func() {
						Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
					})
				})
			})
		})
	})

	Describe("List", func() {
		var cfOrg1, cfOrg2, cfOrg3 *workloads.CFOrg

		BeforeEach(func() {
			ctx = context.Background()

			cfOrg1 = createOrgWithCleanup(ctx, prefixedGUID("org1"))
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfOrg1.Name)
			cfOrg2 = createOrgWithCleanup(ctx, prefixedGUID("org2"))
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfOrg2.Name)
			cfOrg3 = createOrgWithCleanup(ctx, prefixedGUID("org3"))
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfOrg3.Name)
		})

		Describe("Orgs", func() {
			It("returns the 3 orgs", func() {
				orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{})
				Expect(err).NotTo(HaveOccurred())

				Expect(orgs).To(ContainElements(
					repositories.OrgRecord{
						Name:      cfOrg1.Spec.DisplayName,
						CreatedAt: cfOrg1.CreationTimestamp.Time,
						UpdatedAt: cfOrg1.CreationTimestamp.Time,
						GUID:      cfOrg1.Name,
					},
					repositories.OrgRecord{
						Name:      cfOrg2.Spec.DisplayName,
						CreatedAt: cfOrg2.CreationTimestamp.Time,
						UpdatedAt: cfOrg2.CreationTimestamp.Time,
						GUID:      cfOrg2.Name,
					},
					repositories.OrgRecord{
						Name:      cfOrg3.Spec.DisplayName,
						CreatedAt: cfOrg3.CreationTimestamp.Time,
						UpdatedAt: cfOrg3.CreationTimestamp.Time,
						GUID:      cfOrg3.Name,
					},
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
						repositories.OrgRecord{
							Name:      cfOrg1.Spec.DisplayName,
							CreatedAt: cfOrg1.CreationTimestamp.Time,
							UpdatedAt: cfOrg1.CreationTimestamp.Time,
							GUID:      cfOrg1.Name,
						},
						repositories.OrgRecord{
							Name:      cfOrg3.Spec.DisplayName,
							CreatedAt: cfOrg3.CreationTimestamp.Time,
							UpdatedAt: cfOrg3.CreationTimestamp.Time,
							GUID:      cfOrg3.Name,
						},
					))
				})
			})

			When("we filter for guids org1 and org3", func() {
				It("returns just those", func() {
					orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{GUIDs: []string{cfOrg1.Name, cfOrg2.Name}})
					Expect(err).NotTo(HaveOccurred())

					Expect(orgs).To(ConsistOf(
						repositories.OrgRecord{
							Name:      cfOrg1.Spec.DisplayName,
							CreatedAt: cfOrg1.CreationTimestamp.Time,
							UpdatedAt: cfOrg1.CreationTimestamp.Time,
							GUID:      cfOrg1.Name,
						},
						repositories.OrgRecord{
							Name:      cfOrg2.Spec.DisplayName,
							CreatedAt: cfOrg2.CreationTimestamp.Time,
							UpdatedAt: cfOrg2.CreationTimestamp.Time,
							GUID:      cfOrg2.Name,
						},
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

		Describe("Spaces", func() {
			var space11Anchor, space12Anchor, space21Anchor, space22Anchor, space31Anchor, space32Anchor *hncv1alpha2.SubnamespaceAnchor

			BeforeEach(func() {
				space11Anchor = createSpaceAnchorAndNamespace(ctx, cfOrg1.Name, "space1")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space11Anchor.Name)
				space12Anchor = createSpaceAnchorAndNamespace(ctx, cfOrg1.Name, "space2")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space12Anchor.Name)

				space21Anchor = createSpaceAnchorAndNamespace(ctx, cfOrg2.Name, "space1")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space21Anchor.Name)
				space22Anchor = createSpaceAnchorAndNamespace(ctx, cfOrg2.Name, "space3")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space22Anchor.Name)

				space31Anchor = createSpaceAnchorAndNamespace(ctx, cfOrg3.Name, "space1")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space31Anchor.Name)
				space32Anchor = createSpaceAnchorAndNamespace(ctx, cfOrg3.Name, "space4")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space32Anchor.Name)
			})

			It("returns the 6 spaces", func() {
				spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{})
				Expect(err).NotTo(HaveOccurred())

				Expect(spaces).To(ConsistOf(
					repositories.SpaceRecord{
						Name:             "space1",
						CreatedAt:        space11Anchor.CreationTimestamp.Time,
						UpdatedAt:        space11Anchor.CreationTimestamp.Time,
						GUID:             space11Anchor.Name,
						OrganizationGUID: cfOrg1.Name,
					},
					repositories.SpaceRecord{
						Name:             "space2",
						CreatedAt:        space12Anchor.CreationTimestamp.Time,
						UpdatedAt:        space12Anchor.CreationTimestamp.Time,
						GUID:             space12Anchor.Name,
						OrganizationGUID: cfOrg1.Name,
					},
					repositories.SpaceRecord{
						Name:             "space1",
						CreatedAt:        space21Anchor.CreationTimestamp.Time,
						UpdatedAt:        space21Anchor.CreationTimestamp.Time,
						GUID:             space21Anchor.Name,
						OrganizationGUID: cfOrg2.Name,
					},
					repositories.SpaceRecord{
						Name:             "space3",
						CreatedAt:        space22Anchor.CreationTimestamp.Time,
						UpdatedAt:        space22Anchor.CreationTimestamp.Time,
						GUID:             space22Anchor.Name,
						OrganizationGUID: cfOrg2.Name,
					},
					repositories.SpaceRecord{
						Name:             "space1",
						CreatedAt:        space31Anchor.CreationTimestamp.Time,
						UpdatedAt:        space31Anchor.CreationTimestamp.Time,
						GUID:             space31Anchor.Name,
						OrganizationGUID: cfOrg3.Name,
					},
					repositories.SpaceRecord{
						Name:             "space4",
						CreatedAt:        space32Anchor.CreationTimestamp.Time,
						UpdatedAt:        space32Anchor.CreationTimestamp.Time,
						GUID:             space32Anchor.Name,
						OrganizationGUID: cfOrg3.Name,
					},
				))
			})

			When("the space anchor is not ready", func() {
				BeforeEach(func() {
					space11AnchorCopy := space11Anchor.DeepCopy()
					space11AnchorCopy.Status.State = hncv1alpha2.Missing
					Expect(k8sClient.Patch(ctx, space11AnchorCopy, client.MergeFrom(space11Anchor))).To(Succeed())
				})

				It("does not list it", func() {
					spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{})
					Expect(err).NotTo(HaveOccurred())

					Expect(spaces).NotTo(ContainElement(
						repositories.SpaceRecord{
							Name:             "space1",
							CreatedAt:        space11Anchor.CreationTimestamp.Time,
							UpdatedAt:        space11Anchor.CreationTimestamp.Time,
							GUID:             space11Anchor.Name,
							OrganizationGUID: cfOrg1.Name,
						},
					))
				})
			})

			When("filtering by org guids", func() {
				It("only retruns the spaces belonging to the specified org guids", func() {
					spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
						OrganizationGUIDs: []string{cfOrg1.Name, cfOrg3.Name, "does-not-exist"},
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(spaces).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"Name":             Equal("space1"),
							"OrganizationGUID": Equal(cfOrg1.Name),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Name":             Equal("space1"),
							"OrganizationGUID": Equal(cfOrg3.Name),
						}),
						MatchFields(IgnoreExtras, Fields{"Name": Equal("space2")}),
						MatchFields(IgnoreExtras, Fields{"Name": Equal("space4")}),
					))
				})
			})

			When("filtering by space names", func() {
				It("only returns the spaces matching the specified names", func() {
					spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
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
							"Name":             Equal("space1"),
							"OrganizationGUID": Equal(cfOrg3.Name),
						}),
						MatchFields(IgnoreExtras, Fields{"Name": Equal("space3")}),
					))
				})
			})

			When("filtering by space guids", func() {
				It("only returns the spaces matching the specified guids", func() {
					spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
						GUIDs: []string{space11Anchor.Name, space21Anchor.Name, "does-not-exist"},
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
				It("only retruns the spaces matching the specified names", func() {
					spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
						OrganizationGUIDs: []string{cfOrg1.Name, cfOrg2.Name},
						Names:             []string{"space1", "space2", "space4"},
						GUIDs:             []string{space11Anchor.Name, space21Anchor.Name},
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
				It("only retruns the spaces matching the specified names", func() {
					spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
						Names: []string{"does-not-exist", "still-does-not-exist"},
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(spaces).To(BeEmpty())
				})
			})

			When("filtering by org uids that don't exist", func() {
				It("only retruns the spaces matching the specified names", func() {
					spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
						OrganizationGUIDs: []string{"does-not-exist", "still-does-not-exist"},
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(spaces).To(BeEmpty())
				})
			})
		})
	})

	Describe("Get", func() {
		Describe("Org", func() {
			var cfOrg *workloads.CFOrg

			BeforeEach(func() {
				cfOrg = createOrgWithCleanup(ctx, prefixedGUID("the-org"))
			})

			When("the user has a role binding in the org", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg.Name)
				})

				It("gets the org", func() {
					orgRecord, err := orgRepo.GetOrg(ctx, authInfo, cfOrg.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(orgRecord.Name).To(Equal(cfOrg.Spec.DisplayName))
				})
			})

			When("the org isn't found", func() {
				It("errors", func() {
					_, err := orgRepo.GetOrg(ctx, authInfo, "non-existent-org")
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})

		Describe("Space", func() {
			var (
				spaceAnchor *hncv1alpha2.SubnamespaceAnchor
				cfOrg       *workloads.CFOrg
			)

			BeforeEach(func() {
				cfOrg = createOrgWithCleanup(ctx, "the-org")
				spaceAnchor = createSpaceAnchorAndNamespace(ctx, cfOrg.Name, "the-space")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, spaceAnchor.Name)
			})

			When("on the happy path", func() {
				It("gets the subns resource", func() {
					spaceRecord, err := orgRepo.GetSpace(ctx, authInfo, spaceAnchor.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(spaceRecord.Name).To(Equal("the-space"))
					Expect(spaceRecord.OrganizationGUID).To(Equal(cfOrg.Name))
				})
			})

			When("the space doesn't exist", func() {
				It("errors", func() {
					_, err := orgRepo.GetSpace(ctx, authInfo, "non-existent-space")
					Expect(err).To(MatchError(ContainSubstring("not found")))
				})
			})
		})
	})

	Describe("Delete", func() {
		var (
			ctx context.Context

			cfOrg *workloads.CFOrg
		)

		BeforeEach(func() {
			ctx = context.Background()
			cfOrg = createOrgWithCleanup(ctx, prefixedGUID("org"))
		})

		Describe("Org", func() {
			When("the user has permission to delete orgs", func() {
				BeforeEach(func() {
					beforeCtx := context.Background()
					createRoleBinding(beforeCtx, userName, adminRole.Name, cfOrg.Namespace)
					// As HNC Controllers don't exist in env-test environments, we manually copy role bindings to child ns.
					createRoleBinding(beforeCtx, userName, adminRole.Name, cfOrg.Name)
				})

				When("on the happy path", func() {
					It("deletes the CF Org resource", func() {
						err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
							GUID: cfOrg.Name,
						})
						Expect(err).NotTo(HaveOccurred())

						foundCFOrg := &workloads.CFOrg{}
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

		Describe("Space", func() {
			var spaceAnchor *hncv1alpha2.SubnamespaceAnchor

			BeforeEach(func() {
				spaceAnchor = createSpaceAnchorAndNamespace(ctx, cfOrg.Name, "the-space")
			})

			When("the user has permission to delete spaces", func() {
				BeforeEach(func() {
					beforeCtx := context.Background()
					createRoleBinding(beforeCtx, userName, adminRole.Name, spaceAnchor.Namespace)
				})

				It("deletes the subns resource", func() {
					err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
						GUID:             spaceAnchor.Name,
						OrganizationGUID: cfOrg.Name,
					})
					Expect(err).NotTo(HaveOccurred())

					anchor := &hncv1alpha2.SubnamespaceAnchor{}
					err = k8sClient.Get(ctx, client.ObjectKey{Namespace: cfOrg.Name, Name: spaceAnchor.Name}, anchor)
					Expect(err).To(MatchError(ContainSubstring("not found")))
				})

				When("the space doesn't exist", func() {
					It("errors", func() {
						err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
							GUID:             "non-existent-space",
							OrganizationGUID: cfOrg.Name,
						})
						Expect(err).To(MatchError(ContainSubstring("not found")))
					})
				})
			})

			When("the user does not have permission to delete spaces and", func() {
				It("errors with forbidden", func() {
					err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
						GUID:             spaceAnchor.Name,
						OrganizationGUID: cfOrg.Name,
					})
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})

				When("the space doesn't exist", func() {
					It("errors with forbidden", func() {
						err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
							GUID:             "non-existent-space",
							OrganizationGUID: cfOrg.Name,
						})
						Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
					})
				})
			})
		})
	})
})
