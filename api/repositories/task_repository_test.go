package repositories_test

import (
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("TaskRepository", func() {
	var (
		taskRepo *repositories.TaskRepo
		org      *korifiv1alpha1.CFOrg
		space    *korifiv1alpha1.CFSpace
		cfApp    *korifiv1alpha1.CFApp
	)

	BeforeEach(func() {
		taskRepo = repositories.NewTaskRepo(userClientFactory, 2*time.Second)

		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))

		cfApp = createApp(space.Name)
	})

	Describe("CreateTask", func() {
		var (
			createMessage       repositories.CreateTaskMessage
			taskRecord          repositories.TaskRecord
			createErr           error
			dummyTaskController func(*korifiv1alpha1.CFTask) error
		)

		BeforeEach(func() {
			dummyTaskController = func(cft *korifiv1alpha1.CFTask) error {
				cft.Status.SequenceID = 6
				return k8sClient.Status().Update(ctx, cft)
			}
			createMessage = repositories.CreateTaskMessage{
				Command:   "  echo    hello  ",
				SpaceGUID: space.Name,
				AppGUID:   cfApp.Name,
			}
		})

		JustBeforeEach(func() {
			tasksWatch, err := k8sClient.Watch(ctx, &korifiv1alpha1.CFTaskList{}, client.InNamespace(space.Name))
			Expect(err).NotTo(HaveOccurred())
			defer tasksWatch.Stop()

			go func() {
				defer GinkgoRecover()

				for {
					select {
					case e := <-tasksWatch.ResultChan():
						cft, ok := e.Object.(*korifiv1alpha1.CFTask)
						if !ok {
							continue
						}

						Expect(dummyTaskController(cft)).To(Succeed())
						return
					case <-time.After(2 * time.Second):
						return
					}
				}
			}()

			taskRecord, createErr = taskRepo.CreateTask(ctx, authInfo, createMessage)
		})

		It("returns forbidden error", func() {
			Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user can create tasks", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, space.Name)
			})

			It("creates the task", func() {
				Expect(createErr).NotTo(HaveOccurred())
				Expect(taskRecord.Name).NotTo(BeEmpty())
				Expect(taskRecord.GUID).NotTo(BeEmpty())
				Expect(taskRecord.Command).To(Equal("echo hello"))
				Expect(taskRecord.AppGUID).To(Equal(cfApp.Name))
				Expect(taskRecord.SequenceID).NotTo(BeZero())
				Expect(taskRecord.CreationTimestamp).To(BeTemporally("~", time.Now(), time.Second))
			})

			When("the task never becomes ready", func() {
				BeforeEach(func() {
					dummyTaskController = func(cft *korifiv1alpha1.CFTask) error {
						return nil
					}
				})

				It("returns an error", func() {
					Expect(createErr).To(MatchError(ContainSubstring("did not become ready")))
				})
			})
		})

		When("unprivileged client creation fails", func() {
			BeforeEach(func() {
				authInfo = authorization.Info{}
			})

			It("returns an error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("failed to build user client")))
			})
		})
	})
})
