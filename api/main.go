package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/actions/manifest"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/stats"
	"code.cloudfoundry.org/korifi/api/middleware"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/payloads/validation"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
	"code.cloudfoundry.org/korifi/api/routing"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/k8s"
	toolsregistry "code.cloudfoundry.org/korifi/tools/registry"
	"code.cloudfoundry.org/korifi/version"

	chiMiddlewares "github.com/go-chi/chi/middleware"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/cache"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var conditionTimeout = time.Second * 120

func init() {
	utilruntime.Must(metav1.AddMetaToScheme(scheme.Scheme))
	metav1.AddToGroupVersion(scheme.Scheme, metav1.SchemeGroupVersion)
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme.Scheme))
	utilruntime.Must(buildv1alpha2.AddToScheme(scheme.Scheme))
	utilruntime.Must(metricsv1beta1.AddToScheme(scheme.Scheme))
}

func main() {
	configPath, found := os.LookupEnv("APICONFIG")
	if !found {
		panic("APICONFIG must be set")
	}
	cfg, err := config.LoadFromPath(configPath)
	if err != nil {
		errorMessage := fmt.Sprintf("Config could not be read: %v", err)
		panic(errorMessage)
	}

	payloads.DefaultLifecycleConfig = cfg.DefaultLifecycleConfig
	payloads.DefaultPageSize = cfg.List.DefaultPageSize

	k8sClientConfig := cfg.GenerateK8sClientConfig(ctrl.GetConfigOrDie())

	logger, atomicLevel, err := tools.NewZapLogger(cfg.LogLevel)
	if err != nil {
		panic(fmt.Sprintf("error creating new zap logger: %v", err))
	}
	ctrl.SetLogger(logger)
	klog.SetLogger(ctrl.Log)

	eventChan := make(chan string)
	go func() {
		ctrl.Log.Info("starting to watch config file at "+configPath+" for logger level changes", "currentLevel", atomicLevel.Level())
		if err2 := tools.WatchForConfigChangeEvents(context.Background(), configPath, ctrl.Log, eventChan); err2 != nil {
			ctrl.Log.Error(err2, "error watching logging config")
			os.Exit(1)
		}
	}()

	go tools.SyncLogLevel(context.Background(), ctrl.Log, eventChan, atomicLevel, config.GetLogLevelFromPath)

	ctrl.Log.Info("starting Korifi API", "version", version.Version)

	k8sClient, err := client.NewWithWatch(k8sClientConfig, client.Options{})
	if err != nil {
		panic(fmt.Sprintf("could not create privileged k8s client: %v", err))
	}
	clientset, err := k8sclient.NewForConfig(k8sClientConfig)
	if err != nil {
		panic(fmt.Sprintf("could not create privileged k8s client: %v", err))
	}
	restClient := clientset.RESTClient()
	pluralizer := descriptors.NewCachingPluralizer(discovery.NewDiscoveryClient(restClient))

	dynamicClient, err := dynamic.NewForConfig(k8sClientConfig)
	if err != nil {
		panic(fmt.Sprintf("could not create dynamic k8s client: %v", err))
	}
	namespaceRetriever := repositories.NewNamespaceRetriever(dynamicClient)

	httpClient, err := rest.HTTPClientFor(k8sClientConfig)
	if err != nil {
		panic(fmt.Sprintf("could not create http client from k8s rest config: %v", err))
	}
	mapper, err := apiutil.NewDynamicRESTMapper(k8sClientConfig, httpClient)
	if err != nil {
		panic(fmt.Sprintf("could not create kubernetes REST mapper: %v", err))
	}

	identityProvider := wireIdentityProvider(k8sClient, k8sClientConfig)
	cachingIdentityProvider := authorization.NewCachingIdentityProvider(identityProvider, cache.NewExpiring())
	nsPermissions := authorization.NewNamespacePermissions(k8sClient, cachingIdentityProvider)

	userClientFactory := authorization.NewUnprivilegedClientFactory(k8sClientConfig, mapper, scheme.Scheme).
		WithWrappingFunc(func(client client.WithWatch) client.WithWatch {
			return k8s.NewRetryingClient(client, k8s.IsForbidden, k8s.NewDefaultBackoff())
		})

	spaceScopedUserClientFactory := userClientFactory.WithWrappingFunc(func(client client.WithWatch) client.WithWatch {
		return authorization.NewSpaceFilteringClient(client, k8sClient, authorization.NewSpaceFilteringOpts(nsPermissions))
	})
	spaceScopedKlient := k8sklient.NewK8sKlient(
		namespaceRetriever,
		spaceScopedUserClientFactory,
		k8sklient.NewDescriptorsBasedLister(
			descriptors.NewClient(restClient, pluralizer, scheme.Scheme, authorization.NewSpaceFilteringOpts(nsPermissions)),
			descriptors.NewObjectListMapper(spaceScopedUserClientFactory),
		),
		scheme.Scheme,
	)

	rootNsUserClientFactory := userClientFactory.WithWrappingFunc(func(client client.WithWatch) client.WithWatch {
		return authorization.NewRootNSFilteringClient(client, cfg.RootNamespace)
	})
	rootNSKlient := k8sklient.NewK8sKlient(
		namespaceRetriever,
		rootNsUserClientFactory,
		k8sklient.NewDescriptorsBasedLister(
			descriptors.NewClient(restClient, pluralizer, scheme.Scheme, authorization.NewRootNsFilteringOpts(cfg.RootNamespace)),
			descriptors.NewObjectListMapper(rootNsUserClientFactory),
		),
		scheme.Scheme,
	)

	serverURL, err := url.Parse(cfg.ServerURL)
	if err != nil {
		panic(fmt.Sprintf("could not parse server URL: %v", err))
	}

	paramsClient := repositories.NewServiceBrokerClient(
		osbapi.NewClientFactory(k8sClient, false),
		k8sClient,
		cfg.RootNamespace,
	)

	orgRepo := repositories.NewOrgRepo(
		rootNSKlient,
		cfg.RootNamespace,
		nsPermissions,
		conditions.NewConditionAwaiter[*korifiv1alpha1.CFOrg, korifiv1alpha1.CFOrgList](conditionTimeout),
	)
	spaceRepo := repositories.NewSpaceRepo(
		spaceScopedKlient,
		orgRepo,
		nsPermissions,
		conditions.NewConditionAwaiter[*korifiv1alpha1.CFSpace, korifiv1alpha1.CFSpaceList](conditionTimeout),
	)
	processRepo := repositories.NewProcessRepo(spaceScopedKlient)
	podRepo := repositories.NewPodRepo(userClientFactory)
	appRepo := repositories.NewAppRepo(
		spaceScopedKlient,
		conditions.NewConditionAwaiter[*korifiv1alpha1.CFApp, korifiv1alpha1.CFAppList](conditionTimeout),
	)
	dropletRepo := repositories.NewDropletRepo(spaceScopedKlient)
	routeRepo := repositories.NewRouteRepo(spaceScopedKlient)
	domainRepo := repositories.NewDomainRepo(
		rootNSKlient,
		cfg.RootNamespace,
	)
	deploymentRepo := repositories.NewDeploymentRepo(
		spaceScopedKlient,
	)
	buildRepo := repositories.NewBuildRepo(
		spaceScopedKlient,
	)
	logRepo := repositories.NewLogRepo(
		userClientFactory,
		authorization.NewUnprivilegedClientsetFactory(k8sClientConfig),
		repositories.DefaultLogStreamer,
	)
	runnerInfoRepo := repositories.NewRunnerInfoRepository(
		rootNSKlient,
		cfg.RunnerName,
		cfg.RootNamespace,
	)
	packageRepo := repositories.NewPackageRepo(
		spaceScopedKlient,
		toolsregistry.NewRepositoryCreator(cfg.ContainerRegistryType),
		cfg.ContainerRepositoryPrefix,
		conditions.NewConditionAwaiter[*korifiv1alpha1.CFPackage, korifiv1alpha1.CFPackageList](conditionTimeout),
	)
	serviceInstanceRepo := repositories.NewServiceInstanceRepo(
		spaceScopedKlient,
		conditions.NewConditionAwaiter[*korifiv1alpha1.CFServiceInstance, korifiv1alpha1.CFServiceInstanceList](conditionTimeout),
		cfg.RootNamespace,
	)
	serviceBindingRepo := repositories.NewServiceBindingRepo(
		spaceScopedKlient,
		conditions.NewConditionAwaiter[*korifiv1alpha1.CFServiceBinding, korifiv1alpha1.CFServiceBindingList](conditionTimeout),
		conditions.NewConditionAwaiter[*korifiv1alpha1.CFApp, korifiv1alpha1.CFAppList](conditionTimeout),
		paramsClient,
	)
	stackRepo := repositories.NewStackRepository(
		rootNSKlient,
		cfg.BuilderName,
		cfg.RootNamespace,
	)
	buildpackRepo := repositories.NewBuildpackRepository(
		rootNSKlient,
		cfg.BuilderName,
		cfg.RootNamespace,
		repositories.NewBuildpackSorter(),
	)
	roleRepo := repositories.NewRoleRepo(
		spaceScopedKlient,
		spaceRepo,
		authorization.NewNamespacePermissions(k8sClient, cachingIdentityProvider),
		authorization.NewNamespacePermissions(k8sClient, cachingIdentityProvider),
		cfg.RootNamespace,
		cfg.RoleMappings,
		repositories.NewRoleSorter(),
	)
	imageClient := image.NewClient(clientset)
	imageRepo := repositories.NewImageRepository(
		userClientFactory,
		imageClient,
		cfg.PackageRegistrySecretNames,
		cfg.RootNamespace,
	)
	taskRepo := repositories.NewTaskRepo(
		spaceScopedKlient,
		conditions.NewConditionAwaiter[*korifiv1alpha1.CFTask, korifiv1alpha1.CFTaskList](conditionTimeout),
	)
	metricsRepo := repositories.NewMetricsRepo(userClientFactory)
	serviceBrokerRepo := repositories.NewServiceBrokerRepo(rootNSKlient, cfg.RootNamespace)
	serviceOfferingRepo := repositories.NewServiceOfferingRepo(rootNSKlient, spaceScopedKlient, cfg.RootNamespace)
	servicePlanRepo := repositories.NewServicePlanRepo(rootNSKlient, cfg.RootNamespace, orgRepo)
	securityGroupRepo := repositories.NewSecurityGroupRepo(rootNSKlient, cfg.RootNamespace)
	userRepo := repositories.NewUserRepository()

	processStats := actions.NewProcessStats(processRepo, appRepo, metricsRepo)
	manifest := actions.NewManifest(
		domainRepo,
		cfg.DefaultDomainName,
		manifest.NewStateCollector(appRepo, domainRepo, processRepo, routeRepo, serviceInstanceRepo, serviceBindingRepo),
		manifest.NewNormalizer(cfg.DefaultDomainName),
		manifest.NewApplier(appRepo, domainRepo, processRepo, routeRepo, serviceInstanceRepo, serviceBindingRepo),
	)

	requestValidator := validation.NewDefaultDecoderValidator()

	routerBuilder := routing.NewRouterBuilder()
	routerBuilder.UseMiddleware(
		middleware.Correlation(ctrl.Log),
		middleware.CFCliVersion,
		middleware.HTTPLogging,
		chiMiddlewares.StripSlashes,
	)

	if !cfg.Experimental.ManagedServices.Enabled {
		routerBuilder.UseMiddleware(middleware.DisableManagedServices)
	}

	if !cfg.Experimental.SecurityGroups.Enabled {
		routerBuilder.UseMiddleware(middleware.DisableSecurityGroups)
	}

	authInfoParser := authorization.NewInfoParser()
	routerBuilder.UseAuthMiddleware(
		middleware.Authentication(
			authInfoParser,
			cachingIdentityProvider,
		),
		middleware.CFUser(
			nsPermissions,
			cachingIdentityProvider,
			cfg.RootNamespace,
			cache.NewExpiring(),
		),
	)

	relationshipsRepo := relationships.NewResourseRelationshipsRepo(
		serviceOfferingRepo,
		serviceBrokerRepo,
		servicePlanRepo,
		spaceRepo,
		orgRepo,
	)

	logCacheURL, gaugesCollector, err := wireGaugeCollector(cfg)
	if err != nil {
		panic(fmt.Sprintf("could not create log cache client: %v", err))
	}

	instancesStateCollector := stats.NewProcessInstanceStateCollector(processRepo)
	apiHandlers := []routing.Routable{
		handlers.NewRootV3(*serverURL),
		handlers.NewRoot(*serverURL, cfg.Experimental.UAA, *logCacheURL),
		handlers.NewInfoV3(
			*serverURL,
			cfg.InfoConfig,
		),
		handlers.NewResourceMatches(),
		handlers.NewApp(
			*serverURL,
			appRepo,
			dropletRepo,
			processRepo,
			routeRepo,
			domainRepo,
			spaceRepo,
			packageRepo,
			requestValidator,
			podRepo,
			gaugesCollector,
			instancesStateCollector,
		),
		handlers.NewRoute(
			*serverURL,
			routeRepo,
			domainRepo,
			appRepo,
			spaceRepo,
			requestValidator,
		),
		handlers.NewServiceRouteBinding(
			*serverURL,
		),
		handlers.NewPackage(
			*serverURL,
			packageRepo,
			appRepo,
			dropletRepo,
			imageRepo,
			requestValidator,
			cfg.PackageRegistrySecretNames,
		),
		handlers.NewBuild(
			*serverURL,
			buildRepo,
			packageRepo,
			appRepo,
			requestValidator,
		),
		handlers.NewDroplet(
			*serverURL,
			dropletRepo,
			requestValidator,
		),
		handlers.NewProcess(
			*serverURL,
			processRepo,
			requestValidator,
			podRepo,
			gaugesCollector,
			instancesStateCollector,
		),
		handlers.NewDomain(
			*serverURL,
			requestValidator,
			domainRepo,
		),
		handlers.NewDeployment(
			*serverURL,
			requestValidator,
			deploymentRepo,
			runnerInfoRepo,
			cfg.RunnerName,
		),
		handlers.NewStack(
			*serverURL,
			stackRepo,
			requestValidator,
		),
		handlers.NewJob(
			*serverURL,
			map[string]handlers.DeletionRepository{
				handlers.OrgDeleteJobType:                    orgRepo,
				handlers.SpaceDeleteJobType:                  spaceRepo,
				handlers.AppDeleteJobType:                    appRepo,
				handlers.RouteDeleteJobType:                  routeRepo,
				handlers.DomainDeleteJobType:                 domainRepo,
				handlers.RoleDeleteJobType:                   roleRepo,
				handlers.ServiceBrokerDeleteJobType:          serviceBrokerRepo,
				handlers.ManagedServiceInstanceDeleteJobType: serviceInstanceRepo,
				handlers.ManagedServiceBindingDeleteJobType:  serviceBindingRepo,
			},
			map[string]handlers.StateRepository{
				handlers.ServiceBrokerCreateJobType:          serviceBrokerRepo,
				handlers.ServiceBrokerUpdateJobType:          serviceBrokerRepo,
				handlers.ManagedServiceInstanceCreateJobType: serviceInstanceRepo,
				handlers.ManagedServiceBindingCreateJobType:  serviceBindingRepo,
			},
			routeRepo,
			500*time.Millisecond,
		),
		handlers.NewOrg(
			*serverURL,
			orgRepo,
			domainRepo,
			requestValidator,
			cfg.GetUserCertificateDuration(),
			cfg.DefaultDomainName,
		),
		handlers.NewSpace(
			*serverURL,
			spaceRepo,
			orgRepo,
			routeRepo,
			requestValidator,
			relationshipsRepo,
		),
		handlers.NewSpaceManifest(
			*serverURL,
			manifest,
			spaceRepo,
			requestValidator,
		),
		handlers.NewRole(
			*serverURL,
			roleRepo,
			requestValidator,
		),
		handlers.NewWhoAmI(cachingIdentityProvider, *serverURL),
		handlers.NewUser(*serverURL, userRepo, requestValidator),
		handlers.NewBuildpack(
			*serverURL,
			buildpackRepo,
			requestValidator,
		),
		handlers.NewServiceInstance(
			*serverURL,
			serviceInstanceRepo,
			spaceRepo,
			requestValidator,
			relationshipsRepo,
		),
		handlers.NewServiceBinding(
			*serverURL,
			serviceBindingRepo,
			appRepo,
			serviceInstanceRepo,
			requestValidator,
		),
		handlers.NewTask(
			*serverURL,
			appRepo,
			taskRepo,
			requestValidator,
		),
		handlers.NewServiceBroker(
			*serverURL,
			serviceBrokerRepo,
			requestValidator,
		),
		handlers.NewServiceOffering(
			*serverURL,
			requestValidator,
			serviceOfferingRepo,
			serviceBrokerRepo,
			relationshipsRepo,
		),
		handlers.NewServicePlan(
			*serverURL,
			requestValidator,
			servicePlanRepo,
			relationshipsRepo,
		),
		handlers.NewSecurityGroup(
			*serverURL,
			securityGroupRepo,
			spaceRepo,
			requestValidator,
		),
	}

	if !cfg.Experimental.ExternalLogCache.Enabled {
		apiHandlers = append(apiHandlers, handlers.NewLogCache(
			requestValidator,
			appRepo,
			buildRepo,
			logRepo,
			processStats,
		))
	}

	for _, handler := range apiHandlers {
		routerBuilder.LoadRoutes(handler)
	}

	routerBuilder.SetNotFoundHandler(handlers.NotFound)
	routerBuilder.SetMethodNotAllowedHandler(handlers.NotFound)

	portString := fmt.Sprintf(":%v", cfg.InternalPort)

	certWatcher := createCertWatcher("CERT_PATH")
	internalCertWatcher := createCertWatcher("INTERNAL_CERT_PATH")

	if err := (&http.Server{
		Addr:              portString,
		Handler:           routerBuilder.Build(),
		IdleTimeout:       time.Duration(cfg.IdleTimeout * int(time.Second)),
		ReadTimeout:       time.Duration(cfg.ReadTimeout * int(time.Second)),
		ReadHeaderTimeout: time.Duration(cfg.ReadHeaderTimeout * int(time.Second)),
		WriteTimeout:      time.Duration(cfg.WriteTimeout * int(time.Second)),
		ErrorLog:          log.New(&tools.LogrWriter{Logger: ctrl.Log, Message: "HTTP server error"}, "", 0),
		TLSConfig: &tls.Config{
			NextProtos: []string{"h2"},
			MinVersion: tls.VersionTLS12,
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				if hello.ServerName == cfg.InternalFQDN {
					return internalCertWatcher.GetCertificate(hello)
				}
				return certWatcher.GetCertificate(hello)
			},
		},
	}).ListenAndServeTLS("", ""); err != nil {
		ctrl.Log.Error(err, "error serving TLS")
		os.Exit(1)
	}
}

