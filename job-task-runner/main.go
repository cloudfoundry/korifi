package main

import (
	"flag"
	"fmt"
	"os"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	jobtaskcontrollers "code.cloudfoundry.org/korifi/job-task-runner/controllers"
	"code.cloudfoundry.org/korifi/tools"
	"go.uber.org/zap/zapcore"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"k8s.io/apimachinery/pkg/runtime"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		ttl                  string
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&ttl, "ttl", "24h", "The time to live for a task job.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	logger, _, err := tools.NewZapLogger(zapcore.InfoLevel)
	if err != nil {
		panic(fmt.Sprintf("error creating new zap logger: %v", err))
	}

	ctrl.SetLogger(logger)
	klog.SetLogger(ctrl.Log)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "13c300bs.cloudfoundry.org",
	})
	if err != nil {
		setupLog.Error(err, "unable to initialize manager")
		os.Exit(1)
	}

	jobTTL, err := tools.ParseDuration(ttl)
	if err != nil {
		setupLog.Error(err, "unable to parse job TTL", "ttl", ttl)
		os.Exit(1)
	}

	controllersLog := ctrl.Log.WithName("controllers")
	if err = jobtaskcontrollers.NewTaskWorkloadReconciler(
		controllersLog,
		mgr.GetClient(),
		mgr.GetScheme(),
		jobtaskcontrollers.NewStatusGetter(mgr.GetClient()),
		jobTTL,
	).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TaskWorkload")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
