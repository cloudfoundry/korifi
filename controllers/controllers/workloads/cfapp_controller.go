package workloads

import (
	"context"
	"errors"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	. "code.cloudfoundry.org/korifi/controllers/controllers/shared"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	StatusConditionRestarting = "Restarting"
	StatusConditionRunning    = "Running"
	StatusConditionStaged     = "Staged"
	processHealthCheckType    = "process"
	portHealthCheckType       = "port"
	processTypeWeb            = "web"
	finalizerName             = "cfApp.korifi.cloudfoundry.org"
)

// CFAppReconciler reconciles a CFApp object
type CFAppReconciler struct {
	Client           CFClient
	Scheme           *runtime.Scheme
	Log              logr.Logger
	ControllerConfig *config.ControllerConfig
}

func NewCFAppReconciler(client CFClient, scheme *runtime.Scheme, log logr.Logger, controllerConfig *config.ControllerConfig) *CFAppReconciler {
	return &CFAppReconciler{Client: client, Scheme: scheme, Log: log, ControllerConfig: controllerConfig}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfapps/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CFApp object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *CFAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cfApp := &korifiv1alpha1.CFApp{}
	err := r.Client.Get(ctx, req.NamespacedName, cfApp)
	if err != nil {
		r.Log.Error(err, "unable to fetch CFApp")
		// ignore not-found errors, since they can't be fixed by an immediate requeue
		// (we'll need to wait for a new notification), and we can get them on deleted requests
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	err = r.addFinalizer(ctx, cfApp)
	if err != nil {
		r.Log.Error(err, "Error adding finalizer for cfApp")
		return ctrl.Result{}, err
	}

	if !cfApp.GetDeletionTimestamp().IsZero() {
		return r.finalizeCFApp(ctx, cfApp)
	}

	err = r.createVCAPServicesSecretForApp(ctx, cfApp)
	if err != nil {
		r.Log.Error(err, "unable to create CFApp VCAP Services secret")
		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
		Type:    StatusConditionStaged,
		Status:  metav1.ConditionFalse,
		Reason:  "appStaged",
		Message: "",
	})

	meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
		Type:    StatusConditionRunning,
		Status:  metav1.ConditionFalse,
		Reason:  "unimplemented",
		Message: "",
	})

	if cfApp.Spec.CurrentDropletRef.Name == "" {
		return r.updateStatusAndReturn(ctx, cfApp, nil)
	}

	droplet, err := r.getDroplet(ctx, cfApp)
	if err != nil {
		return r.updateStatusAndReturn(ctx, cfApp, err)
	}

	meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
		Type:    StatusConditionStaged,
		Status:  metav1.ConditionTrue,
		Reason:  "appStaged",
		Message: "",
	})

	err = r.startApp(ctx, cfApp, droplet)
	if err != nil {
		return r.updateStatusAndReturn(ctx, cfApp, err)
	}

	return r.updateStatusAndReturn(ctx, cfApp, nil)
}

func (r *CFAppReconciler) getDroplet(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (*korifiv1alpha1.BuildDropletStatus, error) {
	var cfBuild korifiv1alpha1.CFBuild
	err := r.Client.Get(ctx, types.NamespacedName{Name: cfApp.Spec.CurrentDropletRef.Name, Namespace: cfApp.Namespace}, &cfBuild)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFBuild")
		return nil, err
	}

	if cfBuild.Status.Droplet == nil {
		err = errors.New("status field CFBuildDropletStatus is nil on CFBuild")
		r.Log.Error(err, "CFBuildDropletStatus is nil on CFBuild.Status, check if referenced Build/Droplet was successfully staged")
		return nil, err
	}

	return cfBuild.Status.Droplet, nil
}

func (r *CFAppReconciler) startApp(ctx context.Context, cfApp *korifiv1alpha1.CFApp, droplet *korifiv1alpha1.BuildDropletStatus) error {
	for _, process := range addWebIfMissing(droplet.ProcessTypes) {
		processExistsForType, err := r.checkCFProcessExistsForType(ctx, cfApp.Name, cfApp.Namespace, process.Type)
		if err != nil {
			r.Log.Error(err, "Error when checking if CFProcess exists")
			return err
		}

		if !processExistsForType {
			err = r.createCFProcess(ctx, process, droplet.Ports, cfApp)
			if err != nil {
				r.Log.Error(err, fmt.Sprintf("Error creating CFProcess for Type: %s", process.Type))
				return err
			}
		}
	}

	return nil
}

func addWebIfMissing(processTypes []korifiv1alpha1.ProcessType) []korifiv1alpha1.ProcessType {
	for _, p := range processTypes {
		if p.Type == processTypeWeb {
			return processTypes
		}
	}
	return append([]korifiv1alpha1.ProcessType{{Type: processTypeWeb}}, processTypes...)
}

