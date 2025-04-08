package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IgnoreEmptyPatchesClient struct {
	client.Client
}

func IgnoreEmptyPatches(k8sClient client.Client) *IgnoreEmptyPatchesClient {
	return &IgnoreEmptyPatchesClient{
		Client: k8sClient,
	}
}

func (c *IgnoreEmptyPatchesClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	isEmptyPatch, err := isEmptyPatch(patch, obj)
	if err != nil {
		return err
	}

	if isEmptyPatch {
		return nil
	}

	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *IgnoreEmptyPatchesClient) Status() client.SubResourceWriter {
	return &ignoreEmptyPatchesStatusWriter{
		StatusWriter: c.Client.Status(),
	}
}

type ignoreEmptyPatchesStatusWriter struct {
	client.StatusWriter
}

func (w *ignoreEmptyPatchesStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	isEmptyPatch, err := isEmptyPatch(patch, obj)
	if err != nil {
		return err
	}

	if isEmptyPatch {
		return nil
	}

	return w.StatusWriter.Patch(ctx, obj, patch, opts...)
}

func isEmptyPatch(patch client.Patch, obj client.Object) (bool, error) {
	patchData, err := patch.Data(obj)
	if err != nil {
		return false, fmt.Errorf("getting patch data failed: %w", err)
	}

	patchObj := map[string]any{}
	err = json.Unmarshal(patchData, &patchObj)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal patch data: %w", err)
	}

	return len(patchObj) == 0, nil
}
