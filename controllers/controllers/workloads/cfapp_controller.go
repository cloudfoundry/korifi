package workloads

import (
	"context"
	"errors"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools/k8s"

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
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	cfAppFinalizerName = "cfApp.korifi.cloudfoundry.org"
)

type EnvValueBuilder interface {
	BuildEnvValue(context.Context, *korifiv1alpha1.CFApp) (map[string]string, error)
}

// CFAppReconciler reconciles a CFApp object
type CFAppReconciler struct {
	log                       logr.Logger
	k8sClient                 client.Client
	scheme                    *runtime.Scheme
	vcapServicesEnvBuilder    EnvValueBuilder
	vcapApplicationEnvBuilder EnvValueBuilder
}

func NewCFAppReconciler(k8sClient client.Client, scheme *runtime.Scheme, log logr.Logger, vcapServicesBuilder, vcapApplicationBuilder EnvValueBuilder) *k8s.PatchingReconciler[korifiv1alpha1.CFApp, *korifiv1alpha1.CFApp] {
	appReconciler := CFAppReconciler{
		log:                       log,
		k8sClient:                 k8sClient,
		scheme:                    scheme,
		vcapServicesEnvBuilder:    vcapServicesBuilder,
		vcapApplicationEnvBuilder: vcapApplicationBuilder,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.CFApp, *korifiv1alpha1.CFApp](log, k8sClient, &appReconciler)
}

func (r *CFAppReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFApp{}).
		Watches(
			&source.Kind{Type: &korifiv1alpha1.CFBuild{}},
			handler.EnqueueRequestsFromMapFunc(buildToApp),
		).
		Watches(
			&source.Kind{Type: &korifiv1alpha1.CFServiceBinding{}},
			handler.EnqueueRequestsFromMapFunc(serviceBindingToApp),
		)
}

func buildToApp(o client.Object) []reconcile.Request {
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

func serviceBindingToApp(o client.Object) []reconcile.Request {
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

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfapps/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;patch

func (r *CFAppReconciler) ReconcileResource(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (ctrl.Result, error) {
	log := r.log.WithValues("namespace", cfApp.Namespace, "name", cfApp.Name)

	cfApp.Status.ObservedGeneration = cfApp.Generation
	log.V(1).Info("set observed generation", "generation", cfApp.Status.ObservedGeneration)

	if !cfApp.GetDeletionTimestamp().IsZero() {
		err := r.finalizeCFApp(ctx, log, cfApp)
		return ctrl.Result{}, err
	}

	err := k8s.AddFinalizer(ctx, log, r.k8sClient, cfApp, cfAppFinalizerName)
	if err != nil {
		log.Info("error adding finalizer", "reason", err)
		return ctrl.Result{}, err
	}

	secretName := cfApp.Name + "-vcap-application"
	err = r.reconcileVCAPSecret(ctx, log, cfApp, secretName, r.vcapApplicationEnvBuilder)
	if err != nil {
		log.Info("unable to create CFApp VCAP Application secret", "reason", err)
		return ctrl.Result{}, err
	}
	cfApp.Status.VCAPApplicationSecretName = secretName

	secretName = cfApp.Name + "-vcap-services"
	err = r.reconcileVCAPSecret(ctx, log, cfApp, secretName, r.vcapServicesEnvBuilder)
	if err != nil {
		log.Info("unable to create CFApp VCAP Services secret", "reason", err)
		return ctrl.Result{}, err
	}

	cfApp.Status.VCAPServicesSecretName = secretName

	if cfApp.Status.Conditions == nil {
		cfApp.Status.Conditions = make([]metav1.Condition, 0)
	}

	if cfApp.Spec.CurrentDropletRef.Name == "" {
		meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
			Type:               shared.StatusConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "DropletNotAssigned",
			ObservedGeneration: cfApp.Generation,
		})

		return ctrl.Result{}, nil
	}

	droplet, err := r.getDroplet(ctx, log, cfApp)
	if err != nil {
		meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
			Type:               shared.StatusConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "CannotResolveCurrentDropletRef",
			ObservedGeneration: cfApp.Generation,
		})

		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
		Type:               shared.StatusConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "DropletAssigned",
		ObservedGeneration: cfApp.Generation,
	})

	err = r.startApp(ctx, log, cfApp, droplet)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFAppReconciler) getDroplet(ctx context.Context, log logr.Logger, cfApp *korifiv1alpha1.CFApp) (*korifiv1alpha1.BuildDropletStatus, error) {
	log = log.WithName("getDroplet").WithValues("dropletName", cfApp.Spec.CurrentDropletRef.Name)

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

func (r *CFAppReconciler) startApp(ctx context.Context, log logr.Logger, cfApp *korifiv1alpha1.CFApp, droplet *korifiv1alpha1.BuildDropletStatus) error {
	log = log.WithName("startApp")

	for _, dropletProcess := range addWebIfMissing(droplet.ProcessTypes) {
		loopLog := log.WithValues("processType", dropletProcess.Type)

		existingProcess, err := r.fetchProcessByType(ctx, loopLog, cfApp.Name, cfApp.Namespace, dropletProcess.Type)
		if err != nil {
			loopLog.Info("error when fetching CFProcess by type", "reason", err)
			return err
		}

		if existingProcess != nil {
			err = r.updateCFProcessCommand(ctx, existingProcess, dropletProcess.Command)
			if err != nil {
				loopLog.Info("error updating CFProcess", "reason", err)
				return err
			}
		} else {
			err = r.createCFProcess(ctx, loopLog, dropletProcess, droplet.Ports, cfApp)
			if err != nil {
				loopLog.Info("error creating CFProcess", "reason", err)
				return err
			}
		}
	}

	return nil
}

func addWebIfMissing(processTypes []korifiv1alpha1.ProcessType) []korifiv1alpha1.ProcessType {
	for _, p := range processTypes {
		if p.Type == korifiv1alpha1.ProcessTypeWeb {
			return processTypes
		}
	}
	return append([]korifiv1alpha1.ProcessType{{Type: korifiv1alpha1.ProcessTypeWeb}}, processTypes...)
}

func (r *CFAppReconciler) updateCFProcessCommand(ctx context.Context, process *korifiv1alpha1.CFProcess, command string) error {
	return k8s.Patch(ctx, r.k8sClient, process, func() {
		process.Spec.DetectedCommand = command
	})
}

func (r *CFAppReconciler) createCFProcess(ctx context.Context, log logr.Logger, process korifiv1alpha1.ProcessType, ports []int32, cfApp *korifiv1alpha1.CFApp) error {
	desiredCFProcess := &korifiv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfApp.Namespace,
			Labels: map[string]string{
				korifiv1alpha1.CFAppGUIDLabelKey:     cfApp.Name,
				korifiv1alpha1.CFProcessTypeLabelKey: process.Type,
			},
		},
		Spec: korifiv1alpha1.CFProcessSpec{
			AppRef:          corev1.LocalObjectReference{Name: cfApp.Name},
			ProcessType:     process.Type,
			DetectedCommand: process.Command,
			Ports:           ports,
		},
	}
	desiredCFProcess.SetStableName(cfApp.Name)

	if err := controllerutil.SetControllerReference(cfApp, desiredCFProcess, r.scheme); err != nil {
		err = fmt.Errorf("failed to set OwnerRef on CFProcess: %w", err)
		return err
	}

	return r.k8sClient.Create(ctx, desiredCFProcess)
}

