package apps

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type EnvValueBuilder interface {
	BuildEnvValue(context.Context, *korifiv1alpha1.CFApp) (map[string][]byte, error)
}

type Reconciler struct {
	log                       logr.Logger
	k8sClient                 client.Client
	scheme                    *runtime.Scheme
	vcapServicesEnvBuilder    EnvValueBuilder
	vcapApplicationEnvBuilder EnvValueBuilder
}

func NewReconciler(k8sClient client.Client, scheme *runtime.Scheme, log logr.Logger, vcapServicesBuilder, vcapApplicationBuilder EnvValueBuilder) *k8s.PatchingReconciler[korifiv1alpha1.CFApp, *korifiv1alpha1.CFApp] {
	appReconciler := Reconciler{
		log:                       log,
		k8sClient:                 k8sClient,
		scheme:                    scheme,
		vcapServicesEnvBuilder:    vcapServicesBuilder,
		vcapApplicationEnvBuilder: vcapApplicationBuilder,
	}
	return k8s.NewPatchingReconciler(log, k8sClient, &appReconciler)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFApp{}).
		Owns(&korifiv1alpha1.CFProcess{}).
		Watches(
			&korifiv1alpha1.CFBuild{},
			handler.EnqueueRequestsFromMapFunc(buildToApp),
		).
		Watches(
			&korifiv1alpha1.CFServiceBinding{},
			handler.EnqueueRequestsFromMapFunc(serviceBindingToApp),
		)
}

func buildToApp(ctx context.Context, o client.Object) []reconcile.Request {
	cfBuild, ok := o.(*korifiv1alpha1.CFBuild)
	if !ok {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      cfBuild.Spec.AppRef.Name,
				Namespace: o.GetNamespace(),
			},
		},
	}
}

func serviceBindingToApp(ctx context.Context, o client.Object) []reconcile.Request {
	serviceBinding, ok := o.(*korifiv1alpha1.CFServiceBinding)
	if !ok {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      serviceBinding.Spec.AppRef.Name,
				Namespace: o.GetNamespace(),
			},
		},
	}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfapps,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfapps/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;patch

func (r *Reconciler) ReconcileResource(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	cfApp.Status.ObservedGeneration = cfApp.Generation
	log.V(1).Info("set observed generation", "generation", cfApp.Status.ObservedGeneration)

	cfApp.Status.ActualState = korifiv1alpha1.StoppedState

	if !cfApp.GetDeletionTimestamp().IsZero() {
		return r.finalizeCFApp(ctx, cfApp)
	}

	if cfApp.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey] == "" {
		cfApp.Annotations = tools.SetMapValue(cfApp.Annotations, korifiv1alpha1.CFAppLastStopRevisionKey, cfApp.Annotations[korifiv1alpha1.CFAppRevisionKey])
	}

	bindings, err := r.getServiceBindings(ctx, cfApp)
	if err != nil {
		return ctrl.Result{}, err
	}
	cfApp.Status.ServiceBindings = bindingObjectRefs(bindings)

	if !bindingsReady(bindings) {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("BindingNotReady")
	}

	secretName := cfApp.Name + "-vcap-application"
	err = r.reconcileVCAPSecret(ctx, cfApp, secretName, r.vcapApplicationEnvBuilder)
	if err != nil {
		return ctrl.Result{}, err
	}
	cfApp.Status.VCAPApplicationSecretName = secretName

	secretName = cfApp.Name + "-vcap-services"
	err = r.reconcileVCAPSecret(ctx, cfApp, secretName, r.vcapServicesEnvBuilder)
	if err != nil {
		return ctrl.Result{}, err
	}

	cfApp.Status.VCAPServicesSecretName = secretName

	if cfApp.Spec.CurrentDropletRef.Name == "" {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("DropletNotAssigned")
	}

	droplet, err := r.getDroplet(ctx, cfApp)
	if err != nil {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("CannotResolveCurrentDropletRef")
	}

	reconciledProcesses, err := r.reconcileProcesses(ctx, cfApp, droplet)
	if err != nil {
		return ctrl.Result{}, err
	}

	cfApp.Status.ActualState = getActualState(reconciledProcesses)
	if cfApp.Status.ActualState != cfApp.Spec.DesiredState {
		return ctrl.Result{}, k8s.NewNotReadyError().WithReason("DesiredStateNotReached")
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) getServiceBindings(ctx context.Context, cfApp *korifiv1alpha1.CFApp) ([]korifiv1alpha1.CFServiceBinding, error) {
	bindings := &korifiv1alpha1.CFServiceBindingList{}
	if err := r.k8sClient.List(ctx, bindings,
		client.InNamespace(cfApp.Namespace),
		client.MatchingFields{shared.IndexServiceBindingAppGUID: cfApp.Name},
	); err != nil {
		return nil, err
	}

	return slices.Collect(it.Exclude(slices.Values(bindings.Items), func(b korifiv1alpha1.CFServiceBinding) bool {
		return b.DeletionTimestamp != nil
	})), nil
}

