/**
 * deploy/kind — Korifi on kind in one `pulumi up`.
 *
 * Layers:
 *
 *   UaaCerts                 CA + server PEMs for OIDC + TLS proxy
 *   cluster.ts               kind cluster + OIDC + registry NodePorts
 *   LocalRegistry            in-cluster docker-registry NodePort 30050
 *   KorifiDependencies       cert-manager, kpack, contour, metrics-server
 *   UaaVcluster              vcluster + UAA + TLS NodePort proxy :30443
 *   KindKorifiImages         docker build + kind load controllers/api/migration
 *   KorifiRelease            Korifi Helm chart (knative-runner, experimental.uaa)
 *   KnativeServing           Operator Helm + KnativeServing CR (Kourier ClusterIP)
 *   ContourGateway           NodePort GatewayClass params
 *   ServiceBrokerServices    shared Postgres (broker-ready connection facts)
 *
 * Usage:
 *   cd deploy/kind && bun install
 *   export PULUMI_CONFIG_PASSPHRASE=...
 *   pulumi up --stack dev
 */
import * as path from "node:path";
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import {
	ContourGateway,
	KindKorifiImages,
	KnativeServing,
	KorifiDependencies,
	KorifiNamespaces,
	KorifiRelease,
	LocalRegistry,
	ServiceBrokerServices,
	UaaCerts,
	UaaVcluster,
	kindGatewayPorts,
	kindKpackBuilderRepository,
	kindRegistryPrefix,
	kindUaaHostname,
	kindUaaNodePort,
} from "@korifi/deploy-lib";
import { KindCluster } from "./cluster";
import {
	adminEmail,
	apiUrl,
	appDomain,
	clusterName,
	kubeconfigPath,
	oidcPrefix,
	pinned,
	registryUser,
} from "./config";

const uaaUrl = `https://127.0.0.1:${kindUaaNodePort}/uaa`;
const adminUserName = `${oidcPrefix}:${adminEmail}`;

const certs = new UaaCerts("uaa-certs", {
	hostname: kindUaaHostname,
});

const cluster = new KindCluster(
	"kind",
	{
		clusterName,
		kubeconfigPath,
		oidc: {
			issuerUrl: `${uaaUrl}/oauth/token`,
			caDir: certs.outputDir,
			clientId: "cf",
			usernameClaim: "user_name",
			usernamePrefix: `${oidcPrefix}:`,
		},
	},
	{ dependsOn: [certs.filesReady] },
);

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

const korifiPullSecret = registry.pullSecret(
	"korifi-registry-credentials",
	namespaces.korifi.metadata.name,
	{
		provider: cluster.provider,
		dependsOn: [registry.release, namespaces.korifi],
	},
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

const uaa = new UaaVcluster(
	"uaa",
	{
		provider: cluster.provider,
		certs,
		kindClusterName: clusterName,
		uaaUrl,
		adminEmail,
		oidcPrefix,
		dependsOn: [dependencies.job, cluster],
	},
	{ dependsOn: [dependencies, certs] },
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
			uaaUrl,
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
			korifiPullSecret,
			namespaces.gateway,
			uaa.proxyService,
			images.loaded,
		],
	},
	{ dependsOn: [dependencies, registry, uaa, images] },
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

new k8s.rbac.v1.ClusterRoleBinding(
	"uaa-admin-cluster-admin",
	{
		metadata: { name: "uaa-admin-cluster-admin" },
		roleRef: {
			apiGroup: "rbac.authorization.k8s.io",
			kind: "ClusterRole",
			name: "cluster-admin",
		},
		subjects: [
			{
				apiGroup: "rbac.authorization.k8s.io",
				kind: "User",
				name: adminUserName,
			},
		],
	},
	{ provider: cluster.provider, dependsOn: [cluster] },
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

const brokerServices = new ServiceBrokerServices(
	"broker-services",
	{
		provider: cluster.provider,
		enable: { postgres: true },
		dependsOn: [korifi.release],
	},
	{ dependsOn: [korifi] },
);

export const kubeconfig = kubeconfigPath;
export const cfApiUrl = `https://${apiUrl}`;
export const appsDomain = `*.${appDomain}`;
export const orgHint = "cf create-org org && cf create-space -o org space";
export const authHint = `cf api ${cfApiUrl} --skip-ssl-validation && cf login -u ${adminEmail} -p "$(pulumi stack output uaaAdminPassword --show-secrets)"`;
export const uaaIssuerUrl = uaaUrl;
export const uaaAdminEmail = adminEmail;
export const uaaAdminPassword = pulumi.secret(uaa.adminPassword);
export const cfAdminUserName = adminUserName;
export const gatewayClass = gateway.gatewayClass.metadata.name;
export const registryHost = registry.clusterHost;
export const knativeServing = knative.serving.metadata.name;
export const controllersImage = images.controllersImage;
export const apiImage = images.apiImage;

export const postgres = brokerServices.postgres
	? {
			host: brokerServices.postgres.host,
			port: brokerServices.postgres.port,
			adminUser: brokerServices.postgres.adminUser,
			adminPassword: pulumi.secret(brokerServices.postgres.adminPassword),
			adminUrl: pulumi.secret(brokerServices.postgres.adminUrl!),
		}
	: undefined;
