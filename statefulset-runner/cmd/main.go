package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/cmd/wiring"
	eirinischeme "code.cloudfoundry.org/korifi/statefulset-runner/pkg/generated/clientset/versioned/scheme"
	"code.cloudfoundry.org/korifi/statefulset-runner/util"
	"code.cloudfoundry.org/lager"
	"github.com/jessevdk/go-flags"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type options struct {
	ConfigFile string `short:"c" long:"config" description:"Config for running eirini-controller"`
}

type wiringFunc func(loger lager.Logger, manager manager.Manager, config eirinictrl.ControllerConfig) error

func getWirings() []wiringFunc {
	return []wiringFunc{
		wiring.LRPReconciler,
		wiring.PodCrashReconciler,
		wiring.TaskReconciler,
		wiring.ResourceValidator,
		wiring.InstanceIndexEnvInjector,
	}
}

func main() {
	if err := kscheme.AddToScheme(eirinischeme.Scheme); err != nil {
		exitf("failed to add the k8s scheme to the LRP CRD scheme: %v", err)
	}

	var opts options
	_, err := flags.ParseArgs(&opts, os.Args)
	exitfIfError(err, "Failed to parse args")

	var cfg eirinictrl.ControllerConfig
	err = readConfigFile(opts.ConfigFile, &cfg)
	exitfIfError(err, "Failed to read config file")

	kubeConfig, err := clientcmd.BuildConfigFromFlags("", cfg.ConfigPath)
	exitfIfError(err, "Failed to build kubeconfig")

	exitfIfError(err, "Failed to create k8s runtime client")

	logger := lager.NewLogger("eirini-controller")
	logger.RegisterSink(lager.NewPrettySink(os.Stdout, lager.DEBUG))

	certDir := getEnvOrDefault(
		eirinictrl.EnvEiriniCertsDir,
		eirinictrl.EiriniCertsDir,
	)

	managerOptions := manager.Options{
		MetricsBindAddress: "0",
		Scheme:             eirinischeme.Scheme,
		Namespace:          cfg.WorkloadsNamespace,
		Logger:             util.NewLagerLogr(logger),
		LeaderElection:     true,
		LeaderElectionID:   "eirini-controller-leader",
		CertDir:            certDir,
		Host:               "0.0.0.0",
		Port:               int(cfg.WebhookPort),
	}

	if cfg.PrometheusPort > 0 {
		managerOptions.MetricsBindAddress = fmt.Sprintf(":%d", cfg.PrometheusPort)
	}

	if cfg.LeaderElectionID != "" {
		managerOptions.LeaderElectionNamespace = cfg.LeaderElectionNamespace
		managerOptions.LeaderElectionID = cfg.LeaderElectionID
	}

	mgr, err := manager.New(kubeConfig, managerOptions)
	exitfIfError(err, "Failed to create k8s controller runtime manager")

	for _, wire := range getWirings() {
		exitfIfError(wire(logger, mgr, cfg), "wiring failure")
	}

	err = mgr.Start(ctrl.SetupSignalHandler())
	exitfIfError(err, "Failed to start manager")
}

func readConfigFile(path string, conf interface{}) error {
	if path == "" {
		return nil
	}

	fileBytes, err := ioutil.ReadFile(filepath.Clean(path))
	if err != nil {
		return errors.Wrap(err, "failed to read file")
	}

	return errors.Wrap(yaml.Unmarshal(fileBytes, conf), "failed to unmarshal yaml")
}

func exitIfError(err error) {
	exitfIfError(err, "an unexpected error occurred")
}

func exitfIfError(err error, message string) {
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("%s: %w", message, err))
		os.Exit(1)
	}
}

func exitf(messageFormat string, args ...interface{}) {
	exitIfError(fmt.Errorf(messageFormat, args...))
}

func getOrDefault(actualValue, defaultValue string) string {
	if actualValue != "" {
		return actualValue
	}

	return defaultValue
}

func getEnvOrDefault(envVar, defaultValue string) string {
	return getOrDefault(os.Getenv(envVar), defaultValue)
}
