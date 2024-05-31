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

package spaces

import (
	"context"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/k8sns"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/labels"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s_labels "k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	client                       client.Client
	namespaceReconciler          *k8sns.Reconciler[korifiv1alpha1.CFSpace, *korifiv1alpha1.CFSpace]
	containerRegistrySecretNames []string
	rootNamespace                string
	appDeletionTimeout           int64
}

func NewReconciler(
	client client.Client,
	log logr.Logger,
	containerRegistrySecretNames []string,
	rootNamespace string,
	appDeletionTimeout int64,
	labelCompiler labels.Compiler,
) *k8s.PatchingReconciler[korifiv1alpha1.CFSpace, *korifiv1alpha1.CFSpace] {
	namespaceController := k8sns.NewReconciler[korifiv1alpha1.CFSpace, *korifiv1alpha1.CFSpace](
		client,
		k8sns.NewNamespaceFinalizer[korifiv1alpha1.CFSpace, *korifiv1alpha1.CFSpace](
			client,
			k8sns.NewSpaceAppsFinalizer(client, appDeletionTimeout),
			korifiv1alpha1.CFSpaceFinalizerName,
		),
		&cfSpaceMetadataCompiler{
			labelCompiler: labelCompiler,
		},
		containerRegistrySecretNames,
	)

	return k8s.NewPatchingReconciler[korifiv1alpha1.CFSpace, *korifiv1alpha1.CFSpace](log, client, &Reconciler{
		client:                       client,
		namespaceReconciler:          namespaceController,
		rootNamespace:                rootNamespace,
		appDeletionTimeout:           appDeletionTimeout,
		containerRegistrySecretNames: containerRegistrySecretNames,
	})
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFSpace{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFSpaceRequests),
		).
		Watches(
			&rbacv1.RoleBinding{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFSpaceRequests),
		).
		Watches(
			&corev1.ServiceAccount{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueCFSpaceRequestsForServiceAccount),
		)
}

func (r *Reconciler) enqueueCFSpaceRequests(ctx context.Context, object client.Object) []reconcile.Request {
	cfSpaceList := &korifiv1alpha1.CFSpaceList{}
	err := r.client.List(ctx, cfSpaceList, client.InNamespace(object.GetNamespace()))
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(cfSpaceList.Items))
	for i := range cfSpaceList.Items {
		requests[i] = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&cfSpaceList.Items[i])}
	}

	return requests
}

