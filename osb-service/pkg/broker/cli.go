package broker

import (
	"flag"
	"os"
	"strconv"
)

// Options holds the options specified by the broker's code on the command
// line. Users should add their own options here and add flags for them in
// AddFlags.
type Options struct {
	Async            bool
	PostgresHost     string
	PostgresPort     int
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	// PostgresSSLMode is a libpq sslmode. Default require (TLS is required).
	PostgresSSLMode      string
	EverestKubeconfig    string
	EverestNamespace     string
	EverestHostNamespace string
	EverestVClusterName  string
}

// AddFlags is a hook called to initialize the CLI flags for broker options.
// It is called after the flags are added for the skeleton and before flag
// parse is called.
func AddFlags(o *Options) {
	flag.BoolVar(&o.Async, "async", false, "Indicates whether the broker is handling the requests asynchronously.")
	flag.StringVar(&o.PostgresHost, "postgres-host", os.Getenv("POSTGRES_HOST"), "Admin host for the postgres offering (and durable OSB metadata). Empty keeps the in-memory store.")
	flag.IntVar(&o.PostgresPort, "postgres-port", envInt("POSTGRES_PORT", 5432), "Postgres port")
	flag.StringVar(&o.PostgresUser, "postgres-user", envDefault("POSTGRES_USER", "postgres"), "Postgres admin user")
	flag.StringVar(&o.PostgresPassword, "postgres-password", os.Getenv("POSTGRES_PASSWORD"), "Postgres admin password (prefer POSTGRES_PASSWORD)")
	flag.StringVar(&o.PostgresDB, "postgres-db", envDefault("POSTGRES_DB", "postgres"), "Postgres maintenance database")
	flag.StringVar(&o.PostgresSSLMode, "postgres-sslmode", envDefault("POSTGRES_SSLMODE", "require"), "libpq sslmode for the admin connection (default require; TLS required)")
	flag.StringVar(&o.EverestKubeconfig, "everest-kubeconfig", envDefault("EVEREST_KUBECONFIG", ""), "kubeconfig for the OpenEverest vcluster (empty keeps fake/in-memory postgres provision)")
	flag.StringVar(&o.EverestNamespace, "everest-namespace", envDefault("EVEREST_NAMESPACE", "everest"), "vcluster namespace for DatabaseClusters")
	flag.StringVar(&o.EverestHostNamespace, "everest-host-namespace", envDefault("EVEREST_HOST_NAMESPACE", "everest-vcluster"), "host namespace where vcluster syncs Services")
	flag.StringVar(&o.EverestVClusterName, "everest-vcluster-name", envDefault("EVEREST_VCLUSTER_NAME", "everest"), "vcluster name used in synced Service DNS")
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}
