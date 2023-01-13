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
	"strings"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/labels"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s_labels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
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
	client                      client.Client
	scheme                      *runtime.Scheme
	log                         logr.Logger
	containerRegistrySecretName string
	rootNamespace               string
	labelCompiler               labels.Compiler
}

func NewCFSpaceReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	containerRegistrySecretName string,
	rootNamespace string,
	labelCompiler labels.Compiler,
) *k8s.PatchingReconciler[korifiv1alpha1.CFSpace, *korifiv1alpha1.CFSpace] {
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFSpace, *korifiv1alpha1.CFSpace](log, client, &CFSpaceReconciler{
		client:                      client,
		scheme:                      scheme,
		log:                         log,
		containerRegistrySecretName: containerRegistrySecretName,
		rootNamespace:               rootNamespace,
		labelCompiler:               labelCompiler,
	})
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
func (r *CFSpaceReconciler) ReconcileResource(ctx context.Context, cfSpace *korifiv1alpha1.CFSpace) (ctrl.Result, error) {
	log := r.log.WithValues("namespace", cfSpace.Namespace, "name", cfSpace.Name)

	if !cfSpace.GetDeletionTimestamp().IsZero() {
		return r.finalize(ctx, log, cfSpace)
	}

	err := k8s.AddFinalizer(ctx, log, r.client, cfSpace, spaceFinalizerName)
	if err != nil {
		log.Error(err, "Error adding finalizer")
		return ctrl.Result{}, err
	}

	cfSpace.Status.GUID = cfSpace.GetName()

	getConditionOrSetAsUnknown(&cfSpace.Status.Conditions, korifiv1alpha1.ReadyConditionType)

	err = createOrPatchNamespace(ctx, r.client, log, cfSpace, r.labelCompiler.Compile(map[string]string{
		korifiv1alpha1.SpaceNameLabel: cfSpace.Spec.DisplayName,
	}))
	if err != nil {
		log.Error(err, "Error creating namespace")
		return ctrl.Result{}, err
	}

	err = getNamespace(ctx, log, r.client, cfSpace.Name)
	if err != nil {
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	err = propagateSecret(ctx, r.client, log, cfSpace, r.containerRegistrySecretName)
	if err != nil {
		log.Error(err, "Error propagating secrets")
		return ctrl.Result{}, err
	}

	err = reconcileRoleBindings(ctx, r.client, log, cfSpace)
	if err != nil {
		log.Error(err, "Error propagating role-bindings")
		return ctrl.Result{}, err
	}

	err = r.reconcileServiceAccounts(ctx, cfSpace, log)
	if err != nil {
		log.Error(err, "Error propagating service accounts")
		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&cfSpace.Status.Conditions, metav1.Condition{
		Type:   StatusConditionReady,
		Status: metav1.ConditionTrue,
		Reason: StatusConditionReady,
	})

	return ctrl.Result{}, nil
}

func (r *CFSpaceReconciler) reconcileServiceAccounts(ctx context.Context, space client.Object, log logr.Logger) error {
	log = log.WithName("reconcileServiceAccounts").
		WithValues("rootNamespace", r.rootNamespace, "targetNamespace", space.GetName())

	var (
		result controllerutil.OperationResult
		err    error
	)

	serviceAccounts := new(corev1.ServiceAccountList)
	err = r.client.List(ctx, serviceAccounts, client.InNamespace(r.rootNamespace))
	if err != nil {
		log.Error(err, "Error listing service accounts from root namespace")
		return err
	}

	serviceAccountMap := make(map[string]struct{})
	for _, rootServiceAccount := range serviceAccounts.Items {
		loopLog := log.WithValues("serviceAccountName", rootServiceAccount.Name)
		if rootServiceAccount.Annotations[korifiv1alpha1.PropagateServiceAccountAnnotation] == "true" {
			serviceAccountMap[rootServiceAccount.Name] = struct{}{}

			spaceServiceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rootServiceAccount.Name,
					Namespace: space.GetName(),
				},
			}

			var rootPackageRegistrySecret *corev1.ObjectReference
			var rootPackageRegistryImagePullSecret *corev1.LocalObjectReference

			// some versions of k8s will add their own secret/imagepullsecret references which will not be available in the new namespace, so we will only reference the package registry secret we explicitly propagate.
			for i := range rootServiceAccount.Secrets {
				if rootServiceAccount.Secrets[i].Name == r.containerRegistrySecretName {
					rootPackageRegistrySecret = &rootServiceAccount.Secrets[i]
					break
				}
			}
			for i := range rootServiceAccount.ImagePullSecrets {
				if rootServiceAccount.ImagePullSecrets[i].Name == r.containerRegistrySecretName {
					rootPackageRegistryImagePullSecret = &rootServiceAccount.ImagePullSecrets[i]
					break
				}
			}

			result, err = controllerutil.CreateOrPatch(ctx, r.client, spaceServiceAccount, func() error {
				spaceServiceAccount.Labels = rootServiceAccount.Labels
				if spaceServiceAccount.Labels == nil {
					spaceServiceAccount.Labels = map[string]string{}
				}
				spaceServiceAccount.Labels[korifiv1alpha1.PropagatedFromLabel] = r.rootNamespace
				spaceServiceAccount.Annotations = rootServiceAccount.Annotations

				spaceServiceAccount.Secrets = keepSecrets(spaceServiceAccount.Name, spaceServiceAccount.Secrets)
				if rootPackageRegistrySecret != nil {
					spaceServiceAccount.Secrets = append(spaceServiceAccount.Secrets, *rootPackageRegistrySecret)
				}

				spaceServiceAccount.ImagePullSecrets = keepImagePullSecrets(spaceServiceAccount.Name, spaceServiceAccount.ImagePullSecrets)
				if rootPackageRegistrySecret != nil {
					spaceServiceAccount.ImagePullSecrets = append(spaceServiceAccount.ImagePullSecrets, *rootPackageRegistryImagePullSecret)
				}

				return nil
			})
			if err != nil {
				loopLog.Error(err, "Error creating/patching service account")
				return err
			}

			loopLog.Info("Service Account propagated", "operation", result)

		}
	}

	propagatedServiceAccounts := new(corev1.ServiceAccountList)
	labelSelector, err := k8s_labels.ValidatedSelectorFromSet(map[string]string{
		korifiv1alpha1.PropagatedFromLabel: r.rootNamespace,
	})
	if err != nil {
		return err
	}

	err = r.client.List(ctx, propagatedServiceAccounts, &client.ListOptions{Namespace: space.GetName(), LabelSelector: labelSelector})
	if err != nil {
		log.Error(err, "Error listing role-bindings from target namespace")
		return err
	}

	for index := range propagatedServiceAccounts.Items {
		propagatedServiceAccount := propagatedServiceAccounts.Items[index]
		if _, found := serviceAccountMap[propagatedServiceAccount.Name]; !found {
			err = r.client.Delete(ctx, &propagatedServiceAccount)
			if err != nil {
				log.Error(err, "error deleting service account from the target namespace", "serviceAccount", propagatedServiceAccount.Name)
				return err
			}
		}
	}

	return nil
}

