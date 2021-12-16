package workloads

import (
	"context"
	"errors"
	"fmt"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/config"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/shared"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	StatusConditionRestarting = "Restarting"
	StatusConditionRunning    = "Running"
	processHealthCheckType    = "process"
	processTypeWeb            = "web"
	finalizerName             = "cfApp.workloads.cloudfoundry.org"
)

// CFAppReconciler reconciles a CFApp object
type CFAppReconciler struct {
	Client           CFClient
	Scheme           *runtime.Scheme
	Log              logr.Logger
	ControllerConfig *config.ControllerConfig
}

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfapps/finalizers,verbs=update

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
	cfApp := &workloadsv1alpha1.CFApp{}
	err := r.Client.Get(ctx, req.NamespacedName, cfApp)
	if err != nil {
		r.Log.Error(err, "unable to fetch CFApp")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	err = r.addFinalizer(ctx, cfApp)
	if err != nil {
		r.Log.Error(err, "Error adding finalizer for cfApp")
		return ctrl.Result{}, err
	}

	if isFinalizing(cfApp) {
		return r.finalizeCFApp(ctx, cfApp)
	}

	// Create CFProcesses if current droplet reference is not empty.
	if cfApp.Spec.CurrentDropletRef.Name != "" {
		var cfBuild workloadsv1alpha1.CFBuild
		err = r.Client.Get(ctx, types.NamespacedName{Name: cfApp.Spec.CurrentDropletRef.Name, Namespace: cfApp.Namespace}, &cfBuild)
		if err != nil {
			r.Log.Error(err, "Error when fetching CFBuild")
			return ctrl.Result{}, err
		}

		// CFBuildDropletStatus is nil when build has not completed staging or that it has failed.
		// In such cases return error.
		if cfBuild.Status.BuildDropletStatus == nil {
			err = errors.New("status field CFBuildDropletStatus is nil on CFBuild")
			r.Log.Error(err, "CFBuildDropletStatus is nil on CFBuild.Status, check if referenced Build/Droplet was successfully staged")
			return ctrl.Result{}, err
		}

		droplet := cfBuild.Status.BuildDropletStatus

		// Iterate over the processTypes array on the droplet
		for _, process := range droplet.ProcessTypes {
			// Check if CFProcess exists for a given process type
			var processExistsForType bool
			processExistsForType, err = r.checkCFProcessExistsForType(ctx, cfApp.Name, cfApp.Namespace, process.Type)
			if err != nil {
				r.Log.Error(err, "Error when checking if CFProcess exists")
				return ctrl.Result{}, err
			}

			// Only if CFProcess does no exist for a given process type, invoke create
			if !processExistsForType {
				err = r.createCFProcess(ctx, process, droplet.Ports, cfApp)
				if err != nil {
					r.Log.Error(err, fmt.Sprintf("Error creating CFProcess for Type: %s", process.Type))
					return ctrl.Result{}, err
				}
			}
		}
	}

	// set the status.conditions "Running" to false
	meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
		Type:    StatusConditionRunning,
		Status:  metav1.ConditionFalse,
		Reason:  "unimplemented",
		Message: "",
	})
	cfApp.Status.ObservedDesiredState = cfApp.Spec.DesiredState

	// Update CF App Status Conditions based on local copy
	if statusErr := r.Client.Status().Update(ctx, cfApp); statusErr != nil {
		r.Log.Error(statusErr, "unable to update CFApp status")
		r.Log.Info(fmt.Sprintf("CFApps status: %+v", cfApp.Status))
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, nil
}

func (r *CFAppReconciler) createCFProcess(ctx context.Context, process workloadsv1alpha1.ProcessType, ports []int32, cfApp *workloadsv1alpha1.CFApp) error {
	cfProcessGUID := generateGUID()

	desiredCFProcess := workloadsv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfProcessGUID,
			Namespace: cfApp.Namespace,
			Labels: map[string]string{
				workloadsv1alpha1.CFAppGUIDLabelKey:     cfApp.Name,
				workloadsv1alpha1.CFProcessGUIDLabelKey: cfProcessGUID,
				workloadsv1alpha1.CFProcessTypeLabelKey: process.Type,
			},
		},
		Spec: workloadsv1alpha1.CFProcessSpec{
			AppRef:      corev1.LocalObjectReference{Name: cfApp.Name},
			ProcessType: process.Type,
			Command:     process.Command,
			HealthCheck: workloadsv1alpha1.HealthCheck{
				Type: processHealthCheckType,
				Data: workloadsv1alpha1.HealthCheckData{
					InvocationTimeoutSeconds: 0,
					TimeoutSeconds:           0,
				},
			},
			DesiredInstances: getDesiredInstanceCount(process.Type),
			MemoryMB:         r.ControllerConfig.CFProcessDefaults.MemoryMB,
			DiskQuotaMB:      r.ControllerConfig.CFProcessDefaults.DefaultDiskQuotaMB,
			Ports:            ports,
		},
	}

	err := controllerutil.SetOwnerReference(cfApp, &desiredCFProcess, r.Scheme)
	if err != nil {
		r.Log.Error(err, "failed to set OwnerRef on CFProcess")
		return err
	}

	return r.Client.Create(ctx, &desiredCFProcess)
}