func (r *CFAppReconciler) fetchProcessByType(ctx context.Context, log logr.Logger, appGUID, appNamespace, processType string) (*korifiv1alpha1.CFProcess, error) {
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

func (r *CFAppReconciler) finalizeCFApp(ctx context.Context, log logr.Logger, cfApp *korifiv1alpha1.CFApp) error {
	log = log.WithName("finalize")

	if !controllerutil.ContainsFinalizer(cfApp, cfAppFinalizerName) {
		return nil
	}

	err := r.finalizeCFAppRoutes(ctx, log, cfApp)
	if err != nil {
		return err
	}

	err = r.finalizeCFServiceBindings(ctx, log, cfApp)
	if err != nil {
		return err
	}

	if controllerutil.RemoveFinalizer(cfApp, cfAppFinalizerName) {
		log.V(1).Info("finalizer removed")
	}

	return nil
}

func (r *CFAppReconciler) finalizeCFAppRoutes(ctx context.Context, log logr.Logger, cfApp *korifiv1alpha1.CFApp) error {
	cfRoutes, err := r.getCFRoutes(ctx, log, cfApp.Name, cfApp.Namespace)
	if err != nil {
		return err
	}

	err = r.updateRouteDestinations(ctx, log, cfApp.Name, cfRoutes)
	if err != nil {
		return err
	}

	return nil
}

func (r *CFAppReconciler) finalizeCFServiceBindings(ctx context.Context, log logr.Logger, cfApp *korifiv1alpha1.CFApp) error {
	sbList := korifiv1alpha1.CFServiceBindingList{}
	err := r.k8sClient.List(ctx, &sbList, client.InNamespace(cfApp.Namespace), client.MatchingFields{shared.IndexServiceBindingAppGUID: cfApp.Name})
	if err != nil {
		log.Info("failed to list app service bindings", "reason", err)
		return err
	}

	for i := range sbList.Items {
		err = r.k8sClient.Delete(ctx, &sbList.Items[i])
		if err != nil {
			log.Info("failed to delete service binding", "serviceBindingName", sbList.Items[i].Name, "reason", err)
			return err
		}
	}

	return nil
}

func (r *CFAppReconciler) updateRouteDestinations(ctx context.Context, log logr.Logger, cfAppGUID string, cfRoutes []korifiv1alpha1.CFRoute) error {
	log = log.WithName("updateRouteDestinations")

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

func (r *CFAppReconciler) getCFRoutes(ctx context.Context, log logr.Logger, cfAppGUID string, cfAppNamespace string) ([]korifiv1alpha1.CFRoute, error) {
	var foundRoutes korifiv1alpha1.CFRouteList
	matchingFields := client.MatchingFields{shared.IndexRouteDestinationAppName: cfAppGUID}
	err := r.k8sClient.List(context.Background(), &foundRoutes, client.InNamespace(cfAppNamespace), matchingFields)
	if err != nil {
		log.Info("failed to List CFRoutes", "reason", err)
		return []korifiv1alpha1.CFRoute{}, err
	}

	return foundRoutes.Items, nil
}

func (r *CFAppReconciler) reconcileVCAPSecret(
	ctx context.Context,
	log logr.Logger,
	cfApp *korifiv1alpha1.CFApp,
	secretName string,
	envBuilder EnvValueBuilder,
) error {
	log = log.WithName("reconcileVCAPSecret").WithValues("secretName", secretName)

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
		secret.StringData = envValue

		return controllerutil.SetOwnerReference(cfApp, secret, r.scheme)
	})
	if err != nil {
		log.Info("unable to create or patch Secret", "reason", err)
		return err
	}

	return nil
}
