package client_interfaces

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/client"

)

//


type ShellCFAppClient struct {
	GetFunc func(ctx context.Context, key client.ObjectKey, obj client.Object) error
	StatusFunc func() client.StatusWriter
}
func (a *ShellCFAppClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return a.GetFunc(ctx, key, obj)
}
func (a *ShellCFAppClient) Status() client.StatusWriter {
	return a.StatusFunc()
}
