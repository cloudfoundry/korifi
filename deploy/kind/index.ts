/**
 * deploy/kind — Korifi on kind in one `pulumi up`.
 *
 * Layers:
 *
 *   cluster.ts          kind cluster + containerd registry trust
 *   LocalRegistry       in-cluster docker-registry NodePort 30050
 *   KindKorifiImages    docker build + kind load controllers/api/migration
 *   KorifiDependencies  cert-manager, kpack, contour, metrics-server
 *   KorifiRelease       in-tree Helm chart (knative-runner)
 *   KnativeServing      Operator Helm + KnativeServing CR (Kourier ClusterIP)
 *   ContourGateway      NodePort GatewayClass params
 *
 * Usage:
 *   cd deploy/kind && bun install
 *   export PULUMI_CONFIG_PASSPHRASE=...
 *   pulumi up --stack dev
 */
import * as path from "node:path";
import {
	ContourGateway,
	KindKorifiImages,
	KnativeServing,
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

// In-tree chart + images built from this checkout. Hub *:latest is the last
// release (no knative-runner). The local registry is for apps/kpack only.
const repoRoot = path.join(__dirname, "..", "..");
const localChart = path.join(repoRoot, "helm", "korifi");

const images = new KindKorifiImages(
	"images",
	{
		clusterName,
		repoRoot,
		dependsOn: [cluster],
	},
	{ dependsOn: [cluster] },
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
		chart: localChart,
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
			images: {
				controllers: images.controllersImage,
				api: images.apiImage,
				migration: images.migrationImage,
			},
			extraValues: {
				helm: { hooksImage: "alpine/k8s:1.36.4" },
			},
		},
		dependsOn: [
			dependencies.job,
			registry.release,
			cfPullSecret,
			namespaces.gateway,
			images.loaded,
		],
	},
	{ dependsOn: [dependencies, registry, images] },
);

const knative = new KnativeServing(
	"knative",
	{
		provider: cluster.provider,
		domain: appDomain,
		korifiNamespace: namespaces.korifiName,
		rootNamespace: namespaces.rootName,
		installRunnerSupport: false,
		dependsOn: [korifi.release, dependencies.job],
	},
	{ dependsOn: [korifi] },
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
export const knativeServing = knative.serving.metadata.name;
export const controllersImage = images.controllersImage;
export const apiImage = images.apiImage;
