/**
 * deploy/kind — Korifi on kind in one `pulumi up`.
 *
 * Layers (each maps to INSTALL.kind.md / the kind installer job):
 *
 *   cluster.ts          kind cluster + containerd registry trust
 *   LocalRegistry       in-cluster docker-registry NodePort 30050
 *   KorifiDependencies  cert-manager, kpack, contour, knative, metrics-server
 *   KorifiRelease       Korifi Helm chart (knative-runner)
 *   ContourGateway      NodePort GatewayClass params
 *
 * Usage:
 *   cd deploy/kind && bun install
 *   export PULUMI_CONFIG_PASSPHRASE=...
 *   pulumi up --stack dev
 */
import {
	ContourGateway,
	KorifiDependencies,
	KorifiNamespaces,
	KorifiRelease,
	LocalRegistry,
	kindGatewayPorts,
	kindKpackBuilderRepository,
	kindRegistryPrefix,
} from "@korifi/deploy-lib";
import { KindCluster } from "./cluster";
import {
	adminUserName,
	apiUrl,
	appDomain,
	clusterName,
	kubeconfigPath,
	pinned,
	registryUser,
} from "./config";

const cluster = new KindCluster("kind", {
	clusterName,
	kubeconfigPath,
});

const namespaces = new KorifiNamespaces(
	"ns",
	{ provider: cluster.provider, installerNamespace: true },
	{ dependsOn: [cluster] },
);

const registry = new LocalRegistry(
	"registry",
	{
		provider: cluster.provider,
		username: registryUser,
		dependsOn: [namespaces],
	},
	{ dependsOn: [namespaces] },
);

const cfPullSecret = registry.pullSecret(
	"cf-registry-credentials",
	namespaces.root.metadata.name,
	{ provider: cluster.provider, dependsOn: [registry.release, namespaces.root] },
);

const dependencies = new KorifiDependencies(
	"deps",
	{
		provider: cluster.provider,
		knativeDomain: appDomain,
		clusterType: "kind",
		insecureTlsMetricsServer: true,
		installerImage: pinned.installerImage,
		installerNamespace: namespaces.installer!.metadata.name,
		dependsOn: [namespaces.installer!],
	},
	{ dependsOn: [namespaces] },
);

const korifi = new KorifiRelease(
	"korifi",
	{
		provider: cluster.provider,
		namespace: namespaces.korifi.metadata.name,
		chartVersion: pinned.korifi,
		values: {
			platform: "kind",
			adminUserName,
			apiUrl,
			appDomain,
			containerRepositoryPrefix: kindRegistryPrefix(),
			kpackBuilderRepository: kindKpackBuilderRepository(),
			networking: {
				gatewayClass: "contour",
				gatewayNamespace: namespaces.gatewayName,
				gatewayPorts: kindGatewayPorts,
			},
		},
		dependsOn: [
			dependencies.job,
			registry.release,
			cfPullSecret,
			namespaces.gateway,
		],
	},
	{ dependsOn: [dependencies, registry] },
);

const gateway = new ContourGateway(
	"contour",
	{
		provider: cluster.provider,
		publishType: "NodePortService",
		dependsOn: [korifi.release],
	},
	{ dependsOn: [korifi] },
);

export const kubeconfig = kubeconfigPath;
export const cfApiUrl = `https://${apiUrl}`;
export const appsDomain = `*.${appDomain}`;
export const orgHint = "cf create-org org && cf create-space -o org space";
export const authHint = `cf api ${cfApiUrl} --skip-ssl-validation && cf auth ${adminUserName}`;
export const gatewayClass = gateway.gatewayClass.metadata.name;
export const registryHost = registry.clusterHost;
