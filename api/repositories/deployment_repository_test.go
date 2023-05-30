package repositories_test

import (
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"code.cloudfoundry.org/korifi/version"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

		deploymentRepo = repositories.NewDeploymentRepo(userClientFactory, namespaceRetriever)
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

				_, err := time.Parse(repositories.TimestampFormat, deployment.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				_, err = time.Parse(repositories.TimestampFormat, deployment.UpdatedAt)
				Expect(err).NotTo(HaveOccurred())
			})

			When("the app is ready", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
						meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
							Type:   shared.StatusConditionReady,
							Status: metav1.ConditionTrue,
							Reason: "ready",
						})
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

				_, err := time.Parse(repositories.TimestampFormat, deployment.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				_, err = time.Parse(repositories.TimestampFormat, deployment.UpdatedAt)
				Expect(err).NotTo(HaveOccurred())
			})

			It("bumps the app-rev annotation on the app", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				Expect(cfApp.Annotations).To(HaveKeyWithValue(CFAppRevisionKey, "2"))
			})

			It("sets the app desired state to STARTED", func() {
				Expect(createErr).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				Expect(cfApp.Spec.DesiredState).To(Equal(korifiv1alpha1.DesiredState("STARTED")))
			})

			It("does not change the app droplet", func() {
				Expect(createErr).NotTo(HaveOccurred())

				currentDropletGUID := cfApp.Spec.CurrentDropletRef.Name
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfApp), cfApp)).To(Succeed())
				Expect(cfApp.Spec.CurrentDropletRef.Name).To(Equal(currentDropletGUID))
			})

			When("the app is ready", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, k8sClient, cfApp, func() {
						meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
							Type:   shared.StatusConditionReady,
							Status: metav1.ConditionTrue,
							Reason: "ready",
						})
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
					newDropletGUID = generateGUID()
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
})
