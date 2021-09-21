package repositories_test

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "code.cloudfoundry.org/cf-k8s-api/repositories"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sclevine/spec"
)

var _ = SuiteDescribe("Package Repository CreatePackage", func(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	when("on the happy path", func() {
		var (
			packageRepo   *PackageRepo
			client        client.Client
			packageCreate PackageCreate
			ctx           context.Context
		)

		const (
			appGUID   = "the-app-guid"
			spaceGUID = "the-space-guid"
		)

		it.Before(func() {
			packageRepo = new(PackageRepo)

			var err error
			client, err = BuildClient(k8sConfig)
			g.Expect(err).NotTo(HaveOccurred())

			packageCreate = PackageCreate{
				Type:      "bits",
				AppGUID:   appGUID,
				SpaceGUID: spaceGUID,
			}

			ctx = context.Background()
			g.Expect(
				k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}}),
			).To(Succeed())

		})

		it.After(func() {
			g.Expect(
				k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spaceGUID}}),
			).To(Succeed())
		})

		it("creates a Package record", func() {
			returnedPackageRecord, err := packageRepo.CreatePackage(ctx, client, packageCreate)
			g.Expect(err).NotTo(HaveOccurred())

			packageGUID := returnedPackageRecord.GUID
			g.Expect(packageGUID).NotTo(BeEmpty())
			g.Expect(returnedPackageRecord.Type).To(Equal("bits"))
			g.Expect(returnedPackageRecord.AppGUID).To(Equal(appGUID))
			g.Expect(returnedPackageRecord.State).To(Equal("AWAITING_UPLOAD"))

			createdAt, err := time.Parse(time.RFC3339, returnedPackageRecord.CreatedAt)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(createdAt).To(BeTemporally("~", time.Now(), time.Second))

			updatedAt, err := time.Parse(time.RFC3339, returnedPackageRecord.CreatedAt)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updatedAt).To(BeTemporally("~", time.Now(), time.Second))

			packageNSName := types.NamespacedName{Name: packageGUID, Namespace: spaceGUID}
			createdCFPackage := new(workloadsv1alpha1.CFPackage)
			g.Eventually(func() bool {
				err := k8sClient.Get(context.Background(), packageNSName, createdCFPackage)
				return err == nil
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())

			g.Expect(createdCFPackage.Name).To(Equal(packageGUID))
			g.Expect(createdCFPackage.Namespace).To(Equal(spaceGUID))
			g.Expect(createdCFPackage.Spec.Type).To(Equal(workloadsv1alpha1.PackageType("bits")))
			g.Expect(createdCFPackage.Spec.AppRef.Name).To(Equal(appGUID))

			g.Expect(cleanupPackage(ctx, k8sClient, packageGUID, spaceGUID)).To(Succeed())
		})
	})
})

func cleanupPackage(ctx context.Context, k8sClient client.Client, packageGUID, namespace string) error {
	cfPackage := workloadsv1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      packageGUID,
			Namespace: namespace,
		},
	}
	return k8sClient.Delete(ctx, &cfPackage)
}