func createCertWatcher(envVar string) *certwatcher.CertWatcher {
	tlsPath := os.Getenv(envVar)
	certPath := filepath.Join(tlsPath, "tls.crt")
	keyPath := filepath.Join(tlsPath, "tls.key")

	var certWatcher *certwatcher.CertWatcher
	certWatcher, err := certwatcher.New(certPath, keyPath)
	if err != nil {
		ctrl.Log.Error(err, "error creating TLS watcher")
		os.Exit(1)
	}

	go func() {
		if err2 := certWatcher.Start(context.Background()); err2 != nil {
			ctrl.Log.Error(err2, "error watching TLS")
			os.Exit(1)
		}
	}()

	return certWatcher
}

func wireIdentityProvider(client client.Client, restConfig *rest.Config) authorization.IdentityProvider {
	tokenReviewer := authorization.NewTokenReviewer(client)
	certInspector := authorization.NewCertInspector(restConfig)
	return authorization.NewCertTokenIdentityProvider(tokenReviewer, certInspector)
}

func wireGaugeCollector(cfg *config.APIConfig) (*url.URL, handlers.GaugesCollector, error) {
	if !cfg.Experimental.ExternalLogCache.Enabled {
		logCacheURL, err := url.Parse(cfg.ServerURL)
		if err != nil {
			return nil, nil, fmt.Errorf("could not parse internal log cache URL: %w", err)
		}

		return logCacheURL, stats.NewGaugesCollector(
			fmt.Sprintf("https://%s", cfg.InternalFQDN),
			&http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						MinVersion: tls.VersionTLS12,
						RootCAs:    loadCA("INTERNAL_CERT_PATH"),
					},
				},
			},
		), nil
	}

	logCacheURL, err := url.Parse(cfg.Experimental.ExternalLogCache.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("could not parse external log cache URL: %w", err)
	}

	return logCacheURL, stats.NewGaugesCollector(
		logCacheURL.String(),
		&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: cfg.Experimental.ExternalLogCache.TrustInsecureLogCache, // #nosec G402
				},
			},
		},
	), nil
}

func loadCA(caPathEnv string) *x509.CertPool {
	caPath := os.Getenv(caPathEnv)
	caPEM, err := os.ReadFile(filepath.Join(caPath, "ca.crt"))
	if err != nil {
		ctrl.Log.Error(err, "could not read CA bundle from file", "path", caPath)
		os.Exit(1)
	}
	trustedCAs := x509.NewCertPool()
	if ok := trustedCAs.AppendCertsFromPEM(caPEM); !ok {
		ctrl.Log.Error(err, "could not append CA bundle to trustedCAs cert pool", "path", caPath)
		os.Exit(1)
	}
	return trustedCAs
}
