package main

import (
	"flag"
	"fmt"
	"os"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"
	kpackimagebuilderfinalizer "code.cloudfoundry.org/korifi/kpack-image-builder/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/registry"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"go.uber.org/zap/zapcore"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	k8sclient "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
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
	utilruntime.Must(buildv1alpha2.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		configPath           string
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&configPath, "config", "", "")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	logger, _, err := tools.NewZapLogger(zapcore.InfoLevel)
	if err != nil {
		setupLog.Error(err, "unable to set up zap logger")
		os.Exit(1)
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
		LeaderElectionID:       "13w500bs.cloudfoundry.org",
	})
	if err != nil {
		setupLog.Error(err, "unable to initialize manager")
		os.Exit(1)
	}

	if err = setupControllers(mgr, conf, configPath); err != nil {
		setupLog.Error(err, "unable to set up controllers")
		os.Exit(1)
	}

	if err = setupWebhooks(mgr); err != nil {
		setupLog.Error(err, "unable to set up webhooks")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setupControllers(mgr manager.Manager, restConf *rest.Config, configPath string) error {
	controllersLog := ctrl.Log.WithName("controllers")
	imageClientSet, err := k8sclient.NewForConfig(restConf)
	if err != nil {
		return fmt.Errorf("could not create k8s client: %v", err)
	}

	controllerConfig := &controllers.Config{}
	err = tools.LoadConfigInto(controllerConfig, configPath)
	if err != nil {
		return fmt.Errorf("config could not be read: %v", err)
	}

	imageClient := image.NewClient(imageClientSet)
	if err = controllers.NewBuildWorkloadReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		controllersLog,
		controllerConfig,
		imageClient,
		registry.NewRepositoryCreator(controllerConfig.ContainerRegistryType),
	).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create BuildWorkload controller: %v", err)
	}

	if err = controllers.NewBuilderInfoReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		controllersLog,
		controllerConfig.ClusterBuilderName,
		controllerConfig.CFRootNamespace,
	).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create BuilderInfo controller: %v", err)
	}

	if err = controllers.NewKpackBuildController(
		mgr.GetClient(),
		controllersLog,
		imageClient,
		controllerConfig.BuilderServiceAccount,
	).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create KpackBuild controller: %v", err)
	}

	return nil
}

func setupWebhooks(mgr manager.Manager) error {
	kpackimagebuilderfinalizer.NewKpackImageBuilderFinalizerWebhook().SetupWebhookWithManager(mgr)

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %v", err)
	}

	if err := mgr.AddReadyzCheck("readyz", mgr.GetWebhookServer().StartedChecker()); err != nil {
		return fmt.Errorf("unable to set up ready check: %v", err)
	}

	return nil
}
