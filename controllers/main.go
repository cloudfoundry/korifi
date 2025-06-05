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
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/cleanup"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/networking/domains"
	"code.cloudfoundry.org/korifi/controllers/controllers/networking/routes"
	securitygroups "code.cloudfoundry.org/korifi/controllers/controllers/networking/security_groups"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/bindings"
	managed_bindings "code.cloudfoundry.org/korifi/controllers/controllers/services/bindings/managed"
	upsi_bindings "code.cloudfoundry.org/korifi/controllers/controllers/services/bindings/upsi"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/brokers"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/instances/managed"
	upsi_instances "code.cloudfoundry.org/korifi/controllers/controllers/services/instances/upsi"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/apps"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/build/buildpack"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/build/docker"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/labels"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/orgs"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/packages"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/processes"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/spaces"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/tasks"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/controllers/k8s"
	"code.cloudfoundry.org/korifi/controllers/webhooks/common_labels"
	controllersfinalizer "code.cloudfoundry.org/korifi/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer"
	domainswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/networking/domains"
	routeswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/networking/routes"
	securitygroupswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/networking/security_groups"
	bindingswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/services/bindings"
	brokerswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/services/brokers"
	instanceswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/services/instances"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	versionwebhook "code.cloudfoundry.org/korifi/controllers/webhooks/version"
	appswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/apps"
	orgswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/orgs"
	packageswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/packages"
	spaceswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/spaces"
	taskswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/tasks"
	projectcalicoV3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"

	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/version"

	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"github.com/projectcalico/api/pkg/client/clientset_generated/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	k8sclient "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	admission "k8s.io/pod-security-admission/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
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
	utilruntime.Must(projectcalicoV3.AddToScheme(scheme))
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
	klog.SetLogger(ctrl.Log)

	log.SetOutput(&tools.LogrWriter{Logger: ctrl.Log, Message: "HTTP server error"})

	ctrl.Log.Info("starting Korifi controllers", "version", version.Version)

	conf := ctrl.GetConfigOrDie()
	calicoClient, err := clientset.NewForConfig(conf)
	if err != nil {
		panic(fmt.Sprintf("unable to create Calico client: %v", err))
	}

	k8sClient, err := k8sclient.NewForConfig(conf)
	if err != nil {
		panic(fmt.Sprintf("could not create k8s client: %v", err))
	}

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
		imageClient := image.NewClient(k8sClient)

		if err = apps.NewReconciler(
			controllersClient,
			mgr.GetScheme(),
			controllersLog,
			env.NewVCAPServicesEnvValueBuilder(controllersClient),
			env.NewVCAPApplicationEnvValueBuilder(controllersClient, controllerConfig.ExtraVCAPApplicationValues),
		).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "CFApp")
			os.Exit(1)
		}

		buildCleaner := cleanup.NewBuildCleaner(controllersClient, controllerConfig.MaxRetainedBuildsPerApp)
		if err = buildpack.NewReconciler(
			controllersClient,
			buildCleaner,
			mgr.GetScheme(),
			controllersLog,
			controllerConfig,
			env.NewAppEnvBuilder(controllersClient),
		).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "CFBuildpackBuild")
			os.Exit(1)
		}

		if err = docker.NewReconciler(
			controllersClient,
			buildCleaner,
			imageClient,
			mgr.GetScheme(),
			controllersLog,
		).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "CFDockerBuild")
			os.Exit(1)
		}

		if err = packages.NewReconciler(
			controllersClient,
			mgr.GetScheme(),
			controllersLog,
			imageClient,
			cleanup.NewPackageCleaner(controllersClient, controllerConfig.MaxRetainedPackagesPerApp),
			controllerConfig.ContainerRegistrySecretNames,
		).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "CFPackage")
			os.Exit(1)
		}

		if err = processes.NewReconciler(
			controllersClient,
			mgr.GetScheme(),
			controllersLog,
			controllerConfig,
			env.NewProcessEnvBuilder(controllersClient),
		).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "CFProcess")
			os.Exit(1)
		}

		if err = (upsi_instances.NewReconciler(
			controllersClient,
			mgr.GetScheme(),
			controllersLog,
		)).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "UPSICFServiceInstance")
			os.Exit(1)
		}

		if err = (bindings.NewReconciler(
			controllersClient,
			mgr.GetScheme(),
			controllersLog,
			upsi_bindings.NewReconciler(controllersClient, mgr.GetScheme()),
			managed_bindings.NewReconciler(
				controllersClient,
				osbapi.NewClientFactory(controllersClient, controllerConfig.TrustInsecureServiceBrokers),
				controllerConfig.CFRootNamespace,
				mgr.GetScheme(),
			),
		)).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "CFServiceBinding")
			os.Exit(1)
		}

		labelCompiler := labels.NewCompiler().
			Defaults(map[string]string{
				admission.EnforceLevelLabel: string(admission.LevelRestricted),
				admission.AuditLevelLabel:   string(admission.LevelRestricted),
			}).
			Defaults(controllerConfig.NamespaceLabels)

		if err = orgs.NewReconciler(
			controllersClient,
			controllersLog,
			controllerConfig.ContainerRegistrySecretNames,
			labelCompiler,
		).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "CFOrg")
			os.Exit(1)
		}

		if err = spaces.NewReconciler(
			controllersClient,
			controllersLog,
			controllerConfig.ContainerRegistrySecretNames,
			controllerConfig.CFRootNamespace,
			*controllerConfig.SpaceFinalizerAppDeletionTimeout,
			labelCompiler,
		).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "CFSpace")
			os.Exit(1)
		}

		var taskTTL time.Duration
		taskTTL, err = controllerConfig.ParseTaskTTL()
		if err != nil {
			setupLog.Error(err, "failed to parse task TTL", "controller", "CFTask", "taskTTL", controllerConfig.TaskTTL)
			os.Exit(1)

		}
		if err = tasks.NewReconciler(
			controllersClient,
			mgr.GetScheme(),
			mgr.GetEventRecorderFor("cftask-controller"),
			controllersLog,
			env.NewAppEnvBuilder(controllersClient),
			taskTTL,
		).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "CFTask")
			os.Exit(1)
		}

		if err = domains.NewReconciler(
			controllersClient,
			mgr.GetScheme(),
			controllersLog,
		).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "CFDomain")
			os.Exit(1)
		}

		if controllerConfig.ExperimentalSecurityGroupsEnabled {
			if err = securitygroups.NewReconciler(
				mgr.GetClient(),
				calicoClient,
				controllersLog,
				controllerConfig.CFRootNamespace,
			).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "CFSecurityGroup")
				os.Exit(1)
			}
		}

		if controllerConfig.ExperimentalManagedServicesEnabled {
			if err = brokers.NewReconciler(
				controllersClient,
				osbapi.NewClientFactory(controllersClient, controllerConfig.TrustInsecureServiceBrokers),
				mgr.GetScheme(),
				controllersLog,
			).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "CFServiceBroker")
				os.Exit(1)
			}

			if err = managed.NewReconciler(
				controllersClient,
				osbapi.NewClientFactory(controllersClient, controllerConfig.TrustInsecureServiceBrokers),
				mgr.GetScheme(),
				controllerConfig.CFRootNamespace,
				controllersLog,
			).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "ManagedCFServiceInstance")
				os.Exit(1)
			}
		}

		//+kubebuilder:scaffold:builder

		// Setup Index with Manager
		err = shared.SetupIndexWithManager(mgr)
		if err != nil {
			setupLog.Error(err, "unable to setup index on manager")
			os.Exit(1)
		}

		if !controllerConfig.DisableRouteController {
			if err = routes.NewReconciler(
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
		if err = korifiv1alpha1.NewCFAppDefaulter().SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFApp")
			os.Exit(1)
		}

		(&appswebhook.AppRevWebhook{}).SetupWebhookWithManager(mgr)

		if err = korifiv1alpha1.NewCFProcessDefaulter(
			controllerConfig.CFProcessDefaults.MemoryMB,
			controllerConfig.CFProcessDefaults.DiskQuotaMB,
			*controllerConfig.CFProcessDefaults.Timeout,
		).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFProcess")
			os.Exit(1)
		}

		uncachedClient, err := client.New(mgr.GetConfig(), client.Options{
			Scheme: scheme,
		})
		if err != nil {
			setupLog.Error(err, "unable to create uncached client")
			os.Exit(1)
		}

		if err = appswebhook.NewValidator(
			validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, appswebhook.AppEntityType)),
		).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFApp")
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

		if err = instanceswebhook.NewValidator(
			validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, instanceswebhook.ServiceInstanceEntityType)),
		).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFServiceInstance")
			os.Exit(1)
		}

		if err = bindingswebhook.NewCFServiceBindingValidator(
			validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, bindingswebhook.ServiceBindingEntityType)),
		).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFServiceBinding")
			os.Exit(1)
		}

		if err = domainswebhook.NewValidator(
			uncachedClient,
		).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFDomain")
			os.Exit(1)
		}

		if err = orgswebhook.NewValidator(
			validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, orgswebhook.CFOrgEntityType)),
			validation.NewPlacementValidator(uncachedClient, controllerConfig.CFRootNamespace),
		).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFOrg")
			os.Exit(1)
		}

		if err = spaceswebhook.NewValidator(
			validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, spaceswebhook.CFSpaceEntityType)),
			validation.NewPlacementValidator(uncachedClient, controllerConfig.CFRootNamespace),
		).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFSpace")
			os.Exit(1)
		}

		if err = brokerswebhook.NewValidator(
			validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, brokerswebhook.ServiceBrokerEntityType)),
		).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFServiceBroker")
			os.Exit(1)
		}

		if err = securitygroupswebhook.NewValidator(
			validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, securitygroupswebhook.SecurityGroupEntityType)),
			controllerConfig.CFRootNamespace,
		).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFSecurityGroup")
			os.Exit(1)
		}

		if err = taskswebhook.NewDefaulter(controllerConfig.CFProcessDefaults).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFTask")
			os.Exit(1)
		}

		if err = taskswebhook.NewValidator().SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFTask")
			os.Exit(1)
		}

		versionwebhook.NewVersionWebhook(version.Version).SetupWebhookWithManager(mgr)
		controllersfinalizer.NewControllersFinalizerWebhook().SetupWebhookWithManager(mgr)
		common_labels.NewWebhook().SetupWebhookWithManager(mgr)
		label_indexer.NewWebhook().SetupWebhookWithManager(mgr)

		if err = packageswebhook.NewValidator().SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "CFPackage")
			os.Exit(1)
		}

		if err = mgr.AddReadyzCheck("readyz", mgr.GetWebhookServer().StartedChecker()); err != nil {
			setupLog.Error(err, "unable to set up ready check")
			os.Exit(1)
		}
	} else {
		setupLog.Info("skipping webhook setup because ENABLE_WEBHOOKS set to false.")
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
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
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
