package controllers

import (
	"context"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Reconciler interface {
	Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
	SetupWithManager(mgr ctrl.Manager) error
}

type CFAppController struct {
	Reconciler Reconciler
}


// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CFApp object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (c *CFAppController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return c.Reconciler.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (c *CFAppController) SetupWithManager(mgr ctrl.Manager) error {
	return c.Reconciler.SetupWithManager(mgr)
}