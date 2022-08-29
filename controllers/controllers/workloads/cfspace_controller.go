/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workloads

import (
	"context"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	spaceFinalizerName = "cfSpace.korifi.cloudfoundry.org"
)

// CFSpaceReconciler reconciles a CFSpace object
type CFSpaceReconciler struct {
	client                    client.Client
	scheme                    *runtime.Scheme
	log                       logr.Logger
	packageRegistrySecretName string
	rootNamespace             string
}

func NewCFSpaceReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	packageRegistrySecretName string,
	rootNamespace string,
) *CFSpaceReconciler {
	return &CFSpaceReconciler{
		client:                    client,
		scheme:                    scheme,
		log:                       log,
		packageRegistrySecretName: packageRegistrySecretName,
		rootNamespace:             rootNamespace,
	}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfspaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfspaces/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfspaces/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=create;patch;delete;get;list;watch
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CFSpace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *CFSpaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cfSpace := new(korifiv1alpha1.CFSpace)
	err := r.client.Get(ctx, req.NamespacedName, cfSpace)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			r.log.Error(err, fmt.Sprintf("Error when trying to fetch CFSpace %s/%s", req.Namespace, req.Name))
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	readyCondition := getConditionOrSetAsUnknown(&cfSpace.Status.Conditions, korifiv1alpha1.ReadyConditionType)
	if readyCondition == metav1.ConditionUnknown {
		if err = r.client.Status().Update(ctx, cfSpace); err != nil {
			r.log.Error(err, fmt.Sprintf("Error when trying to set status conditions on CFSpace %s/%s", req.Namespace, req.Name))
			return ctrl.Result{}, err
		}
	}

	err = r.addFinalizer(ctx, cfSpace)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error adding finalizer on CFSpace %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	if !cfSpace.GetDeletionTimestamp().IsZero() {
		return finalize(ctx, r.client, r.log, cfSpace, spaceFinalizerName)
	}

	labels := map[string]string{korifiv1alpha1.SpaceNameLabel: cfSpace.Spec.DisplayName}
	err = createOrPatchNamespace(ctx, r.client, r.log, cfSpace, labels)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error creating namespace for CFSpace %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	namespace, ok := getNamespace(ctx, r.client, cfSpace.Name)
	if !ok {
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	err = propagateSecrets(ctx, r.client, r.log, cfSpace, r.packageRegistrySecretName)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error propagating secrets into CFSpace %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	err = reconcileRoleBindings(ctx, r.client, r.log, cfSpace)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error propagating role-bindings into CFSpace %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	err = r.reconcileServiceAccounts(ctx, cfSpace)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error propagating service accounts into CFSpace %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	cfSpace.Status.GUID = namespace.Name
	err = updateStatus(ctx, r.client, cfSpace, metav1.ConditionTrue)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error updating status on CFSpace %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFSpaceReconciler) reconcileServiceAccounts(ctx context.Context, space client.Object) error {
	var (
		result controllerutil.OperationResult
		err    error
	)

	serviceAccounts := new(corev1.ServiceAccountList)
	err = r.client.List(ctx, serviceAccounts, client.InNamespace(r.rootNamespace))
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error listing service accounts from namespace %s", space.GetNamespace()))
		return err
	}

	serviceAccountMap := make(map[string]struct{})
	for _, serviceAccount := range serviceAccounts.Items {
		if serviceAccount.Annotations[korifiv1alpha1.PropagateServiceAccountAnnotation] == "true" {

			serviceAccountMap[serviceAccount.Name] = struct{}{}

			newServiceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccount.Name,
					Namespace: space.GetName(),
				},
			}

			result, err = controllerutil.CreateOrPatch(ctx, r.client, newServiceAccount, func() error {
				newServiceAccount.Labels = serviceAccount.Labels
				if newServiceAccount.Labels == nil {
					newServiceAccount.Labels = map[string]string{}
				}
				newServiceAccount.Labels[korifiv1alpha1.PropagatedFromLabel] = r.rootNamespace
				newServiceAccount.Annotations = serviceAccount.Annotations
				newServiceAccount.ImagePullSecrets = serviceAccount.ImagePullSecrets
				newServiceAccount.Secrets = serviceAccount.Secrets
				return nil
			})
			if err != nil {
				r.log.Error(err, fmt.Sprintf("Error creating/patching service accounts %s/%s", newServiceAccount.Namespace, newServiceAccount.Name))
				return err
			}

			r.log.Info(fmt.Sprintf("Service Account %s/%s %s", newServiceAccount.Namespace, newServiceAccount.Name, result))

		}
	}

	propagatedServiceAccounts := new(corev1.ServiceAccountList)
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		korifiv1alpha1.PropagatedFromLabel: r.rootNamespace,
	})
	if err != nil {
		return err
	}

	err = r.client.List(ctx, propagatedServiceAccounts, &client.ListOptions{Namespace: space.GetName(), LabelSelector: labelSelector})
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error listing role-bindings from namespace %s", space.GetName()))
		return err
	}

	for index := range propagatedServiceAccounts.Items {
		propagatedServiceAccount := propagatedServiceAccounts.Items[index]
		if _, found := serviceAccountMap[propagatedServiceAccount.Name]; !found {
			err = r.client.Delete(ctx, &propagatedServiceAccount)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *CFSpaceReconciler) addFinalizer(ctx context.Context, cfSpace *korifiv1alpha1.CFSpace) error {
	if controllerutil.ContainsFinalizer(cfSpace, spaceFinalizerName) {
		return nil
	}

	originalCFSpace := cfSpace.DeepCopy()
	controllerutil.AddFinalizer(cfSpace, spaceFinalizerName)

	err := r.client.Patch(ctx, cfSpace, client.MergeFrom(originalCFSpace))
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error adding finalizer to CFSpace/%s", cfSpace.Name))
		return err
	}

	r.log.Info(fmt.Sprintf("Finalizer added to CFSpace/%s", cfSpace.Name))
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFSpaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFSpace{}).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFSpaceRequests),
		).
		Watches(
			&source.Kind{Type: &rbacv1.RoleBinding{}},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFSpaceRequests),
		).
		Watches(
			&source.Kind{Type: &corev1.ServiceAccount{}},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFSpaceRequestsForServiceAccount),
		).
		Complete(r)
}

func (r *CFSpaceReconciler) enqueueCFSpaceRequests(object client.Object) []reconcile.Request {
	cfSpaceList := &korifiv1alpha1.CFSpaceList{}
	err := r.client.List(context.Background(), cfSpaceList, client.InNamespace(object.GetNamespace()))
	if err != nil {
		return []reconcile.Request{}
	}
	requests := make([]reconcile.Request, len(cfSpaceList.Items))
	for i, space := range cfSpaceList.Items {
		requests[i] = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&space)}
	}
	return requests
}

func (r *CFSpaceReconciler) enqueueCFSpaceRequestsForServiceAccount(object client.Object) []reconcile.Request {
	if object.GetNamespace() != r.rootNamespace {
		return nil
	}

	cfSpaceList := &korifiv1alpha1.CFSpaceList{}
	err := r.client.List(context.Background(), cfSpaceList)
	if err != nil {
		return []reconcile.Request{}
	}
	requests := make([]reconcile.Request, len(cfSpaceList.Items))
	for i := range cfSpaceList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&cfSpaceList.Items[i]),
		}
	}
	return requests
}
