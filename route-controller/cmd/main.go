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

package main

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/controllers/k8s"
	controllersfinalizer "code.cloudfoundry.org/korifi/controllers/webhooks/finalizer"
	routeswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/networking/routes"
	"code.cloudfoundry.org/korifi/controllers/webhooks/relationships"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	versionwebhook "code.cloudfoundry.org/korifi/controllers/webhooks/version"
	appswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/apps"
	routeControllers "code.cloudfoundry.org/korifi/route-controller/controllers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/version"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(buildv1alpha2.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gatewayv1beta1.Install(scheme))
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name Client sigs.k8s.io/controller-runtime/pkg/client.Client
//counterfeiter:generate -o fake -fake-name EventRecorder k8s.io/client-go/tools/record.EventRecorder
//counterfeiter:generate -o fake -fake-name StatusWriter sigs.k8s.io/controller-runtime/pkg/client.StatusWriter

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	configPath, found := os.LookupEnv("CONTROLLERSCONFIG")
	if !found {
		panic("CONTROLLERSCONFIG must be set")
	}

	controllerConfig, err := config.LoadFromPath(configPath)
	if err != nil {
		errorMessage := fmt.Sprintf("Config could not be read: %v", err)
		panic(errorMessage)
	}

	logger, atomicLevel, err := tools.NewZapLogger(controllerConfig.LogLevel)
	if err != nil {
		panic(fmt.Sprintf("error creating new zap logger: %v", err))
	}

	ctrl.SetLogger(logger)

	log.SetOutput(&tools.LogrWriter{Logger: ctrl.Log, Message: "HTTP server error"})

	ctrl.Log.Info("starting Korifi controllers", "version", version.Version)

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
		LeaderElectionID:       "13c200ec.cloudfoundry.org",
	})
	if err != nil {
		setupLog.Error(err, "unable to initialize manager")
		os.Exit(1)
	}
	controllersClient := k8s.IgnoreEmptyPatches(mgr.GetClient())
	if os.Getenv("ENABLE_CONTROLLERS") != "false" {
		controllersLog := ctrl.Log.WithName("controllers")

		if !controllerConfig.DisableRouteController {
			if err = routeControllers.NewReconciler(
				controllersClient,
				mgr.GetScheme(),
				controllersLog,
				controllerConfig,
			).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "CFRoute")
				os.Exit(1)
			}
		}
	}
	// Setup webhooks with manager
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {

		(&appswebhook.AppRevWebhook{}).SetupWebhookWithManager(mgr)

		uncachedClient, err := client.New(mgr.GetConfig(), client.Options{
			Scheme: scheme,
		})
		if err != nil {
			setupLog.Error(err, "unable to create uncached client")
			os.Exit(1)
		}

		if err = routeswebhook.NewValidator(
			validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, routeswebhook.RouteEntityType)),
			controllerConfig.CFRootNamespace,
			uncachedClient,
		).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFRoute")
			os.Exit(1)
		}

		if err = korifiv1alpha1.NewCFRouteDefaulter().SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFRoute")
			os.Exit(1)
		}

		versionwebhook.NewVersionWebhook(version.Version).SetupWebhookWithManager(mgr)
		controllersfinalizer.NewControllersFinalizerWebhook().SetupWebhookWithManager(mgr)
		relationships.NewSpaceGUIDWebhook().SetupWebhookWithManager(mgr)

		if err = mgr.AddReadyzCheck("readyz", mgr.GetWebhookServer().StartedChecker()); err != nil {
			setupLog.Error(err, "unable to set up ready check")
			os.Exit(1)
		}
	} else {
		setupLog.Info("skipping webhook setup because ENABLE_WEBHOOKS set to false.")
	}

	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	eventChan := make(chan string)
	go func() {
		setupLog.Info("starting to watch config file at "+configPath+" for logger level changes", "currentLevel", atomicLevel.Level())
		if err2 := tools.WatchForConfigChangeEvents(context.Background(), configPath, setupLog, eventChan); err2 != nil {
			setupLog.Error(err2, "error watching logging config")
			os.Exit(1)
		}
	}()

	go tools.SyncLogLevel(context.Background(), setupLog, eventChan, atomicLevel, config.GetLogLevelFromPath)

	setupLog.Info("starting manager")
	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