func keepSecrets(serviceAccountName string, secretRefs []corev1.ObjectReference) []corev1.ObjectReference {
	var results []corev1.ObjectReference
	for _, secretRef := range secretRefs {
		if strings.HasPrefix(secretRef.Name, serviceAccountName+"-token-") || strings.HasPrefix(secretRef.Name, serviceAccountName+"-dockercfg-") {
			results = append(results, secretRef)
		}
	}
	return results
}

func keepImagePullSecrets(serviceAccountName string, secretRefs []corev1.LocalObjectReference) []corev1.LocalObjectReference {
	var results []corev1.LocalObjectReference
	for _, secretRef := range secretRefs {
		if strings.HasPrefix(secretRef.Name, serviceAccountName+"-token-") || strings.HasPrefix(secretRef.Name, serviceAccountName+"-dockercfg-") {
			results = append(results, secretRef)
		}
	}
	return results
}

func (r *CFSpaceReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
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
		)
}

func (r *CFSpaceReconciler) enqueueCFSpaceRequests(object client.Object) []reconcile.Request {
	cfSpaceList := &korifiv1alpha1.CFSpaceList{}
	err := r.client.List(context.Background(), cfSpaceList, client.InNamespace(object.GetNamespace()))
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(cfSpaceList.Items))
	for i := range cfSpaceList.Items {
		requests[i] = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&cfSpaceList.Items[i])}
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

func (r *CFSpaceReconciler) finalize(ctx context.Context, log logr.Logger, space client.Object) (ctrl.Result, error) {
	log = log.WithName("finalize")

	if !controllerutil.ContainsFinalizer(space, spaceFinalizerName) {
		return ctrl.Result{}, nil
	}

	duration := time.Since(space.GetDeletionTimestamp().Time)
	log.Info(fmt.Sprintf("finalizing CFSpace for %fs", duration.Seconds()))
	if duration < 60.0*time.Second {
		err := r.finalizeCFApps(ctx, log, space.GetName())
		if err != nil {
			log.Info("failed to finalize CFApps while deleting CFSpace")
			return ctrl.Result{RequeueAfter: 500 * time.Millisecond}, nil
		}
	} else {
		log.Info("timed out finalizing CFApps while deleting CFSpace")
	}

	log.Info("deleting namespace while finalizing CFSpace")
	err := r.client.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: space.GetName()}})
	if err != nil {
		log.Error(err, "failed to delete namespace")
		return ctrl.Result{}, err
	}

	if controllerutil.RemoveFinalizer(space, spaceFinalizerName) {
		log.Info("finalizer removed")
	}

	return ctrl.Result{}, nil
}

func (r *CFSpaceReconciler) finalizeCFApps(ctx context.Context, log logr.Logger, namespace string) error {
	appList := korifiv1alpha1.CFAppList{}
	err := r.client.List(ctx, &appList, client.InNamespace(namespace))
	if err != nil {
		log.Error(err, "failed to list CFApps while finalizing CFSpace")
		return err
	}

	for i := range appList.Items {
		if appList.Items[i].GetDeletionTimestamp().IsZero() {
			log.Info(fmt.Sprintf("deleting CFApp %s", appList.Items[i].Name))
			err = r.client.Delete(ctx, &appList.Items[i], client.PropagationPolicy(metav1.DeletePropagationForeground))
			if err != nil {
				log.Error(err, "failed to delete CFApp", "AppName", appList.Items[i].Name)
			}
		}
	}

	var cfAppList korifiv1alpha1.CFAppList
	err = r.client.List(ctx, &cfAppList, client.InNamespace(namespace))
	if err != nil {
		log.Error(err, "failed to list CFApps while watching CFSpace")
		return fmt.Errorf("failed to list CFApps while watching CFSpace")
	}

	if len(cfAppList.Items) > 0 {
		err = fmt.Errorf("%d CFApps still found", len(cfAppList.Items))
		log.Info(fmt.Sprintf("%d CFApps still found", len(cfAppList.Items)))
		return err
	}

	return nil
}
