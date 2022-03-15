package repositories_test

import (
	"context"
	"fmt"
	"sort"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	hncv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("OrgRepository", func() {
	var (
		ctx     context.Context
		orgRepo *repositories.OrgRepo
	)

	BeforeEach(func() {
		ctx = context.Background()
		orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, userClientFactory, nsPerms, time.Millisecond*2000, true)
	})

	Describe("Create", func() {
		var (
			doHNCSimulation bool
			createErr       error
		)

		waitForAnchor := func(anchorNamespace, anchorName string, anchor *hncv1alpha2.SubnamespaceAnchor, done chan bool) error {
			for {
				select {
				case <-done:
					return fmt.Errorf("waitForAnchor was 'signalled' to stop polling")
				default:
				}

				if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: anchorNamespace, Name: anchorName}, anchor); err == nil {
					return nil
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
		simulateHNC := func(anchorNamespace, anchorName string, createRoleBindings bool, done chan bool) {
			defer GinkgoRecover()

			var anchor hncv1alpha2.SubnamespaceAnchor
			if err := waitForAnchor(anchorNamespace, anchorName, &anchor, done); err != nil {
				return
			}

			createNamespace(ctx, anchorNamespace, anchorName)

			Expect(k8sClient.Create(ctx, &hncv1alpha2.HierarchyConfiguration{
				ObjectMeta: metav1.ObjectMeta{Namespace: anchorName, Name: "hierarchy"},
			})).To(Succeed())

			newAnchor := anchor.DeepCopy()

			if createRoleBindings {
				createRoleBinding(ctx, userName, adminRole.Name, anchor.Name)
			}
			newAnchor.Status.State = hncv1alpha2.Ok
			Expect(k8sClient.Patch(ctx, newAnchor, client.MergeFrom(&anchor))).To(Succeed())
		}

		Describe("Org", func() {
			var (
				orgName            string
				orgGUID            string
				org                repositories.OrgRecord
				createRoleBindings bool
				done               chan bool
			)

			BeforeEach(func() {
				doHNCSimulation = true
				done = make(chan bool, 1)
				orgGUID = prefixedGUID("org-guid")
				orgName = prefixedGUID("org-name")
				createRoleBindings = true
			})

			JustBeforeEach(func() {
				if doHNCSimulation {
					go simulateHNC(rootNamespace, orgGUID, createRoleBindings, done)
				}

				org, createErr = orgRepo.CreateOrg(ctx, authInfo, repositories.CreateOrgMessage{
					GUID: orgGUID,
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

				It("creates a subnamespace anchor in the root namespace", func() {
					Expect(createErr).NotTo(HaveOccurred())

					namesRequirement, err := labels.NewRequirement(repositories.OrgNameLabel, selection.Equals, []string{orgName})
					Expect(err).NotTo(HaveOccurred())
					anchorList := hncv1alpha2.SubnamespaceAnchorList{}
					err = k8sClient.List(ctx, &anchorList, client.InNamespace(rootNamespace), client.MatchingLabelsSelector{
						Selector: labels.NewSelector().Add(*namesRequirement),
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(anchorList.Items).To(HaveLen(1))

					Expect(org.Name).To(Equal(orgName))
					Expect(org.GUID).To(Equal(orgGUID))
					Expect(org.CreatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
					Expect(org.UpdatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
				})

				It("sets the AllowCascadingDeletion property on the HNC hierarchyconfiguration in the namespace", func() {
					Expect(createErr).NotTo(HaveOccurred())

					var hc hncv1alpha2.HierarchyConfiguration
					Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: org.GUID, Name: "hierarchy"}, &hc)).To(Succeed())
					Expect(hc.Spec.AllowCascadingDeletion).To(BeTrue())
				})

				When("hnc fails to propagate the role bindings in the timeout", func() {
					BeforeEach(func() {
						createRoleBindings = false
					})

					It("returns an error", func() {
						Expect(createErr).To(MatchError(ContainSubstring("failed establishing permissions in new namespace")))
					})
				})

				When("the org isn't ready in the timeout", func() {
					BeforeEach(func() {
						doHNCSimulation = false
					})

					It("returns an error", func() {
						Expect(createErr).To(MatchError(ContainSubstring("did not get state 'ok'")))
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
				orgGUID                  string
				spaceGUID                string
				spaceName                string
				space                    repositories.SpaceRecord
				imageRegistryCredentials string
			)

			BeforeEach(func() {
				spaceGUID = prefixedGUID("space-guid")
				spaceName = prefixedGUID("space-name")
				imageRegistryCredentials = "imageRegistryCredentials"
				org := createOrgAnchorAndNamespace(ctx, rootNamespace, "org")
				orgGUID = org.Name
				doHNCSimulation = true
			})

			JustBeforeEach(func() {
				if doHNCSimulation {
					done := make(chan bool, 1)
					defer func(done chan bool) { done <- true }(done)

					go simulateHNC(orgGUID, spaceGUID, true, done)
				}

				space, createErr = orgRepo.CreateSpace(ctx, authInfo, repositories.CreateSpaceMessage{
					GUID:                     spaceGUID,
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
						Expect(space.GUID).To(Equal(spaceGUID))
						Expect(space.CreatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
						Expect(space.UpdatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
					})

					By("Creating ServiceAccounts in the Space namespace", func() {
						serviceAccountList := corev1.ServiceAccountList{}
						var err error
						Eventually(func() []corev1.ServiceAccount {
							err = k8sClient.List(ctx, &serviceAccountList, client.InNamespace(spaceGUID))
							if err != nil {
								return []corev1.ServiceAccount{}
							}
							return serviceAccountList.Items
						}, timeCheckThreshold*time.Second, 250*time.Millisecond).Should(HaveLen(2), "could not find the service accounts created by the repo")
						Expect(err).NotTo(HaveOccurred())

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
						doHNCSimulation = false
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
						otherOrg := createOrgAnchorAndNamespace(ctx, rootNamespace, "org")
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
		var org1Anchor, org2Anchor, org3Anchor, org4Anchor *hncv1alpha2.SubnamespaceAnchor

		BeforeEach(func() {
			ctx = context.Background()

			org1Anchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "org1")
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, org1Anchor.Name)
			org2Anchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "org2")
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, org2Anchor.Name)
			org3Anchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "org3")
			createRoleBinding(ctx, userName, spaceDeveloperRole.Name, org3Anchor.Name)
			org4Anchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "org4")
		})

		Describe("Orgs", func() {
			It("returns the 3 orgs", func() {
				orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{})
				Expect(err).NotTo(HaveOccurred())

				Expect(orgs).To(ConsistOf(
					repositories.OrgRecord{
						Name:      "org1",
						CreatedAt: org1Anchor.CreationTimestamp.Time,
						UpdatedAt: org1Anchor.CreationTimestamp.Time,
						GUID:      org1Anchor.Name,
					},
					repositories.OrgRecord{
						Name:      "org2",
						CreatedAt: org2Anchor.CreationTimestamp.Time,
						UpdatedAt: org2Anchor.CreationTimestamp.Time,
						GUID:      org2Anchor.Name,
					},
					repositories.OrgRecord{
						Name:      "org3",
						CreatedAt: org3Anchor.CreationTimestamp.Time,
						UpdatedAt: org3Anchor.CreationTimestamp.Time,
						GUID:      org3Anchor.Name,
					},
				))
			})

			When("auth is disabled", func() {
				BeforeEach(func() {
					orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, userClientFactory, nsPerms, time.Millisecond*2000, false)
				})

				It("returns all orgs", func() {
					orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{})
					Expect(err).NotTo(HaveOccurred())

					Expect(orgs).To(ConsistOf(
						repositories.OrgRecord{
							Name:      "org1",
							CreatedAt: org1Anchor.CreationTimestamp.Time,
							UpdatedAt: org1Anchor.CreationTimestamp.Time,
							GUID:      org1Anchor.Name,
						},
						repositories.OrgRecord{
							Name:      "org2",
							CreatedAt: org2Anchor.CreationTimestamp.Time,
							UpdatedAt: org2Anchor.CreationTimestamp.Time,
							GUID:      org2Anchor.Name,
						},
						repositories.OrgRecord{
							Name:      "org3",
							CreatedAt: org3Anchor.CreationTimestamp.Time,
							UpdatedAt: org3Anchor.CreationTimestamp.Time,
							GUID:      org3Anchor.Name,
						},
						repositories.OrgRecord{
							Name:      "org4",
							CreatedAt: org4Anchor.CreationTimestamp.Time,
							UpdatedAt: org4Anchor.CreationTimestamp.Time,
							GUID:      org4Anchor.Name,
						},
					))
				})
			})

			When("the org anchor is not ready", func() {
				BeforeEach(func() {
					org1AnchorCopy := org1Anchor.DeepCopy()
					org1AnchorCopy.Status.State = hncv1alpha2.Missing
					Expect(k8sClient.Patch(ctx, org1AnchorCopy, client.MergeFrom(org1Anchor))).To(Succeed())
				})

				It("does not list it", func() {
					orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{})
					Expect(err).NotTo(HaveOccurred())

					Expect(orgs).NotTo(ContainElement(
						repositories.OrgRecord{
							Name:      "org1",
							CreatedAt: org1Anchor.CreationTimestamp.Time,
							UpdatedAt: org1Anchor.CreationTimestamp.Time,
							GUID:      org1Anchor.Name,
						},
					))
				})
			})

			When("we filter for names org1 and org3", func() {
				It("returns just those", func() {
					orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{Names: []string{"org1", "org3"}})
					Expect(err).NotTo(HaveOccurred())

					Expect(orgs).To(ConsistOf(
						repositories.OrgRecord{
							Name:      "org1",
							CreatedAt: org1Anchor.CreationTimestamp.Time,
							UpdatedAt: org1Anchor.CreationTimestamp.Time,
							GUID:      org1Anchor.Name,
						},
						repositories.OrgRecord{
							Name:      "org3",
							CreatedAt: org3Anchor.CreationTimestamp.Time,
							UpdatedAt: org3Anchor.CreationTimestamp.Time,
							GUID:      org3Anchor.Name,
						},
					))
				})
			})

			When("we filter for guids org1 and org3", func() {
				It("returns just those", func() {
					orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{GUIDs: []string{org1Anchor.Name, org3Anchor.Name}})
					Expect(err).NotTo(HaveOccurred())

					Expect(orgs).To(ConsistOf(
						repositories.OrgRecord{
							Name:      "org1",
							CreatedAt: org1Anchor.CreationTimestamp.Time,
							UpdatedAt: org1Anchor.CreationTimestamp.Time,
							GUID:      org1Anchor.Name,
						},
						repositories.OrgRecord{
							Name:      "org3",
							CreatedAt: org3Anchor.CreationTimestamp.Time,
							UpdatedAt: org3Anchor.CreationTimestamp.Time,
							GUID:      org3Anchor.Name,
						},
					))
				})
			})

			When("fetching authorized namespaces fails", func() {
				var listErr error

				BeforeEach(func() {
					_, listErr = orgRepo.ListOrgs(ctx, authorization.Info{}, repositories.ListOrgsMessage{Names: []string{"org1", "org3"}})
				})

				It("returns the error", func() {
					Expect(listErr).To(MatchError(ContainSubstring("failed to get identity")))
				})
			})
		})

		Describe("Spaces", func() {
			var space11Anchor, space12Anchor, space21Anchor, space22Anchor, space31Anchor, space32Anchor, space33Anchor *hncv1alpha2.SubnamespaceAnchor

			BeforeEach(func() {
				space11Anchor = createSpaceAnchorAndNamespace(ctx, org1Anchor.Name, "space1")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space11Anchor.Name)
				space12Anchor = createSpaceAnchorAndNamespace(ctx, org1Anchor.Name, "space2")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space12Anchor.Name)

				space21Anchor = createSpaceAnchorAndNamespace(ctx, org2Anchor.Name, "space1")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space21Anchor.Name)
				space22Anchor = createSpaceAnchorAndNamespace(ctx, org2Anchor.Name, "space3")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space22Anchor.Name)

				space31Anchor = createSpaceAnchorAndNamespace(ctx, org3Anchor.Name, "space1")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space31Anchor.Name)
				space32Anchor = createSpaceAnchorAndNamespace(ctx, org3Anchor.Name, "space4")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space32Anchor.Name)

				space33Anchor = createSpaceAnchorAndNamespace(ctx, org3Anchor.Name, "space5")
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
						OrganizationGUID: org1Anchor.Name,
					},
					repositories.SpaceRecord{
						Name:             "space2",
						CreatedAt:        space12Anchor.CreationTimestamp.Time,
						UpdatedAt:        space12Anchor.CreationTimestamp.Time,
						GUID:             space12Anchor.Name,
						OrganizationGUID: org1Anchor.Name,
					},
					repositories.SpaceRecord{
						Name:             "space1",
						CreatedAt:        space21Anchor.CreationTimestamp.Time,
						UpdatedAt:        space21Anchor.CreationTimestamp.Time,
						GUID:             space21Anchor.Name,
						OrganizationGUID: org2Anchor.Name,
					},
					repositories.SpaceRecord{
						Name:             "space3",
						CreatedAt:        space22Anchor.CreationTimestamp.Time,
						UpdatedAt:        space22Anchor.CreationTimestamp.Time,
						GUID:             space22Anchor.Name,
						OrganizationGUID: org2Anchor.Name,
					},
					repositories.SpaceRecord{
						Name:             "space1",
						CreatedAt:        space31Anchor.CreationTimestamp.Time,
						UpdatedAt:        space31Anchor.CreationTimestamp.Time,
						GUID:             space31Anchor.Name,
						OrganizationGUID: org3Anchor.Name,
					},
					repositories.SpaceRecord{
						Name:             "space4",
						CreatedAt:        space32Anchor.CreationTimestamp.Time,
						UpdatedAt:        space32Anchor.CreationTimestamp.Time,
						GUID:             space32Anchor.Name,
						OrganizationGUID: org3Anchor.Name,
					},
				))
			})

			When("auth is disabled", func() {
				BeforeEach(func() {
					orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, userClientFactory, nsPerms, time.Millisecond*2000, false)
				})

				It("includes spaces without role bindings", func() {
					spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{})
					Expect(err).NotTo(HaveOccurred())

					Expect(spaces).To(HaveLen(7))
					Expect(spaces).To(ContainElement(
						repositories.SpaceRecord{
							Name:             "space5",
							CreatedAt:        space33Anchor.CreationTimestamp.Time,
							UpdatedAt:        space33Anchor.CreationTimestamp.Time,
							GUID:             space33Anchor.Name,
							OrganizationGUID: org3Anchor.Name,
						},
					))
				})
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
							OrganizationGUID: org1Anchor.Name,
						},
					))
				})
			})

			When("filtering by org guids", func() {
				It("only retruns the spaces belonging to the specified org guids", func() {
					spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
						OrganizationGUIDs: []string{string(org1Anchor.Name), string(org3Anchor.Name), "does-not-exist"},
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(spaces).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"Name":             Equal("space1"),
							"OrganizationGUID": Equal(string(org1Anchor.Name)),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Name":             Equal("space1"),
							"OrganizationGUID": Equal(string(org3Anchor.Name)),
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
							"OrganizationGUID": Equal(string(org1Anchor.Name)),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Name":             Equal("space1"),
							"OrganizationGUID": Equal(string(org2Anchor.Name)),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Name":             Equal("space1"),
							"OrganizationGUID": Equal(string(org3Anchor.Name)),
						}),
						MatchFields(IgnoreExtras, Fields{"Name": Equal("space3")}),
					))
				})
			})

			When("filtering by space guids", func() {
				It("only returns the spaces matching the specified guids", func() {
					spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
						GUIDs: []string{string(space11Anchor.Name), string(space21Anchor.Name), "does-not-exist"},
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(spaces).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"Name":             Equal("space1"),
							"OrganizationGUID": Equal(string(org1Anchor.Name)),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Name":             Equal("space1"),
							"OrganizationGUID": Equal(string(org2Anchor.Name)),
						}),
					))
				})
			})

			When("filtering by org guids, space names and space guids", func() {
				It("only retruns the spaces matching the specified names", func() {
					spaces, err := orgRepo.ListSpaces(ctx, authInfo, repositories.ListSpacesMessage{
						OrganizationGUIDs: []string{string(org1Anchor.Name), string(org2Anchor.Name)},
						Names:             []string{"space1", "space2", "space4"},
						GUIDs:             []string{string(space11Anchor.Name), string(space21Anchor.Name)},
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(spaces).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{
							"Name":             Equal("space1"),
							"OrganizationGUID": Equal(string(org1Anchor.Name)),
						}),
						MatchFields(IgnoreExtras, Fields{
							"Name":             Equal("space1"),
							"OrganizationGUID": Equal(string(org2Anchor.Name)),
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
			var orgAnchor *hncv1alpha2.SubnamespaceAnchor

			BeforeEach(func() {
				orgAnchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "the-org")
			})

			When("the user has a role binding in the org", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, orgUserRole.Name, orgAnchor.Name)
				})

				It("gets the org", func() {
					orgRecord, err := orgRepo.GetOrg(ctx, authInfo, orgAnchor.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(orgRecord.Name).To(Equal("the-org"))
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
				orgAnchor   *hncv1alpha2.SubnamespaceAnchor
			)

			BeforeEach(func() {
				orgAnchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "the-org")
				spaceAnchor = createSpaceAnchorAndNamespace(ctx, orgAnchor.Name, "the-space")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, spaceAnchor.Name)
			})

			When("on the happy path", func() {
				It("gets the subns resource", func() {
					spaceRecord, err := orgRepo.GetSpace(ctx, authInfo, spaceAnchor.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(spaceRecord.Name).To(Equal("the-space"))
					Expect(spaceRecord.OrganizationGUID).To(Equal(orgAnchor.Name))
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

			orgAnchor *hncv1alpha2.SubnamespaceAnchor
		)

		BeforeEach(func() {
			ctx = context.Background()

			orgAnchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "the-org")
		})

		Describe("Org", func() {
			When("the user has permission to delete orgs", func() {
				BeforeEach(func() {
					beforeCtx := context.Background()
					createRoleBinding(beforeCtx, userName, adminRole.Name, orgAnchor.Namespace)
					// As HNC Controllers don't exist in env-test environments, we manually copy role bindings to child ns.
					createRoleBinding(beforeCtx, userName, adminRole.Name, orgAnchor.Name)
				})

				When("on the happy path", func() {
					It("deletes the subns resource", func() {
						err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
							GUID: orgAnchor.Name,
						})
						Expect(err).NotTo(HaveOccurred())

						Eventually(func() error {
							anchor := &hncv1alpha2.SubnamespaceAnchor{}
							return k8sClient.Get(ctx, client.ObjectKey{Namespace: rootNamespace, Name: orgAnchor.Name}, anchor)
						}).Should(MatchError(ContainSubstring("not found")))
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
						GUID: orgAnchor.Name,
					})
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})

				When("the org doesn't exist", func() {
					It("errors with not found", func() {
						err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
							GUID: "non-existent-space",
						})
						Expect(err).To(MatchError(ContainSubstring("not found")))
					})
				})
			})
		})

		Describe("Space", func() {
			var spaceAnchor *hncv1alpha2.SubnamespaceAnchor

			BeforeEach(func() {
				spaceAnchor = createSpaceAnchorAndNamespace(ctx, orgAnchor.Name, "the-space")
			})

			When("the user has permission to delete spaces", func() {
				BeforeEach(func() {
					beforeCtx := context.Background()
					createRoleBinding(beforeCtx, userName, adminRole.Name, spaceAnchor.Namespace)
				})

				It("deletes the subns resource", func() {
					err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
						GUID:             spaceAnchor.Name,
						OrganizationGUID: orgAnchor.Name,
					})
					Expect(err).NotTo(HaveOccurred())

					Eventually(func() error {
						anchor := &hncv1alpha2.SubnamespaceAnchor{}
						return k8sClient.Get(ctx, client.ObjectKey{Namespace: orgAnchor.Name, Name: spaceAnchor.Name}, anchor)
					}).Should(MatchError(ContainSubstring("not found")))
				})

				When("the space doesn't exist", func() {
					It("errors", func() {
						err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
							GUID:             "non-existent-space",
							OrganizationGUID: orgAnchor.Name,
						})
						Expect(err).To(MatchError(ContainSubstring("not found")))
					})
				})
			})

			When("the user does not have permission to delete spaces and", func() {
				It("errors with forbidden", func() {
					err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
						GUID:             spaceAnchor.Name,
						OrganizationGUID: orgAnchor.Name,
					})
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})

				When("the space doesn't exist", func() {
					It("errors with forbidden", func() {
						err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
							GUID:             "non-existent-space",
							OrganizationGUID: orgAnchor.Name,
						})
						Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
					})
				})
			})

			When("auth is disabled and", func() {
				BeforeEach(func() {
					mapper, err := apiutil.NewDynamicRESTMapper(k8sConfig)
					Expect(err).NotTo(HaveOccurred())
					userClientFactory = repositories.NewPrivilegedClientFactory(k8sConfig, mapper)
					orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, userClientFactory, nsPerms, time.Millisecond*2000, false)
				})

				When("on the happy path", func() {
					It("deletes the subns resource", func() {
						err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
							GUID:             spaceAnchor.Name,
							OrganizationGUID: orgAnchor.Name,
						})
						Expect(err).NotTo(HaveOccurred())

						Eventually(func() error {
							anchor := &hncv1alpha2.SubnamespaceAnchor{}
							return k8sClient.Get(ctx, client.ObjectKey{Namespace: orgAnchor.Name, Name: spaceAnchor.Name}, anchor)
						}).Should(MatchError(ContainSubstring("not found")))
					})
				})

				When("the space doesn't exist", func() {
					It("errors", func() {
						err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
							GUID:             "non-existent-space",
							OrganizationGUID: orgAnchor.Name,
						})
						Expect(err).To(MatchError(ContainSubstring("not found")))
					})
				})
			})
		})
	})
})