func (r *Reconciler) enqueueCFSpaceRequestsForServiceAccount(ctx context.Context, object client.Object) []reconcile.Request {
	if object.GetNamespace() != r.rootNamespace {
		return nil
	}

	cfSpaceList := &korifiv1alpha1.CFSpaceList{}
	err := r.client.List(ctx, cfSpaceList)
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

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfspaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfspaces/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfspaces/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=create;patch;delete;get;list;watch
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;patch;delete

func (r *Reconciler) ReconcileResource(ctx context.Context, cfSpace *korifiv1alpha1.CFSpace) (ctrl.Result, error) {
	var err error
	readyConditionBuilder := k8s.NewReadyConditionBuilder(cfSpace)
	defer func() {
		meta.SetStatusCondition(&cfSpace.Status.Conditions, readyConditionBuilder.WithError(err).Build())
	}()

	nsReconcileResult, err := r.namespaceReconciler.ReconcileResource(ctx, cfSpace)
	if (nsReconcileResult != ctrl.Result{}) || (err != nil) {
		return nsReconcileResult, err
	}

	log := logr.FromContextOrDiscard(ctx)

	err = r.reconcileServiceAccounts(ctx, cfSpace)
	if err != nil {
		log.Info("not ready yet", "reason", "error propagating service accounts", "error", err)

		readyConditionBuilder.WithReason("ServiceAccountPropagation")
		return ctrl.Result{}, err
	}

	readyConditionBuilder.Ready()
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileServiceAccounts(ctx context.Context, space client.Object) error {
	log := logr.FromContextOrDiscard(ctx).WithName("reconcileServiceAccounts").
		WithValues("rootNamespace", r.rootNamespace, "targetNamespace", space.GetName())

	serviceAccounts := new(corev1.ServiceAccountList)
	err := r.client.List(ctx, serviceAccounts, client.InNamespace(r.rootNamespace))
	if err != nil {
		log.Info("error listing service accounts from root namespace", "reason", err)
		return err
	}

	var result controllerutil.OperationResult
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

			var rootPackageRegistrySecrets []corev1.ObjectReference
			var rootPackageRegistryImagePullSecrets []corev1.LocalObjectReference

			// Some versions of K8s will add their own secret/imagepullsecret references which will not be available in the new namespace, so we will only reference the package registry secret we explicitly propagate.
			for _, secretName := range r.containerRegistrySecretNames {
				for _, secret := range rootServiceAccount.Secrets {
					if secret.Name == secretName {
						rootPackageRegistrySecrets = append(rootPackageRegistrySecrets, secret)
						break
					}
				}
				for _, secret := range rootServiceAccount.ImagePullSecrets {
					if secret.Name == secretName {
						rootPackageRegistryImagePullSecrets = append(rootPackageRegistryImagePullSecrets, secret)
						break
					}
				}
			}

			result, err = controllerutil.CreateOrPatch(ctx, r.client, spaceServiceAccount, func() error {
				spaceServiceAccount.Annotations = shared.RemovePackageManagerKeys(rootServiceAccount.Annotations, loopLog)

				spaceServiceAccount.Labels = shared.RemovePackageManagerKeys(rootServiceAccount.Labels, loopLog)
				if spaceServiceAccount.Labels == nil {
					spaceServiceAccount.Labels = map[string]string{}
				}
				spaceServiceAccount.Labels[korifiv1alpha1.PropagatedFromLabel] = r.rootNamespace

				spaceServiceAccount.Secrets = keepSecrets(spaceServiceAccount.Name, spaceServiceAccount.Secrets)
				spaceServiceAccount.Secrets = append(spaceServiceAccount.Secrets, rootPackageRegistrySecrets...)

				spaceServiceAccount.ImagePullSecrets = keepImagePullSecrets(spaceServiceAccount.Name, spaceServiceAccount.ImagePullSecrets)
				spaceServiceAccount.ImagePullSecrets = append(spaceServiceAccount.ImagePullSecrets, rootPackageRegistryImagePullSecrets...)

				return nil
			})
			if err != nil {
				loopLog.Info("error creating/patching service account", "reason", err)
				return err
			}

			loopLog.V(1).Info("service Account propagated", "operation", result)
		}
	}

	propagatedServiceAccounts := new(corev1.ServiceAccountList)
	labelSelector, err := k8s_labels.ValidatedSelectorFromSet(map[string]string{
		korifiv1alpha1.PropagatedFromLabel: r.rootNamespace,
	})
	if err != nil {
		log.Info("failed to create label selector", "reason", err)
		return err
	}

	err = r.client.List(ctx, propagatedServiceAccounts, &client.ListOptions{Namespace: space.GetName(), LabelSelector: labelSelector})
	if err != nil {
		log.Info("error listing role-bindings from target namespace", "reason", err)
		return err
	}

	for index := range propagatedServiceAccounts.Items {
		propagatedServiceAccount := propagatedServiceAccounts.Items[index]
		if propagatedServiceAccount.Annotations[korifiv1alpha1.PropagateDeletionAnnotation] == "false" {
			continue
		}

		if _, found := serviceAccountMap[propagatedServiceAccount.Name]; !found {
			err = r.client.Delete(ctx, &propagatedServiceAccount)
			if err != nil {
				log.Info("error deleting service account from the target namespace", "serviceAccount", propagatedServiceAccount.Name, "reason", err)
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

type cfSpaceMetadataCompiler struct {
	labelCompiler labels.Compiler
}

func (c *cfSpaceMetadataCompiler) CompileLabels(cfSpace *korifiv1alpha1.CFSpace) map[string]string {
	return c.labelCompiler.Compile(map[string]string{
		korifiv1alpha1.SpaceNameKey: korifiv1alpha1.OrgSpaceDeprecatedName,
		korifiv1alpha1.SpaceGUIDKey: cfSpace.Name,
	})
}

func (c *cfSpaceMetadataCompiler) CompileAnnotations(cfSpace *korifiv1alpha1.CFSpace) map[string]string {
	return map[string]string{
		korifiv1alpha1.SpaceNameKey: cfSpace.Spec.DisplayName,
	}
}
