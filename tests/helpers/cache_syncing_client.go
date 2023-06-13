package helpers

import (
	"context"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CacheSyncingClient struct {
	client.Client
}

func NewCacheSyncingClient(client client.Client) *CacheSyncingClient {
	return &CacheSyncingClient{
		Client: client,
	}
}

func (c *CacheSyncingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if err := c.Client.Create(ctx, obj, opts...); err != nil {
		return err
	}

	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(c.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
	}).Should(Succeed())
	return nil
}

func (c *CacheSyncingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, _ ...client.PatchOption) error {
	err := c.Client.Patch(ctx, obj, patch)
	if err != nil {
		return err
	}

	generation := obj.GetGeneration()

	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(c.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		g.Expect(obj.GetGeneration()).To(BeNumerically(">=", generation))
	}).Should(Succeed())

	return nil
}

func (c *CacheSyncingClient) Status() client.SubResourceWriter {
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
	err := w.SubResourceWriter.Patch(ctx, obj, patch, opts...)
	if err != nil {
		return err
	}

	dryRunClient := client.NewDryRunClient(w.client)
	EventuallyWithOffset(1, func(g Gomega) {
		g.Expect(w.client.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		currentStatus, err := getStatus(obj)
		g.Expect(err).NotTo(HaveOccurred())

		objCopy, ok := obj.DeepCopyObject().(client.Object)
		g.Expect(ok).To(BeTrue())

		g.Expect(dryRunClient.Status().Patch(ctx, objCopy, patch, opts...)).To(Succeed())
		patchedStatus, err := getStatus(objCopy)
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(patchedStatus).To(Equal(currentStatus))
	}).Should(Succeed())

	return nil
}

func getStatus(obj runtime.Object) (any, error) {
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	return unstructuredObj["status"], nil
}
