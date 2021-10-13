package workloads

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/labels"

	cfconfig "code.cloudfoundry.org/cf-k8s-controllers/config/cf"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
)

const (
	StatusConditionRestarting = "Restarting"
	StatusConditionRunning    = "Running"
	processHealthCheckType    = "process"
	processTypeWeb            = "web"
)

// CFAppReconciler reconciles a CFApp object
type CFAppReconciler struct {
	Client           CFClient
	Scheme           *runtime.Scheme
	Log              logr.Logger
	ControllerConfig *cfconfig.ControllerConfig
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
	var cfApp workloadsv1alpha1.CFApp
	err := r.Client.Get(ctx, req.NamespacedName, &cfApp)
	if err != nil {
		r.Log.Error(err, "unable to fetch CFApp")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	//Create CFProcesses if current droplet reference is not empty.
	if cfApp.Spec.CurrentDropletRef.Name != "" {
		var cfBuild workloadsv1alpha1.CFBuild
		err = r.Client.Get(ctx, types.NamespacedName{Name: cfApp.Spec.CurrentDropletRef.Name, Namespace: cfApp.Namespace}, &cfBuild)
		if err != nil {
			r.Log.Error(err, "Error when fetching CFBuild")
			return ctrl.Result{}, err
		}

		//If CFBuildStatusDroplet is nil return error
		if cfBuild.Status.BuildDropletStatus == nil {
			err = errors.New("CFBuildDropletStatus is nil")
			r.Log.Error(err, fmt.Sprintf("CFBuildDropletStatus is nil on the build %s", cfBuild.Name))
			return ctrl.Result{}, err
		}

		droplet := cfBuild.Status.BuildDropletStatus

		//Iterate over the processTypes array on the droplet
		for _, process := range droplet.ProcessTypes {
			//Check if CFProcess exists for a given process type
			var processExistsForType bool
			processExistsForType, err = r.checkCFProcessExistsForType(ctx, cfApp.Name, cfApp.Namespace, process.Type)
			if err != nil {
				r.Log.Error(err, "Error when checking if CFProcess exists")
				return ctrl.Result{}, err
			}

			//Only if CFProcess does no exist for a given process type, invoke create
			if !processExistsForType {
				err = r.createCFProcess(ctx, process, droplet.Ports, cfApp.Name, cfApp.Namespace)
				if err != nil {
					r.Log.Error(err, fmt.Sprintf("Error creating CFProcess for Type: %s", process.Type))
					return ctrl.Result{}, err
				}
			}
		}
	}

	// set the status.conditions "Running" and "Restarting" to false
	meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
		Type:    StatusConditionRunning,
		Status:  metav1.ConditionFalse,
		Reason:  "unimplemented",
		Message: "",
	})
	meta.SetStatusCondition(&cfApp.Status.Conditions, metav1.Condition{
		Type:    StatusConditionRestarting,
		Status:  metav1.ConditionFalse,
		Reason:  "unimplemented",
		Message: "",
	})

	// Update CF App Status Conditions based on local copy
	if err := r.Client.Status().Update(ctx, &cfApp); err != nil {
		r.Log.Error(err, "unable to update CFApp status")
		r.Log.Info(fmt.Sprintf("CFApps status: %+v", cfApp.Status))
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *CFAppReconciler) createCFProcess(ctx context.Context, process workloadsv1alpha1.ProcessType, ports []int32, cfAppGUID string, namespace string) error {
	cfProcessGUID := generateGUID()
	desiredCFProcess := workloadsv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfProcessGUID,
			Namespace: namespace,
			Labels: map[string]string{
				workloadsv1alpha1.CFAppGUIDLabelKey:     cfAppGUID,
				workloadsv1alpha1.CFProcessGUIDLabelKey: cfProcessGUID,
				workloadsv1alpha1.CFProcessTypeLabelKey: process.Type,
			},
		},
		Spec: workloadsv1alpha1.CFProcessSpec{
			AppRef:      corev1.LocalObjectReference{Name: cfAppGUID},
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

// SetupWithManager sets up the controller with the Manager.
func (r *CFAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadsv1alpha1.CFApp{}).
		Complete(r)
}