func (r *CFAppReconciler) checkCFProcessExistsForType(ctx context.Context, appGUID string, namespace string, processType string) (bool, error) {
	selector, err := labels.ValidatedSelectorFromSet(map[string]string{
		workloadsv1alpha1.CFAppGUIDLabelKey:     appGUID,
		workloadsv1alpha1.CFProcessTypeLabelKey: processType,
	})
	if err != nil {
		r.Log.Error(err, "Error initializing label selector")
		return false, err
	}

	cfProcessList := workloadsv1alpha1.CFProcessList{}
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

func (r *CFAppReconciler) addFinalizer(ctx context.Context, cfApp *workloadsv1alpha1.CFApp) error {
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

func isFinalizing(cfApp *workloadsv1alpha1.CFApp) bool {
	return cfApp.ObjectMeta.DeletionTimestamp != nil && !cfApp.ObjectMeta.DeletionTimestamp.IsZero()
}

func (r *CFAppReconciler) finalizeCFApp(ctx context.Context, cfApp *workloadsv1alpha1.CFApp) (ctrl.Result, error) {
	r.Log.Info(fmt.Sprintf("Reconciling deletion of CFApp/%s", cfApp.Name))

	if !controllerutil.ContainsFinalizer(cfApp, finalizerName) {
		return ctrl.Result{}, nil
	}

	cfRoutes, err := r.getCFRoutes(ctx, cfApp.Name, cfApp.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.removeRouteDestinations(ctx, cfApp.Name, cfRoutes)
	if err != nil {
		return ctrl.Result{}, err
	}

	originalCFApp := cfApp.DeepCopy()
	controllerutil.RemoveFinalizer(cfApp, finalizerName)

	if err = r.Client.Patch(ctx, cfApp, client.MergeFrom(originalCFApp)); err != nil {
		r.Log.Error(err, "Failed to remove finalizer on cfApp")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFAppReconciler) removeRouteDestinations(ctx context.Context, cfAppGUID string, cfRoutes []networkingv1alpha1.CFRoute) error {
	var updatedDestinations []networkingv1alpha1.Destination
	for _, cfRoute := range cfRoutes {
		originalCFRoute := cfRoute.DeepCopy()
		if cfRoute.Spec.Destinations != nil {
			for _, destination := range cfRoute.Spec.Destinations {
				if destination.AppRef.Name != cfAppGUID {
					updatedDestinations = append(updatedDestinations, destination)
				} else {
					r.Log.Info(fmt.Sprintf("Removing destination for cfapp %s from cfroute %s", cfAppGUID, cfRoute.Name))
				}
			}
		}
		cfRoute.Spec.Destinations = updatedDestinations
		err := r.Client.Patch(ctx, &cfRoute, client.MergeFrom(originalCFRoute))
		if err != nil {
			r.Log.Error(err, "failed to patch cfRoute to remove a destination")
			return err
		}
	}
	return nil
}

func (r *CFAppReconciler) getCFRoutes(ctx context.Context, cfAppGUID string, cfAppNamespace string) ([]networkingv1alpha1.CFRoute, error) {
	var foundRoutes networkingv1alpha1.CFRouteList
	matchingFields := client.MatchingFields{DestinationAppName: cfAppGUID}
	err := r.Client.List(context.Background(), &foundRoutes, client.InNamespace(cfAppNamespace), matchingFields)
	if err != nil {
		return []networkingv1alpha1.CFRoute{}, err
	}

	return foundRoutes.Items, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadsv1alpha1.CFApp{}).
		Complete(r)
}
