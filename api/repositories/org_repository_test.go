package repositories_test

import (
	"context"
	"sort"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("OrgRepository", func() {
	var (
		ctx                       context.Context
		clientFactory             repositories.UserK8sClientFactory
		orgRepo                   *repositories.OrgRepo
		spaceDeveloperClusterRole *rbacv1.ClusterRole
		orgManagerClusterRole     *rbacv1.ClusterRole
		orgUserClusterRole        *rbacv1.ClusterRole
	)

	BeforeEach(func() {
		ctx = context.Background()

		Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())
		clientFactory = repositories.NewUnprivilegedClientFactory(k8sConfig)
		orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, clientFactory, nsPerms, time.Millisecond*500, true)
		spaceDeveloperClusterRole = createClusterRole(ctx, repositories.SpaceDeveloperClusterRoleRules)
		orgManagerClusterRole = createClusterRole(ctx, repositories.OrgManagerClusterRoleRules)
		orgUserClusterRole = createClusterRole(ctx, repositories.OrgUserClusterRoleRules)
	})

	Describe("Create", func() {
		updateStatus := func(anchorNamespace, anchorName string) {
			defer GinkgoRecover()

			anchor := &hnsv1alpha2.SubnamespaceAnchor{}
			for {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: anchorNamespace, Name: anchorName}, anchor)
				if err == nil {
					break
				}

				time.Sleep(time.Millisecond * 100)
				continue
			}

			newAnchor := anchor.DeepCopy()
			newAnchor.Status.State = hnsv1alpha2.Ok
			Expect(k8sClient.Patch(ctx, newAnchor, client.MergeFrom(anchor))).To(Succeed())
		}

		Describe("Org", func() {
			When("the user doesn't have the admin role", func() {
				It("fails when creating an org", func() {
					_, err := orgRepo.CreateOrg(ctx, authInfo, repositories.CreateOrgMessage{
						GUID: "some-guid",
						Name: "our-org",
					})
					Expect(err).To(BeAssignableToTypeOf(repositories.ForbiddenError{}))
				})
			})

			When("the user has the admin role", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, adminClusterRole.Name, rootNamespace)
				})

				It("creates a subnamespace anchor in the root namespace", func() {
					go updateStatus(rootNamespace, "some-guid")
					org, err := orgRepo.CreateOrg(ctx, authInfo, repositories.CreateOrgMessage{
						GUID: "some-guid",
						Name: "our-org",
					})
					Expect(err).NotTo(HaveOccurred())

					namesRequirement, err := labels.NewRequirement(repositories.OrgNameLabel, selection.Equals, []string{"our-org"})
					Expect(err).NotTo(HaveOccurred())
					anchorList := hnsv1alpha2.SubnamespaceAnchorList{}
					err = k8sClient.List(ctx, &anchorList, client.InNamespace(rootNamespace), client.MatchingLabelsSelector{
						Selector: labels.NewSelector().Add(*namesRequirement),
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(anchorList.Items).To(HaveLen(1))

					Expect(org.Name).To(Equal("our-org"))
					Expect(org.GUID).To(Equal("some-guid"))
					Expect(org.CreatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
					Expect(org.UpdatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
				})

				When("the org isn't ready in the timeout", func() {
					It("returns an error", func() {
						// we do not call updateStatus() to set state = ok
						_, err := orgRepo.CreateOrg(ctx, authInfo, repositories.CreateOrgMessage{
							GUID: "some-guid",
							Name: "our-org",
						})
						Expect(err).To(MatchError(ContainSubstring("did not get state 'ok'")))
					})
				})

				When("the client fails to create the org", func() {
					It("returns an error", func() {
						_, err := orgRepo.CreateOrg(ctx, authInfo, repositories.CreateOrgMessage{
							Name: "this-string-has-illegal-characters-ц",
						})
						Expect(err).To(HaveOccurred())
					})
				})
			})
		})

		Describe("Space", func() {
			var org *hnsv1alpha2.SubnamespaceAnchor
			var spaceGUID string
			imageRegistryCredentials := "image-registry-credentials"

			BeforeEach(func() {
				spaceGUID = generateGUID()
				org = createOrgAnchorAndNamespace(ctx, rootNamespace, "org")
				// In the absence of HNC reconciling the SubnamespaceAnchor into a namespace, we must manually create
				// for subsequent use by the Repository createSpace function.
				_ = createNamespace(ctx, "org", spaceGUID)
			})

			When("the user doesn't have the admin role", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, orgUserClusterRole.Name, org.Name)
				})

				It("fails when creating a space", func() {
					_, err := orgRepo.CreateSpace(ctx, authInfo, repositories.CreateSpaceMessage{
						GUID:                     spaceGUID,
						Name:                     "our-space",
						OrganizationGUID:         org.Name,
						ImageRegistryCredentials: imageRegistryCredentials,
					})
					Expect(err).To(BeAssignableToTypeOf(repositories.ForbiddenError{}))
				})
			})

			When("the user has the admin role", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, adminClusterRole.Name, org.Name)
				})

				It("creates a Space", func() {
					go updateStatus(org.Name, spaceGUID)

					space, err := orgRepo.CreateSpace(ctx, authInfo, repositories.CreateSpaceMessage{
						GUID:                     spaceGUID,
						Name:                     "our-space",
						OrganizationGUID:         org.Name,
						ImageRegistryCredentials: imageRegistryCredentials,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Creating a SubnamespaceAnchor in the Org namespace", func() {
						var namesRequirement *labels.Requirement
						namesRequirement, err = labels.NewRequirement(repositories.SpaceNameLabel, selection.Equals, []string{"our-space"})
						Expect(err).NotTo(HaveOccurred())
						anchorList := hnsv1alpha2.SubnamespaceAnchorList{}
						err = k8sClient.List(ctx, &anchorList, client.InNamespace(org.Name), client.MatchingLabelsSelector{
							Selector: labels.NewSelector().Add(*namesRequirement),
						})
						Expect(err).NotTo(HaveOccurred())
						Expect(anchorList.Items).To(HaveLen(1))

						Expect(space.Name).To(Equal("our-space"))
						Expect(space.GUID).To(Equal(spaceGUID))
						Expect(space.CreatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
						Expect(space.UpdatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
					})

					By("Creating ServiceAccounts in the Space namespace", func() {
						serviceAccountList := corev1.ServiceAccountList{}
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
					It("returns an error", func() {
						// we do not call updateStatus() to set state = ok
						_, err := orgRepo.CreateSpace(ctx, authInfo, repositories.CreateSpaceMessage{
							GUID:             "some-guid",
							Name:             "our-org",
							OrganizationGUID: org.Name,
						})
						Expect(err).To(MatchError(ContainSubstring("did not get state 'ok'")))
					})
				})

				When("the client fails to create the space", func() {
					It("returns an error", func() {
						_, err := orgRepo.CreateSpace(ctx, authInfo, repositories.CreateSpaceMessage{
							GUID:             "some-guid",
							Name:             "this-string-has-illegal-characters-ц",
							OrganizationGUID: org.Name,
						})
						Expect(err).To(HaveOccurred())
					})
				})

				When("the org does not exist", func() {
					It("returns an error", func() {
						_, err := orgRepo.CreateSpace(ctx, authInfo, repositories.CreateSpaceMessage{
							GUID:             "some-guid",
							Name:             "some-name",
							OrganizationGUID: "does-not-exist",
						})
						Expect(err).To(BeAssignableToTypeOf(repositories.PermissionDeniedOrNotFoundError{}))
					})
				})

				When("the user doesn't have permission to get the org", func() {
					var otherOrg *hnsv1alpha2.SubnamespaceAnchor

					BeforeEach(func() {
						otherOrg = createOrgAnchorAndNamespace(ctx, rootNamespace, "org")
					})

					It("returns an error", func() {
						_, err := orgRepo.CreateSpace(ctx, authInfo, repositories.CreateSpaceMessage{
							GUID:             "some-guid",
							Name:             "some-name",
							OrganizationGUID: otherOrg.Name,
						})
						Expect(err).To(BeAssignableToTypeOf(repositories.PermissionDeniedOrNotFoundError{}))
					})
				})
			})
		})
	})

	Describe("List", func() {
		var org1Anchor, org2Anchor, org3Anchor, org4Anchor *hnsv1alpha2.SubnamespaceAnchor

		BeforeEach(func() {
			ctx = context.Background()

			org1Anchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "org1")
			createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, org1Anchor.Name)
			org2Anchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "org2")
			createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, org2Anchor.Name)
			org3Anchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "org3")
			createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, org3Anchor.Name)
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
					orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, clientFactory, nsPerms, time.Millisecond*500, false)
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
					org1AnchorCopy.Status.State = hnsv1alpha2.Missing
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
			var space11Anchor, space12Anchor, space21Anchor, space22Anchor, space31Anchor, space32Anchor, space33Anchor *hnsv1alpha2.SubnamespaceAnchor

			BeforeEach(func() {
				space11Anchor = createSpaceAnchorAndNamespace(ctx, org1Anchor.Name, "space1")
				createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, space11Anchor.Name)
				space12Anchor = createSpaceAnchorAndNamespace(ctx, org1Anchor.Name, "space2")
				createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, space12Anchor.Name)

				space21Anchor = createSpaceAnchorAndNamespace(ctx, org2Anchor.Name, "space1")
				createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, space21Anchor.Name)
				space22Anchor = createSpaceAnchorAndNamespace(ctx, org2Anchor.Name, "space3")
				createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, space22Anchor.Name)

				space31Anchor = createSpaceAnchorAndNamespace(ctx, org3Anchor.Name, "space1")
				createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, space31Anchor.Name)
				space32Anchor = createSpaceAnchorAndNamespace(ctx, org3Anchor.Name, "space4")
				createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, space32Anchor.Name)

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
					orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, clientFactory, nsPerms, time.Millisecond*500, false)
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
					space11AnchorCopy.Status.State = hnsv1alpha2.Missing
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
			var orgAnchor *hnsv1alpha2.SubnamespaceAnchor

			BeforeEach(func() {
				orgAnchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "the-org")
			})

			When("the user has a role binding in the org", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, orgManagerClusterRole.Name, orgAnchor.Name)
				})

				It("gets the org", func() {
					orgRecord, err := orgRepo.GetOrg(ctx, authInfo, orgAnchor.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(orgRecord.Name).To(Equal("the-org"))
				})
			})

			When("the org doesn't exist", func() {
				It("errors", func() {
					_, err := orgRepo.GetOrg(ctx, authInfo, "non-existent-org")
					Expect(err).To(BeAssignableToTypeOf(repositories.PermissionDeniedOrNotFoundError{}))
				})
			})

			When("the user doesn't have permissions to see the org", func() {
				It("returns an error", func() {
					_, err := orgRepo.GetOrg(ctx, authInfo, orgAnchor.Name)
					Expect(err).To(BeAssignableToTypeOf(repositories.PermissionDeniedOrNotFoundError{}))
				})
			})
		})

		Describe("Space", func() {
			var (
				spaceAnchor *hnsv1alpha2.SubnamespaceAnchor
				orgAnchor   *hnsv1alpha2.SubnamespaceAnchor
			)

			BeforeEach(func() {
				orgAnchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "the-org")
				spaceAnchor = createSpaceAnchorAndNamespace(ctx, orgAnchor.Name, "the-space")
				createRoleBinding(ctx, userName, spaceDeveloperClusterRole.Name, spaceAnchor.Name)
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

			orgAnchor *hnsv1alpha2.SubnamespaceAnchor
		)

		BeforeEach(func() {
			ctx = context.Background()

			orgAnchor = createOrgAnchorAndNamespace(ctx, rootNamespace, "the-org")
		})

		Describe("Org", func() {
			When("the user has permission to delete orgs", func() {
				BeforeEach(func() {
					beforeCtx := context.Background()
					createRoleBinding(beforeCtx, userName, orgManagerClusterRole.Name, orgAnchor.Namespace)
					// As HNC Controllers don't exist in env-test environments, we manually copy role bindings to child ns.
					createRoleBinding(beforeCtx, userName, orgManagerClusterRole.Name, orgAnchor.Name)
				})

				When("on the happy path", func() {
					It("deletes the subns resource", func() {
						err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
							GUID: orgAnchor.Name,
						})
						Expect(err).NotTo(HaveOccurred())

						Eventually(func() error {
							anchor := &hnsv1alpha2.SubnamespaceAnchor{}
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
				When("the hierarchy object is missing", func() {
					BeforeEach(func() {
						Expect(k8sClient.Delete(ctx, &hnsv1alpha2.HierarchyConfiguration{ObjectMeta: metav1.ObjectMeta{
							Name:      "hierarchy",
							Namespace: orgAnchor.Name,
						}})).To(Succeed())

						Eventually(func() error {
							hierarchy := &hnsv1alpha2.HierarchyConfiguration{}
							return k8sClient.Get(ctx, client.ObjectKey{Namespace: orgAnchor.Name, Name: "hierarchy"}, hierarchy)
						}).Should(MatchError(ContainSubstring("not found")))
					})

					It("fails with an error", func() {
						err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
							GUID: orgAnchor.Name,
						})
						Expect(err).To(HaveOccurred())
					})
				})
			})

			When("the user does not have permission to delete orgs", func() {
				It("errors with forbidden", func() {
					err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
						GUID: orgAnchor.Name,
					})
					Expect(err).To(BeAssignableToTypeOf(authorization.InvalidAuthError{}))
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

			When("auth is disabled and", func() {
				BeforeEach(func() {
					orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, clientFactory, nsPerms, time.Millisecond*500, false)
				})

				When("on the happy path", func() {
					It("deletes the subns resource", func() {
						err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
							GUID: orgAnchor.Name,
						})
						Expect(err).NotTo(HaveOccurred())

						Eventually(func() error {
							anchor := &hnsv1alpha2.SubnamespaceAnchor{}
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
		})

		Describe("Space", func() {
			var spaceAnchor *hnsv1alpha2.SubnamespaceAnchor

			BeforeEach(func() {
				spaceAnchor = createSpaceAnchorAndNamespace(ctx, orgAnchor.Name, "the-space")
			})

			When("the user has permission to delete spaces and", func() {
				BeforeEach(func() {
					beforeCtx := context.Background()
					createRoleBinding(beforeCtx, userName, orgManagerClusterRole.Name, spaceAnchor.Namespace)
				})

				When("on the happy path", func() {
					It("deletes the subns resource", func() {
						err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
							GUID:             spaceAnchor.Name,
							OrganizationGUID: orgAnchor.Name,
						})
						Expect(err).NotTo(HaveOccurred())

						Eventually(func() error {
							anchor := &hnsv1alpha2.SubnamespaceAnchor{}
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

			When("the user does not have permission to delete spaces and", func() {
				It("errors with forbidden", func() {
					err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
						GUID:             spaceAnchor.Name,
						OrganizationGUID: orgAnchor.Name,
					})
					Expect(err).To(BeAssignableToTypeOf(authorization.InvalidAuthError{}))
				})

				When("the space doesn't exist", func() {
					It("errors with forbidden", func() {
						err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
							GUID:             "non-existent-space",
							OrganizationGUID: orgAnchor.Name,
						})
						Expect(err).To(BeAssignableToTypeOf(authorization.InvalidAuthError{}))
					})
				})
			})

			When("auth is disabled and", func() {
				BeforeEach(func() {
					orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, clientFactory, nsPerms, time.Millisecond*500, false)
				})

				When("on the happy path", func() {
					It("deletes the subns resource", func() {
						err := orgRepo.DeleteSpace(ctx, authInfo, repositories.DeleteSpaceMessage{
							GUID:             spaceAnchor.Name,
							OrganizationGUID: orgAnchor.Name,
						})
						Expect(err).NotTo(HaveOccurred())

						Eventually(func() error {
							anchor := &hnsv1alpha2.SubnamespaceAnchor{}
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
