package integration_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/controllers/coordination"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Name Registry", func() {
	var (
		ns1          *corev1.Namespace
		ctx          context.Context
		cancelCtx    context.CancelFunc
		nameRegistry coordination.NameRegistry
		name         string
	)

	BeforeEach(func() {
		nameRegistry = coordination.NewNameRegistry(controllersClient, "my-entity")
		ctx, cancelCtx = context.WithTimeout(context.Background(), 10*time.Second)
		ns1 = createNamespace(ctx, uuid.NewString())
		name = uuid.NewString()
	})

	AfterEach(func() {
		cancelCtx()
	})

	Describe("Registering a Name", func() {
		var ns2 *corev1.Namespace

		BeforeEach(func() {
			ns2 = createNamespace(ctx, uuid.NewString())
		})

		It("can register a unique name in a namespace", func() {
			Expect(nameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")).To(Succeed())
		})

		It("can register the same name in two namespaces", func() {
			Expect(nameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")).To(Succeed())
			Expect(nameRegistry.RegisterName(ctx, ns2.Name, name, "owner-namespace", "owner-name")).To(Succeed())
		})

		It("can register the same name for different entity types in the same namespace", func() {
			anotherNameRegistry := coordination.NewNameRegistry(controllersClient, "something-else")
			Expect(nameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")).To(Succeed())
			Expect(anotherNameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")).To(Succeed())
		})

		When("a name is already registered", func() {
			BeforeEach(func() {
				Expect(nameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")).To(Succeed())
			})

			It("returns an already exists error when trying to register that name again", func() {
				err := nameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")
				Expect(k8serrors.IsAlreadyExists(err)).To(BeTrue())
			})
		})

		When("a name is already registered but using a different registry with the same type", func() {
			BeforeEach(func() {
				Expect(nameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")).To(Succeed())
			})

			It("returns an already exists error when trying to register that name again", func() {
				anotherNameRegistry := coordination.NewNameRegistry(controllersClient, "my-entity")
				err := anotherNameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")
				Expect(k8serrors.IsAlreadyExists(err)).To(BeTrue())
			})
		})
	})

	Describe("Deregistering a name", func() {
		BeforeEach(func() {
			Expect(nameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")).To(Succeed())
		})

		It("can delete a registered name", func() {
			Expect(nameRegistry.DeregisterName(ctx, ns1.Name, name)).To(Succeed())
		})

		It("can re-register a deleted name", func() {
			Expect(nameRegistry.DeregisterName(ctx, ns1.Name, name)).To(Succeed())
			Expect(nameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")).To(Succeed())
		})

		When("the name is locked", func() {
			BeforeEach(func() {
				Expect(nameRegistry.TryLockName(ctx, ns1.Name, name)).To(Succeed())
			})

			It("can still be deregistered", func() {
				Expect(nameRegistry.DeregisterName(ctx, ns1.Name, name)).To(Succeed())
			})
		})

		When("the name doesn't exist", func() {
			It("does not return an error", func() {
				err := nameRegistry.DeregisterName(ctx, ns1.Name, "nope")
				Expect(k8serrors.IsNotFound(err)).To(BeFalse())
			})
		})
	})

	Describe("locking a name", func() {
		BeforeEach(func() {
			Expect(nameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")).To(Succeed())
		})

		It("can lock an unlocked name", func() {
			Expect(nameRegistry.TryLockName(ctx, ns1.Name, name)).To(Succeed())
		})

		When("the name is already locked", func() {
			BeforeEach(func() {
				Expect(nameRegistry.TryLockName(ctx, ns1.Name, name)).To(Succeed())
			})

			It("fails to lock the name", func() {
				err := nameRegistry.TryLockName(ctx, ns1.Name, name)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("unlocking a name", func() {
		BeforeEach(func() {
			Expect(nameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")).To(Succeed())
			Expect(nameRegistry.TryLockName(ctx, ns1.Name, name)).To(Succeed())
		})

		It("can unlock a locked name", func() {
			Expect(nameRegistry.UnlockName(ctx, ns1.Name, name)).To(Succeed())
		})

		When("the name is unlocked", func() {
			BeforeEach(func() {
				Expect(nameRegistry.UnlockName(ctx, ns1.Name, name)).To(Succeed())
			})

			It("succeeds anyway", func() {
				err := nameRegistry.UnlockName(ctx, ns1.Name, name)
				Expect(err).To(MatchError(ContainSubstring("failed to release lock on lease")))
			})
		})

		When("the name is locked an unlocked", func() {
			BeforeEach(func() {
				Expect(nameRegistry.UnlockName(ctx, ns1.Name, name)).To(Succeed())
			})

			It("can be re-locked", func() {
				Expect(nameRegistry.TryLockName(ctx, ns1.Name, name)).To(Succeed())
			})
		})
	})

	Describe("checking ownership", func() {
		var (
			ok             bool
			err            error
			ownerNamespace string
			ownerName      string
		)

		BeforeEach(func() {
			ok = false
			err = nil

			Expect(nameRegistry.RegisterName(ctx, ns1.Name, name, "owner-namespace", "owner-name")).To(Succeed())

			ownerNamespace = "owner-namespace"
			ownerName = "owner-name"
		})

		JustBeforeEach(func() {
			ok, err = nameRegistry.CheckNameOwnership(ctx, ns1.Name, name, ownerNamespace, ownerName)
		})

		It("returns true with the right owner values", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(ok).To(BeTrue())
		})

		When("owner namespace is wrong", func() {
			BeforeEach(func() {
				ownerNamespace = "bob"
			})

			It("returns false", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeFalse())
			})
		})

		When("owner name is wrong", func() {
			BeforeEach(func() {
				ownerName = "bob"
			})

			It("returns false", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeFalse())
			})
		})
	})
})

func createNamespace(ctx context.Context, name string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(adminClient.Create(ctx, ns)).To(Succeed())

	return ns
}
