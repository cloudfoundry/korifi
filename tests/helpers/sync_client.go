package helpers

import (
	"context"

	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SyncClient struct {
	client.Client
}

func NewSyncClient(k8sClient client.Client) *SyncClient {
	return &SyncClient{
		Client: k8sClient,
	}
}

func (c *SyncClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	GinkgoHelper()

	if err := c.Client.Create(ctx, obj, opts...); err != nil {
		return err
	}

	Eventually(func(g Gomega) {
		g.Expect(c.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
	}).Should(Succeed())
	return nil
}

func (c *SyncClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, _ ...client.PatchOption) error {
	GinkgoHelper()

	err := c.Client.Patch(ctx, obj, patch)
	if err != nil {
		return err
	}

	generation := obj.GetGeneration()

	Eventually(func(g Gomega) {
		g.Expect(c.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		g.Expect(obj.GetGeneration()).To(BeNumerically(">=", generation))
	}).Should(Succeed())

	return nil
}

func (c *SyncClient) Status() client.SubResourceWriter {
	GinkgoHelper()

	return &syncStatusWriter{
		SubResourceWriter: c.Client.Status(),
		client:            c,
	}
}

type syncStatusWriter struct {
	client.SubResourceWriter
	client client.Client
}

func (w *syncStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	GinkgoHelper()

	err := w.SubResourceWriter.Patch(ctx, obj, patch, opts...)
	if err != nil {
		return err
	}

	dryRunClient := client.NewDryRunClient(w.client)
	Eventually(func(g Gomega) {
		g.Expect(w.client.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())

		currentStatus, err := getStatus(obj)
		g.Expect(err).NotTo(HaveOccurred())

		err = dryRunClient.Status().Patch(ctx, obj, patch, opts...)
		g.Expect(err).NotTo(HaveOccurred())
		patchedStatus, err := getStatus(obj)
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(patchedStatus).To(Equal(currentStatus))
	}).Should(Succeed())

	return nil
}

func getStatus(obj runtime.Object) (any, error) {
	GinkgoHelper()

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj.DeepCopyObject())
	if err != nil {
		return nil, err
	}

	return unstructuredObj["status"], nil
}
