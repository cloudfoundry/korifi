package repositories_test

import (
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PodRepository", func() {
	var (
		podRepo     *repositories.PodRepo
		org         *korifiv1alpha1.CFOrg
		space       *korifiv1alpha1.CFSpace
		pod         *corev1.Pod
		appGUID     string
		appRevision string
		instance    string
		process     repositories.ProcessRecord
	)

	BeforeEach(func() {
		instance = "2"
		appRevision = "1"
		podRepo = repositories.NewPodRepo(userClientFactory)
		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))
		appGUID = uuid.NewString()
		process = repositories.ProcessRecord{
			AppGUID:          appGUID,
			SpaceGUID:        space.Name,
			Type:             "web",
			DesiredInstances: 3,
		}
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "podname-2",
				Namespace: space.Name,
				Labels: map[string]string{
					"korifi.cloudfoundry.org/app-guid":     appGUID,
					"korifi.cloudfoundry.org/version":      "1",
					"korifi.cloudfoundry.org/process-type": process.Type,
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "web",
						Image: "nginx",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
	})

	Describe("DeletePod", func() {
		var err error

		JustBeforeEach(func() {
			err = podRepo.DeletePod(ctx, authInfo, appRevision, process, instance)
		})

		It("fails to delete the pod", func() {
			Expect(err).To(HaveOccurred())
			Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a SpaceDeveloper", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("deletes the pod", func() {
				Expect(err).ToNot(HaveOccurred())
				Eventually(func(g Gomega) {
					err = k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &corev1.Pod{})
					g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})

			When("the instance does not exist", func() {
				BeforeEach(func() {
					instance = "3"
				})

				It("returns a not found error", func() {
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("there are no pods referencing the app", func() {
				BeforeEach(func() {
					process.AppGUID = "app-does-not-exist"
				})

				It("returns a not found error", func() {
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("there are no pod for the app version specified", func() {
				BeforeEach(func() {
					appRevision = "rev-does-not-exist"
				})

				It("returns a not found error", func() {
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("there are no pods matching the process type", func() {
				BeforeEach(func() {
					process.Type = "type-does-not-exist"
				})

				It("returns a not found error", func() {
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("there are multiple matching pods", func() {
				BeforeEach(func() {
					Expect(k8sClient.Create(ctx, &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "anotherpodname-2",
							Namespace: space.Name,
							Labels: map[string]string{
								"korifi.cloudfoundry.org/app-guid":     appGUID,
								"korifi.cloudfoundry.org/version":      "1",
								"korifi.cloudfoundry.org/process-type": process.Type,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "web",
									Image: "nginx",
								},
							},
						},
					})).To(Succeed())
				})

				It("returns an unprocessable entity error", func() {
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
				})
			})
		})
	})
})