func (r *CFAppReconciler) createCFProcess(ctx context.Context, process korifiv1alpha1.ProcessType, ports []int32, cfApp *korifiv1alpha1.CFApp) error {
	healthCheckType, err := r.getHealthCheckType(ctx, process.Type, cfApp)
	if err != nil {
		return err
	}
	desiredCFProcess := &korifiv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfApp.Namespace,
			Labels: map[string]string{
				korifiv1alpha1.CFAppGUIDLabelKey:     cfApp.Name,
				korifiv1alpha1.CFProcessTypeLabelKey: process.Type,
			},
		},
		Spec: korifiv1alpha1.CFProcessSpec{
			AppRef:      corev1.LocalObjectReference{Name: cfApp.Name},
			ProcessType: process.Type,
			Command:     process.Command,
			HealthCheck: korifiv1alpha1.HealthCheck{
				Type: korifiv1alpha1.HealthCheckType(healthCheckType),
				Data: korifiv1alpha1.HealthCheckData{
					InvocationTimeoutSeconds: 0,
					TimeoutSeconds:           0,
				},
			},
			DesiredInstances: getDesiredInstanceCount(process.Type),
			MemoryMB:         r.ControllerConfig.CFProcessDefaults.MemoryMB,
			DiskQuotaMB:      r.ControllerConfig.CFProcessDefaults.DiskQuotaMB,
			Ports:            ports,
		},
	}
	desiredCFProcess.SetStableName(cfApp.Name)

	err = controllerutil.SetOwnerReference(cfApp, desiredCFProcess, r.Scheme)
	if err != nil {
		r.Log.Error(err, "failed to set OwnerRef on CFProcess")
		return err
	}

	return r.Client.Create(ctx, desiredCFProcess)
}

func (r *CFAppReconciler) getHealthCheckType(ctx context.Context, processType string, cfApp *korifiv1alpha1.CFApp) (string, error) {
	if processType == processTypeWeb {
		cfRoutes, err := r.getCFRoutes(ctx, cfApp.Name, cfApp.Namespace)
		if err != nil {
			return "", err
		}
		if len(cfRoutes) > 0 {
			return portHealthCheckType, nil
		}
	}

	return processHealthCheckType, nil
}

func (r *CFAppReconciler) checkCFProcessExistsForType(ctx context.Context, appGUID string, namespace string, processType string) (bool, error) {
	selector, err := labels.ValidatedSelectorFromSet(map[string]string{
		korifiv1alpha1.CFAppGUIDLabelKey:     appGUID,
		korifiv1alpha1.CFProcessTypeLabelKey: processType,
	})
	if err != nil {
		r.Log.Error(err, "Error initializing label selector")
		return false, err
	}

	cfProcessList := korifiv1alpha1.CFProcessList{}
	err = r.Client.List(ctx, &cfProcessList, &client.ListOptions{LabelSelector: selector, Namespace: namespace})
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error fetching CFProcess for Type: %s", processType))
		return false, err
	}

	return len(cfProcessList.Items) > 0, nil
}

func getDesiredInstanceCount(processType string) int {
	if processType == processTypeWeb {
		return 1
	}
	return 0
}

func (r *CFAppReconciler) addFinalizer(ctx context.Context, cfApp *korifiv1alpha1.CFApp) error {
	if controllerutil.ContainsFinalizer(cfApp, finalizerName) {
		return nil
	}

	originalCFApp := cfApp.DeepCopy()
	controllerutil.AddFinalizer(cfApp, finalizerName)

	err := r.Client.Patch(ctx, cfApp, client.MergeFrom(originalCFApp))
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error adding finalizer to CFApp/%s", cfApp.Name))
		return err
	}

	r.Log.Info(fmt.Sprintf("Finalizer added to CFApp/%s", cfApp.Name))
	return nil
}