func bindingsReady(bindings []korifiv1alpha1.CFServiceBinding) bool {
	return it.All(it.Map(slices.Values(bindings), func(binding korifiv1alpha1.CFServiceBinding) bool {
		return meta.IsStatusConditionTrue(binding.Status.Conditions, korifiv1alpha1.StatusConditionReady)
	}))
}

func bindingObjectRefs(bindings []korifiv1alpha1.CFServiceBinding) []korifiv1alpha1.ServiceBinding {
	return slices.Collect(it.Map(slices.Values(bindings), func(binding korifiv1alpha1.CFServiceBinding) korifiv1alpha1.ServiceBinding {
		bindingName := binding.Status.MountSecretRef.Name
		if binding.Spec.DisplayName != nil {
			bindingName = *binding.Spec.DisplayName
		}

		return korifiv1alpha1.ServiceBinding{
			GUID:   binding.Name,
			Name:   bindingName,
			Secret: binding.Status.MountSecretRef.Name,
		}
	}))
}

func getActualState(processes []*korifiv1alpha1.CFProcess) korifiv1alpha1.AppState {
	processInstances := int32(0)
	for _, p := range processes {
		processInstances += p.Status.ActualInstances
	}

	if processInstances == 0 {
		return korifiv1alpha1.StoppedState
	}
	return korifiv1alpha1.StartedState
}

func (r *Reconciler) getDroplet(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (*korifiv1alpha1.BuildDropletStatus, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("getDroplet").WithValues("dropletName", cfApp.Spec.CurrentDropletRef.Name)

	var cfBuild korifiv1alpha1.CFBuild
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfApp.Spec.CurrentDropletRef.Name, Namespace: cfApp.Namespace}, &cfBuild)
	if err != nil {
		log.Info("error when fetching CFBuild", "reason", err)
		return nil, err
	}

	if cfBuild.Status.Droplet == nil {
		err = errors.New("status field CFBuildDropletStatus is nil on CFBuild")
		log.Info(err.Error())
		return nil, err
	}

	return cfBuild.Status.Droplet, nil
}

func (r *Reconciler) reconcileProcesses(ctx context.Context, cfApp *korifiv1alpha1.CFApp, droplet *korifiv1alpha1.BuildDropletStatus) ([]*korifiv1alpha1.CFProcess, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("startApp")

	reconciledProcess := []*korifiv1alpha1.CFProcess{}

	for _, dropletProcess := range addWebIfMissing(droplet.ProcessTypes) {
		loopLog := log.WithValues("processType", dropletProcess.Type)
		ctx = logr.NewContext(ctx, loopLog)

		existingProcess, err := r.fetchProcessByType(ctx, cfApp.Name, cfApp.Namespace, dropletProcess.Type)
		if err != nil {
			loopLog.Info("error when fetching CFProcess by type", "reason", err)
			return nil, err
		}

		if existingProcess != nil {
			err = r.updateCFProcess(ctx, existingProcess, dropletProcess.Command, cfApp.Status.ServiceBindings)
			if err != nil {
				loopLog.Info("error updating CFProcess", "reason", err)
				return nil, err
			}
			reconciledProcess = append(reconciledProcess, existingProcess)
		} else {
			createdProcess, err := r.createCFProcess(ctx, dropletProcess, cfApp)
			if err != nil {
				loopLog.Info("error creating CFProcess", "reason", err)
				return nil, err
			}
			reconciledProcess = append(reconciledProcess, createdProcess)
		}
	}

	return reconciledProcess, nil
}

