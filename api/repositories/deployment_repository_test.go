package repositories_test

import (
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"code.cloudfoundry.org/korifi/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
)

var _ = Describe("DeploymentRepository", func() {
	var (
		deploymentRepo *repositories.DeploymentRepo
		cfOrg          *korifiv1alpha1.CFOrg
		cfSpace        *korifiv1alpha1.CFSpace
		cfApp          *korifiv1alpha1.CFApp
	)

	BeforeEach(func() {
		cfOrg = createOrgWithCleanup(ctx, prefixedGUID("org"))
		cfSpace = createSpaceWithCleanup(ctx, cfOrg.Name, prefixedGUID("space1"))
		cfApp = createApp(cfSpace.Name)

		Expect(k8sClient.Create(ctx, &korifiv1alpha1.AppWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cfApp.Namespace,
				Name:      uuid.NewString(),
				Labels: map[string]string{
					korifiv1alpha1.CFAppGUIDLabelKey: cfApp.Name,
				},
				Annotations: map[string]string{
					version.KorifiCreationVersionKey: "0.7.2",
				},
			},
		})).To(Succeed())

		deploymentRepo = repositories.NewDeploymentRepo(klient)
	})

	Describe("GetDeployment", func() {
		var (
			deployment repositories.DeploymentRecord
			getErr     error
			cfAppGUID  string
		)

		BeforeEach(func() {
			cfAppGUID = cfApp.Name
		})

		JustBeforeEach(func() {
			deployment, getErr = deploymentRepo.GetDeployment(ctx, authInfo, cfAppGUID)
		})

		It("returns a forbidden error (as the user is not allowed to get apps)", func() {
			Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("authorized in the space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg.Name)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("fetches the deployment", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(deployment.GUID).To(Equal(cfApp.Name))
				Expect(deployment.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
				Expect(deployment.Status.Value).To(Equal(repositories.DeploymentStatusValueActive))
				Expect(deployment.Status.Reason).To(Equal(repositories.DeploymentStatusReasonDeploying))
				Expect(deployment.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))

				Expect(deployment.Relationships()).To(Equal(map[string]string{
					"app": cfApp.Name,
				}))
			})

			When("the deployment is finalized", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, k8sClient, cfApp, func() {
						cfApp.Labels = tools.SetMapValue(cfApp.Labels, korifiv1alpha1.CFAppDeploymentStatusKey, korifiv1alpha1.DeploymentStatusValueFinalized)
					})).To(Succeed())
				})

				It("returns a finalized deployment", func() {
					Expect(getErr).NotTo(HaveOccurred())

					Expect(deployment.Status.Value).To(Equal(repositories.DeploymentStatusValueFinalized))
					Expect(deployment.Status.Reason).To(Equal(repositories.DeploymentStatusReasonDeployed))
				})
			})

			When("the app does not exist", func() {
				BeforeEach(func() {
					cfAppGUID = "i-do-not-exist"
				})

				It("returns a not found error", func() {
					Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})
		})
	})

	Describe("CreateDeployment", func() {
		var (
			createDeploymentMessage repositories.CreateDeploymentMessage
			deployment              repositories.DeploymentRecord
			createErr               error
		)

		BeforeEach(func() {
			createDeploymentMessage = repositories.CreateDeploymentMessage{
				AppGUID: cfApp.Name,
			}
		})

		JustBeforeEach(func() {
			deployment, createErr = deploymentRepo.CreateDeployment(ctx, authInfo, createDeploymentMessage)
		})

		It("returns a forbidden error (as the user is not allowed to get apps)", func() {
			Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("authorized in the space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg.Name)
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("creates the deployment", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(deployment.GUID).To(Equal(cfApp.Name))
				Expect(deployment.DropletGUID).To(Equal(cfApp.Spec.CurrentDropletRef.Name))
				Expect(deployment.Status.Value).To(Equal(repositories.DeploymentStatusValueActive))
				Expect(deployment.Status.Reason).To(Equal(repositories.DeploymentStatusReasonDeploying))
				Expect(deployment.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(deployment.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
			})

			It("bumps the app-rev annotation on the app", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				Expect(cfApp.Annotations).To(HaveKeyWithValue(CFAppRevisionKey, "2"))
			})

			It("sets the app desired state to STARTED", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				Expect(cfApp.Spec.DesiredState).To(Equal(korifiv1alpha1.AppState("STARTED")))
			})

			It("does not change the app droplet", func() {
				Expect(createErr).NotTo(HaveOccurred())

				currentDropletGUID := cfApp.Spec.CurrentDropletRef.Name
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				Expect(cfApp.Spec.CurrentDropletRef.Name).To(Equal(currentDropletGUID))
			})

			When("the app is ready", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, k8sClient, cfApp, func() {
						cfApp.Labels = tools.SetMapValue(cfApp.Labels, korifiv1alpha1.CFAppDeploymentStatusKey, korifiv1alpha1.DeploymentStatusValueFinalized)
					})).To(Succeed())
				})

				It("creates a finalized deployment", func() {
					Expect(createErr).NotTo(HaveOccurred())

					Expect(deployment.Status.Value).To(Equal(repositories.DeploymentStatusValueFinalized))
					Expect(deployment.Status.Reason).To(Equal(repositories.DeploymentStatusReasonDeployed))
				})
			})

			When("droplet guid is set on the create message", func() {
				var newDropletGUID string

				BeforeEach(func() {
					newDropletGUID = uuid.NewString()
					createDeploymentMessage.DropletGUID = newDropletGUID
				})

				It("sets the new droplet guid on the app", func() {
					Expect(createErr).NotTo(HaveOccurred())

					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
					Expect(cfApp.Spec.CurrentDropletRef.Name).To(Equal(newDropletGUID))
				})
			})

			When("the app does not exist", func() {
				BeforeEach(func() {
					createDeploymentMessage.AppGUID = "i-do-not-exist"
				})

				It("returns a not found error", func() {
					Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("one of the app workloads does not support rolling deployments", func() {
				BeforeEach(func() {
					Expect(k8sClient.Create(ctx, &korifiv1alpha1.AppWorkload{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: cfApp.Namespace,
							Name:      uuid.NewString(),
							Labels: map[string]string{
								korifiv1alpha1.CFAppGUIDLabelKey: cfApp.Name,
							},
						},
					})).To(Succeed())
				})

				It("returns an error", func() {
					Expect(createErr).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
				})
			})
		})
	})

	Describe("ListDeployments", func() {
		var (
			message     repositories.ListDeploymentsMessage
			deployments []repositories.DeploymentRecord
			anotherApp  *korifiv1alpha1.CFApp

			listErr error
		)

		BeforeEach(func() {
			unauthorisedSpace := createSpaceWithCleanup(ctx, cfOrg.Name, prefixedGUID("another-space"))
			createApp(unauthorisedSpace.Name)

			anotherApp = createApp(cfSpace.Name)
			message = repositories.ListDeploymentsMessage{}
		})

		JustBeforeEach(func() {
			deployments, listErr = deploymentRepo.ListDeployments(ctx, authInfo, message)
			Expect(listErr).NotTo(HaveOccurred())
		})

		It("returns an empty list", func() {
			Expect(deployments).To(BeEmpty())
		})

		When("the user is authorized in a space", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfSpace.Name)
			})

			It("returns the deployments from that namespace", func() {
				Expect(deployments).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(cfApp.Name),
					}),
					MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(anotherApp.Name),
					}),
				))
			})

			DescribeTable("ordering",
				func(msg repositories.ListDeploymentsMessage, match types.GomegaMatcher) {
					fakeKlient := new(fake.Klient)
					deploymentRepo = repositories.NewDeploymentRepo(fakeKlient)

					_, err := deploymentRepo.ListDeployments(ctx, authInfo, msg)
					Expect(err).NotTo(HaveOccurred())
					Expect(fakeKlient.ListCallCount()).To(Equal(1))
					_, _, listOptions := fakeKlient.ListArgsForCall(0)
					Expect(listOptions).To(match)
				},
				Entry("no ordering", repositories.ListDeploymentsMessage{}, Not(ContainElement(BeAssignableToTypeOf(repositories.SortOpt{})))),
				Entry("created_at", repositories.ListDeploymentsMessage{OrderBy: "created_at"}, ContainElement(repositories.SortBy("Created At", false))),
				Entry("-created_at", repositories.ListDeploymentsMessage{OrderBy: "-created_at"}, ContainElement(repositories.SortBy("Created At", true))),
				Entry("updated_at", repositories.ListDeploymentsMessage{OrderBy: "updated_at"}, ContainElement(repositories.SortBy("Updated At", false))),
				Entry("-updated_at", repositories.ListDeploymentsMessage{OrderBy: "-updated_at"}, ContainElement(repositories.SortBy("Updated At", true))),
				Entry("notexistent-field", repositories.ListDeploymentsMessage{OrderBy: "notexistent-field"}, ContainElement(repositories.ErroringListOption(`unsupported field for ordering: "notexistent-field"`))),
			)

			Describe("filtering", func() {
				Describe("by app guid", func() {
					BeforeEach(func() {
						message = repositories.ListDeploymentsMessage{
							AppGUIDs: []string{cfApp.Name},
						}
					})

					It("filters by app guids", func() {
						Expect(deployments).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
							"GUID": Equal(cfApp.Name),
						})))
					})
				})

				Describe("filter parameters to list options", func() {
					var fakeKlient *fake.Klient

					BeforeEach(func() {
						fakeKlient = new(fake.Klient)
						deploymentRepo = repositories.NewDeploymentRepo(fakeKlient)

						message = repositories.ListDeploymentsMessage{
							AppGUIDs:     []string{"a1", "a2"},
							StatusValues: []repositories.DeploymentStatusValue{"ACTIVE"},
							OrderBy:      "created_at",
						}
					})

					It("translates filter parameters to klient list options", func() {
						Expect(listErr).NotTo(HaveOccurred())
						Expect(fakeKlient.ListCallCount()).To(Equal(1))
						_, _, listOptions := fakeKlient.ListArgsForCall(0)
						Expect(listOptions).To(ConsistOf(
							repositories.WithLabelIn(korifiv1alpha1.GUIDLabelKey, []string{"a1", "a2"}),
							repositories.WithLabelIn(korifiv1alpha1.CFAppDeploymentStatusKey, []string{"ACTIVE"}),
							repositories.SortBy("Created At", false),
						))
					})
				})
			})
		})
	})
})
