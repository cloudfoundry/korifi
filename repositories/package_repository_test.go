package repositories_test

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "code.cloudfoundry.org/cf-k8s-api/repositories"

	"github.com/sclevine/spec"
)

var _ = SuiteDescribe("Package Repository CreatePackage", testCreatePackage)
var _ = SuiteDescribe("Package Repository FetchPackage", testFetchPackage)

func testCreatePackage(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

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
}

func testFetchPackage(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	const (
		appGUID = "the-app-guid"
	)

	var (
		testCtx     context.Context
		packageRepo *PackageRepo
		client      client.Client

		namespace1 *corev1.Namespace
		namespace2 *corev1.Namespace
	)

	it.Before(func() {
		testCtx = context.Background()

		namespace1Name := generateGUID()
		namespace1 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace1Name}}
		g.Expect(k8sClient.Create(context.Background(), namespace1)).To(Succeed())

		namespace2Name := generateGUID()
		namespace2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace2Name}}
		g.Expect(k8sClient.Create(context.Background(), namespace2)).To(Succeed())

		packageRepo = new(PackageRepo)
		var err error
		client, err = BuildClient(k8sConfig)
		g.Expect(err).ToNot(HaveOccurred())
	})

	it.After(func() {
		g.Expect(k8sClient.Delete(context.Background(), namespace1)).To(Succeed())
		g.Expect(k8sClient.Delete(context.Background(), namespace2)).To(Succeed())
	})

	when("on the happy path", func() {
		var (
			package1GUID string
			package2GUID string
			package1     *workloadsv1alpha1.CFPackage
			package2     *workloadsv1alpha1.CFPackage
		)

		it.Before(func() {
			package1GUID = generateGUID()
			package2GUID = generateGUID()
			package1 = &workloadsv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      package1GUID,
					Namespace: namespace1.Name,
				},
				Spec: workloadsv1alpha1.CFPackageSpec{
					Type: "bits",
					AppRef: workloadsv1alpha1.ResourceReference{
						Name: appGUID,
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), package1)).To(Succeed())

			package2 = &workloadsv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      package2GUID,
					Namespace: namespace2.Name,
				},
				Spec: workloadsv1alpha1.CFPackageSpec{
					Type: "bits",
					AppRef: workloadsv1alpha1.ResourceReference{
						Name: appGUID,
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), package2)).To(Succeed())
		})

		it.After(func() {
			g.Expect(k8sClient.Delete(context.Background(), package1)).To(Succeed())
			g.Expect(k8sClient.Delete(context.Background(), package2)).To(Succeed())
		})

		it("can fetch the PackageRecord we're looking for", func() {
			record, err := packageRepo.FetchPackage(testCtx, client, package2GUID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(record.GUID).To(Equal(package2GUID))
			g.Expect(record.Type).To(Equal("bits"))
			g.Expect(record.AppGUID).To(Equal(appGUID))
			g.Expect(record.State).To(Equal("AWAITING_UPLOAD"))

			createdAt, err := time.Parse(time.RFC3339, record.CreatedAt)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(createdAt).To(BeTemporally("~", time.Now(), time.Second))

			updatedAt, err := time.Parse(time.RFC3339, record.CreatedAt)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updatedAt).To(BeTemporally("~", time.Now(), time.Second))
		})
	})

	when("duplicate Packages exist across namespaces with the same GUID", func() {
		var (
			packageGUID string
			cfPackage1  *workloadsv1alpha1.CFPackage
			cfPackage2  *workloadsv1alpha1.CFPackage
		)

		it.Before(func() {
			packageGUID = generateGUID()
			cfPackage1 = &workloadsv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      packageGUID,
					Namespace: namespace1.Name,
				},
				Spec: workloadsv1alpha1.CFPackageSpec{
					Type: "bits",
					AppRef: workloadsv1alpha1.ResourceReference{
						Name: appGUID,
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), cfPackage1)).To(Succeed())

			cfPackage2 = &workloadsv1alpha1.CFPackage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      packageGUID,
					Namespace: namespace2.Name,
				},
				Spec: workloadsv1alpha1.CFPackageSpec{
					Type: "bits",
					AppRef: workloadsv1alpha1.ResourceReference{
						Name: appGUID,
					},
				},
			}
			g.Expect(k8sClient.Create(context.Background(), cfPackage2)).To(Succeed())
		})

		it.After(func() {
			g.Expect(k8sClient.Delete(context.Background(), cfPackage1)).To(Succeed())
			g.Expect(k8sClient.Delete(context.Background(), cfPackage2)).To(Succeed())
		})

		it("returns an error", func() {
			_, err := packageRepo.FetchPackage(testCtx, client, packageGUID)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError("duplicate packages exist"))
		})
	})

	when("no packages exist", func() {
		it("returns an error", func() {
			_, err := packageRepo.FetchPackage(testCtx, client, "i don't exist")
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError(NotFoundError{}))
		})
	})
}

func cleanupPackage(ctx context.Context, k8sClient client.Client, packageGUID, namespace string) error {
	cfPackage := workloadsv1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      packageGUID,
			Namespace: namespace,
		},
	}
	return k8sClient.Delete(ctx, &cfPackage)
}