func addWebIfMissing(processTypes []korifiv1alpha1.ProcessType) []korifiv1alpha1.ProcessType {
	for _, p := range processTypes {
		if p.Type == korifiv1alpha1.ProcessTypeWeb {
			return processTypes
		}
	}

	return append([]korifiv1alpha1.ProcessType{{Type: korifiv1alpha1.ProcessTypeWeb}}, processTypes...)
}

func (r *Reconciler) updateCFProcess(ctx context.Context, process *korifiv1alpha1.CFProcess, command string, bindings []korifiv1alpha1.ServiceBinding) error {
	return k8s.Patch(ctx, r.k8sClient, process, func() {
		process.Spec.DetectedCommand = command
	})
}

func (r *Reconciler) createCFProcess(ctx context.Context, process korifiv1alpha1.ProcessType, cfApp *korifiv1alpha1.CFApp) (*korifiv1alpha1.CFProcess, error) {
	desiredCFProcess := &korifiv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfApp.Namespace,
			Name:      tools.NamespacedUUID(cfApp.Name, process.Type),
			Labels: map[string]string{
				korifiv1alpha1.CFAppGUIDLabelKey:     cfApp.Name,
				korifiv1alpha1.CFProcessTypeLabelKey: process.Type,
			},
		},
		Spec: korifiv1alpha1.CFProcessSpec{
			AppRef:          corev1.LocalObjectReference{Name: cfApp.Name},
			ProcessType:     process.Type,
			DetectedCommand: process.Command,
		},
	}

	if err := controllerutil.SetControllerReference(cfApp, desiredCFProcess, r.scheme); err != nil {
		err = fmt.Errorf("failed to set OwnerRef on CFProcess: %w", err)
		return nil, err
	}

	err := r.k8sClient.Create(ctx, desiredCFProcess)
	if err != nil {
		return nil, err
	}

	return desiredCFProcess, nil
}

func (r *Reconciler) fetchProcessByType(ctx context.Context, appGUID, appNamespace, processType string) (*korifiv1alpha1.CFProcess, error) {
	selector, err := labels.ValidatedSelectorFromSet(map[string]string{
		korifiv1alpha1.CFAppGUIDLabelKey:     appGUID,
		korifiv1alpha1.CFProcessTypeLabelKey: processType,
	})
	if err != nil {
		err = fmt.Errorf("error initializing label selector: %w", err)
		return nil, err
	}

	cfProcessList := korifiv1alpha1.CFProcessList{}
	err = r.k8sClient.List(ctx, &cfProcessList, &client.ListOptions{LabelSelector: selector, Namespace: appNamespace})
	if err != nil {
		err = fmt.Errorf("error listing app CFProcesses: %w", err)
		return nil, err
	}

	if len(cfProcessList.Items) == 0 {
		return nil, nil
	}

	return &cfProcessList.Items[0], nil
}

