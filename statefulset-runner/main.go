package main

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name Client sigs.k8s.io/controller-runtime/pkg/client.Client
//counterfeiter:generate -o fake -fake-name EventRecorder k8s.io/client-go/tools/record.EventRecorder
//counterfeiter:generate -o fake -fake-name StatusWriter sigs.k8s.io/controller-runtime/pkg/client.StatusWriter

import (
	"flag"
	"fmt"
	"os"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	statefulsetcontrollers "code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/tools"
	"go.uber.org/zap/zapcore"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

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
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
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

	conf := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(conf, ctrl.Options{
		Scheme: scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "13c200bs.cloudfoundry.org",
	})
	if err != nil {
		setupLog.Error(err, "unable to initialize manager")
		os.Exit(1)
	}

	if err := setupControllers(mgr); err != nil {
		setupLog.Error(err, "unable to set up controllers")
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

func setupControllers(mgr manager.Manager) error {
	controllersLog := ctrl.Log.WithName("controllers")
	if err := statefulsetcontrollers.NewAppWorkloadReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		statefulsetcontrollers.NewAppWorkloadToStatefulsetConverter(mgr.GetScheme()),
		statefulsetcontrollers.NewPDBUpdater(mgr.GetClient()),
		controllersLog,
	).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create AppWorkload controller: %w", err)
	}

	if err := statefulsetcontrollers.NewRunnerInfoReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		controllersLog,
	).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create RunnerInfo controller: %w", err)
	}

	return nil
}