func (r *CFAppReconciler) finalizeCFApp(ctx context.Context, cfApp *korifiv1alpha1.CFApp) (ctrl.Result, error) {
	r.Log.Info(fmt.Sprintf("Reconciling deletion of CFApp/%s", cfApp.Name))

	if !controllerutil.ContainsFinalizer(cfApp, finalizerName) {
		return ctrl.Result{}, nil
	}

	err := r.finalizeCFAppRoutes(ctx, cfApp)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.finalizeCFAppTasks(ctx, cfApp)
	if err != nil {
		return ctrl.Result{}, err
	}

	originalCFApp := cfApp.DeepCopy()
	controllerutil.RemoveFinalizer(cfApp, finalizerName)

	if err := r.Client.Patch(ctx, cfApp, client.MergeFrom(originalCFApp)); err != nil {
		r.Log.Error(err, "Failed to remove finalizer on cfApp")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFAppReconciler) finalizeCFAppRoutes(ctx context.Context, cfApp *korifiv1alpha1.CFApp) error {
	cfRoutes, err := r.getCFRoutes(ctx, cfApp.Name, cfApp.Namespace)
	if err != nil {
		return err
	}

	err = r.removeRouteDestinations(ctx, cfApp.Name, cfRoutes)
	if err != nil {
		return err
	}

	return nil
}

func (r *CFAppReconciler) finalizeCFAppTasks(ctx context.Context, cfApp *korifiv1alpha1.CFApp) error {
	tasksList := korifiv1alpha1.CFTaskList{}
	err := r.Client.List(ctx, &tasksList, client.InNamespace(cfApp.Namespace), client.MatchingFields{shared.IndexAppTasks: cfApp.Name})
	if err != nil {
		return err
	}

	for i := range tasksList.Items {
		err = r.Client.Delete(ctx, &tasksList.Items[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *CFAppReconciler) removeRouteDestinations(ctx context.Context, cfAppGUID string, cfRoutes []korifiv1alpha1.CFRoute) error {
	var updatedDestinations []korifiv1alpha1.Destination
	for i := range cfRoutes {
		originalCFRoute := cfRoutes[i].DeepCopy()
		if cfRoutes[i].Spec.Destinations != nil {
			for _, destination := range cfRoutes[i].Spec.Destinations {
				if destination.AppRef.Name != cfAppGUID {
					updatedDestinations = append(updatedDestinations, destination)
				} else {
					r.Log.Info(fmt.Sprintf("Removing destination for cfapp %s from cfroute %s", cfAppGUID, cfRoutes[i].Name))
				}
			}
		}
		cfRoutes[i].Spec.Destinations = updatedDestinations
		err := r.Client.Patch(ctx, &cfRoutes[i], client.MergeFrom(originalCFRoute))
		if err != nil {
			r.Log.Error(err, "failed to patch cfRoute to remove a destination")
			return err
		}
	}
	return nil
}

func (r *CFAppReconciler) getCFRoutes(ctx context.Context, cfAppGUID string, cfAppNamespace string) ([]korifiv1alpha1.CFRoute, error) {
	var foundRoutes korifiv1alpha1.CFRouteList
	matchingFields := client.MatchingFields{IndexRouteDestinationAppName: cfAppGUID}
	err := r.Client.List(context.Background(), &foundRoutes, client.InNamespace(cfAppNamespace), matchingFields)
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("failed to List CFRoutes for CFApp %s/%s", cfAppNamespace, cfAppGUID))
		return []korifiv1alpha1.CFRoute{}, err
	}

	return foundRoutes.Items, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFApp{}).
		Watches(&source.Kind{Type: &korifiv1alpha1.CFBuild{}}, handler.EnqueueRequestsFromMapFunc(buildToApp)).
		Complete(r)
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

func (r *CFAppReconciler) createVCAPServicesSecretForApp(ctx context.Context, cfApp *korifiv1alpha1.CFApp) error {
	if cfApp.Status.VCAPServicesSecretName != "" {
		return nil
	}

	vcapServicesSecretName := cfApp.Name + "-vcap-services"
	vcapServicesSecretLookupKey := types.NamespacedName{Name: vcapServicesSecretName, Namespace: cfApp.Namespace}
	vcapServicesSecret := new(corev1.Secret)
	err := r.Client.Get(ctx, vcapServicesSecretLookupKey, vcapServicesSecret)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			r.Log.Error(err, "unable to fetch 'VCAP_SERVICES' Secret")
			return err
		}

		vcapServicesSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vcapServicesSecretName,
				Namespace: cfApp.Namespace,
			},
			Immutable: nil,
			Data:      nil,
			StringData: map[string]string{
				"VCAP_SERVICES": "{}",
			},
			Type: "",
		}

		err = controllerutil.SetOwnerReference(cfApp, vcapServicesSecret, r.Scheme)
		if err != nil {
			r.Log.Error(err, "failed to set OwnerRef on 'VCAP_SERVICES' Secret")
			return err
		}

		err = r.Client.Create(ctx, vcapServicesSecret)
		if err != nil {
			r.Log.Error(err, "unable to create 'VCAP_SERVICES' Secret")
			return err
		}
	}
	originalCFApp := cfApp.DeepCopy()

	cfApp.Status.VCAPServicesSecretName = vcapServicesSecretName

	if cfApp.Status.ObservedDesiredState != cfApp.Spec.DesiredState {
		cfApp.Status.ObservedDesiredState = cfApp.Spec.DesiredState
	}
	if cfApp.Status.Conditions == nil {
		cfApp.Status.Conditions = make([]metav1.Condition, 0)
	}
	if statusErr := r.Client.Status().Patch(ctx, cfApp, client.MergeFrom(originalCFApp)); statusErr != nil {
		r.Log.Error(statusErr, "unable to patch CFApp status")
		r.Log.Info(fmt.Sprintf("CFApps status: %+v", cfApp.Status))
		return statusErr
	}
	return nil
}

func (r *CFAppReconciler) updateStatusAndReturn(ctx context.Context, cfApp *korifiv1alpha1.CFApp, err error) (ctrl.Result, error) {
	if statusErr := r.Client.Status().Update(ctx, cfApp); statusErr != nil {
		r.Log.Error(statusErr, "unable to update CFApp status")
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, err
}