func (r *Reconciler) finalizeCFApp(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalizeCFApp")

	if !controllerutil.ContainsFinalizer(cfApp, korifiv1alpha1.CFAppFinalizerName) {
		return ctrl.Result{}, nil
	}

	err := r.finalizeCFAppRoutes(ctx, cfApp)
	if err != nil {
		return ctrl.Result{}, err
	}

	sbFinalizationResult, err := r.finalizeCFServiceBindings(ctx, cfApp)
	if err != nil {
		return ctrl.Result{}, err
	}

	if (sbFinalizationResult != ctrl.Result{}) {
		return sbFinalizationResult, nil
	}

	if controllerutil.RemoveFinalizer(cfApp, korifiv1alpha1.CFAppFinalizerName) {
		log.V(1).Info("finalizer removed")
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) finalizeCFAppRoutes(ctx context.Context, cfApp *korifiv1alpha1.CFApp) error {
	cfRoutes, err := r.getCFRoutes(ctx, cfApp.Name, cfApp.Namespace)
	if err != nil {
		return err
	}

	err = r.updateRouteDestinations(ctx, cfApp.Name, cfRoutes)
	if err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) finalizeCFServiceBindings(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalizeCFServiceBindings")

	sbList := korifiv1alpha1.CFServiceBindingList{}
	err := r.k8sClient.List(ctx, &sbList, client.InNamespace(cfApp.Namespace), client.MatchingFields{shared.IndexServiceBindingAppGUID: cfApp.Name})
	if err != nil {
		log.Info("failed to list app service bindings", "reason", err)
		return ctrl.Result{}, err
	}

	if len(sbList.Items) == 0 {
		return ctrl.Result{}, nil
	}

	for i := range sbList.Items {
		err = r.k8sClient.Delete(ctx, &sbList.Items[i])
		if err != nil {
			log.Info("failed to delete service binding", "serviceBindingName", sbList.Items[i].Name, "reason", err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: time.Second}, nil
}

func (r *Reconciler) updateRouteDestinations(ctx context.Context, cfAppGUID string, cfRoutes []korifiv1alpha1.CFRoute) error {
	log := logr.FromContextOrDiscard(ctx).WithName("updateRouteDestinations")

	for i := range cfRoutes {
		loopLog := log.WithValues("routeName", cfRoutes[i].Name)

		var updatedDestinations []korifiv1alpha1.Destination
		if cfRoutes[i].Spec.Destinations != nil {
			for _, destination := range cfRoutes[i].Spec.Destinations {
				if destination.AppRef.Name != cfAppGUID {
					updatedDestinations = append(updatedDestinations, destination)
				} else {
					loopLog.V(1).Info("removing app destinations from cfroute")
				}
			}
		}

		err := k8s.Patch(ctx, r.k8sClient, &cfRoutes[i], func() {
			cfRoutes[i].Spec.Destinations = updatedDestinations
		})
		if err != nil {
			loopLog.Info("failed to patch cfRoute to update destinations", "reason", err)
			return err
		}
	}
	return nil
}

func (r *Reconciler) getCFRoutes(ctx context.Context, cfAppGUID string, cfAppNamespace string) ([]korifiv1alpha1.CFRoute, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("getCFRoutes")

	var foundRoutes korifiv1alpha1.CFRouteList
	matchingFields := client.MatchingFields{shared.IndexRouteDestinationAppName: cfAppGUID}
	err := r.k8sClient.List(context.Background(), &foundRoutes, client.InNamespace(cfAppNamespace), matchingFields)
	if err != nil {
		log.Info("failed to List CFRoutes", "reason", err)
		return []korifiv1alpha1.CFRoute{}, err
	}

	return foundRoutes.Items, nil
}

func (r *Reconciler) reconcileVCAPSecret(
	ctx context.Context,
	cfApp *korifiv1alpha1.CFApp,
	secretName string,
	envBuilder EnvValueBuilder,
) error {
	log := logr.FromContextOrDiscard(ctx).WithName("reconcileVCAPSecret").WithValues("secretName", secretName)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cfApp.Namespace,
		},
	}

	envValue, err := envBuilder.BuildEnvValue(ctx, cfApp)
	if err != nil {
		log.Info("failed to build env value", "reason", err)
		return err
	}

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, secret, func() error {
		secret.Data = envValue

		return controllerutil.SetControllerReference(cfApp, secret, r.scheme)
	})
	if err != nil {
		log.Info("unable to create or patch Secret", "reason", err)
		return err
	}

	return nil
}
