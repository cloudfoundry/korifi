package coordination_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/controllers/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("NameRegistry", func() {
	var (
		nameRegistry coordination.NameRegistry
		client       *fake.Client
		ctx          context.Context

		entityType string
		namespace  string
		name       string
		err        error
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = new(fake.Client)
		entityType = "my-type"
		nameRegistry = coordination.NewNameRegistry(client, entityType)

		namespace = "my-namespace"
		name = "my-name"
	})

	Describe("RegisterName", func() {
		JustBeforeEach(func() {
			err = nameRegistry.RegisterName(ctx, namespace, name)
		})

		It("creates a lease", func() {
			Expect(err).NotTo(HaveOccurred())

			Expect(client.CreateCallCount()).To(Equal(1))
			_, obj, _ := client.CreateArgsForCall(0)
			Expect(obj).To(BeAssignableToTypeOf(&coordinationv1.Lease{}))
			lease := obj.(*coordinationv1.Lease)
			Expect(lease.Namespace).To(Equal(namespace))
			Expect(lease.Name).To(HavePrefix("n-"))
			none := "none"
			Expect(lease.Spec.HolderIdentity).To(Equal(&none))
		})

		It("annotates the lease with the parameters from the registration", func() {
			Expect(err).NotTo(HaveOccurred())

			Expect(client.CreateCallCount()).To(Equal(1))
			_, obj, _ := client.CreateArgsForCall(0)
			Expect(obj).To(BeAssignableToTypeOf(&coordinationv1.Lease{}))
			lease := obj.(*coordinationv1.Lease)
			Expect(lease.Annotations).To(SatisfyAll(
				HaveKeyWithValue("coordination.cloudfoundry.org/entity-type", "my-type"),
				HaveKeyWithValue("coordination.cloudfoundry.org/namespace", "my-namespace"),
				HaveKeyWithValue("coordination.cloudfoundry.org/name", "my-name"),
			))
		})

		When("create returns an already exists error", func() {
			BeforeEach(func() {
				client.CreateReturns(k8serrors.NewAlreadyExists(schema.GroupResource{}, "bob"))
			})

			It("returns another already exists error", func() {
				Expect(k8serrors.IsAlreadyExists(err)).To(BeTrue())
				Expect(err).To(MatchError(ContainSubstring("creating a lease failed")))
			})
		})

		When("in a dry-run request context", func() {
			BeforeEach(func() {
				ctx = admission.NewContextWithRequest(ctx, admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						DryRun: tools.PtrTo(true),
					},
				})
			})

			It("does not create a lease", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(client.CreateCallCount()).To(BeZero())
			})
		})
	})

	Describe("DeregisterName", func() {
		JustBeforeEach(func() {
			err = nameRegistry.DeregisterName(ctx, namespace, name)
		})

		It("deletes the lease", func() {
			Expect(err).NotTo(HaveOccurred())

			Expect(client.DeleteCallCount()).To(Equal(1))
			_, obj, _ := client.DeleteArgsForCall(0)
			Expect(obj).To(BeAssignableToTypeOf(&coordinationv1.Lease{}))
			lease := obj.(*coordinationv1.Lease)
			Expect(lease.Namespace).To(Equal(namespace))
			Expect(lease.Name).To(HavePrefix("n-"))
		})

		When("the lease does not exist", func() {
			BeforeEach(func() {
				client.DeleteReturns(k8serrors.NewNotFound(schema.GroupResource{}, "some-name"))
			})

			It("does not return an error", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("deleting the lease fails", func() {
			BeforeEach(func() {
				client.DeleteReturns(errors.New("boom!"))
			})

			It("returns it", func() {
				Expect(err).To(MatchError(SatisfyAll(
					ContainSubstring("boom!"),
					ContainSubstring("deleting a lease failed"),
				)))
			})
		})

		When("in a dry-run request context", func() {
			BeforeEach(func() {
				ctx = admission.NewContextWithRequest(ctx, admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						DryRun: tools.PtrTo(true),
					},
				})
			})

			It("does not delete the lease", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(client.DeleteCallCount()).To(BeZero())
			})
		})
	})

	Describe("TryLockName", func() {
		JustBeforeEach(func() {
			err = nameRegistry.TryLockName(ctx, namespace, name)
		})

		It("acquires the lock atomically using test and replace patching", func() {
			Expect(err).NotTo(HaveOccurred())

			Expect(client.PatchCallCount()).To(Equal(1))
			_, obj, patch, _ := client.PatchArgsForCall(0)
			Expect(obj).To(BeAssignableToTypeOf(&coordinationv1.Lease{}))
			lease := obj.(*coordinationv1.Lease)
			Expect(lease.Namespace).To(Equal(namespace))
			Expect(lease.Name).To(HavePrefix("n-"))

			Expect(patch.Type()).To(Equal(types.JSONPatchType))
			data, dataErr := patch.Data(nil)
			Expect(dataErr).NotTo(HaveOccurred())
			Expect(string(data)).To(SatisfyAll(
				ContainSubstring("test"),
				ContainSubstring("replace"),
				ContainSubstring("locked"),
			))
		})

		When("patching fails", func() {
			BeforeEach(func() {
				client.PatchReturnsOnCall(0, errors.New("boom!"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(SatisfyAll(
					ContainSubstring("boom!"),
					ContainSubstring("failed to acquire lock"),
				)))
			})
		})

		When("patching fails with NotFound but eventually succeeds", func() {
			BeforeEach(func() {
				client.PatchReturns(k8serrors.NewNotFound(schema.GroupResource{}, "boom!"))
				client.PatchReturnsOnCall(5, nil)
			})

			It("succeeds", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("in a dry-run request context", func() {
			BeforeEach(func() {
				ctx = admission.NewContextWithRequest(ctx, admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						DryRun: tools.PtrTo(true),
					},
				})
			})

			It("does not patch the lease", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(client.PatchCallCount()).To(BeZero())
			})
		})
	})

	Describe("UnlockName", func() {
		JustBeforeEach(func() {
			err = nameRegistry.UnlockName(ctx, namespace, name)
		})

		It("patches the lease to reset the holder identity", func() {
			Expect(err).NotTo(HaveOccurred())

			Expect(client.PatchCallCount()).To(Equal(1))
			_, obj, patch, _ := client.PatchArgsForCall(0)
			Expect(obj).To(BeAssignableToTypeOf(&coordinationv1.Lease{}))
			lease := obj.(*coordinationv1.Lease)
			Expect(lease.Namespace).To(Equal(namespace))
			Expect(lease.Name).To(HavePrefix("n-"))

			Expect(patch.Type()).To(Equal(types.JSONPatchType))
			data, dataErr := patch.Data(nil)
			Expect(dataErr).NotTo(HaveOccurred())
			Expect(string(data)).To(SatisfyAll(
				ContainSubstring("test"),
				ContainSubstring("replace"),
				ContainSubstring("none"),
			))
		})

		When("patching fails", func() {
			BeforeEach(func() {
				client.PatchReturns(errors.New("boom!"))
			})

			It("returns the error", func() {
				Expect(err).To(MatchError(SatisfyAll(
					ContainSubstring("boom!"),
					ContainSubstring("failed to release lock on lease"),
				)))
			})
		})

		When("in a dry-run request context", func() {
			BeforeEach(func() {
				ctx = admission.NewContextWithRequest(ctx, admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						DryRun: tools.PtrTo(true),
					},
				})
			})

			It("does not patch the lease", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(client.PatchCallCount()).To(BeZero())
			})
		})
	})
})
