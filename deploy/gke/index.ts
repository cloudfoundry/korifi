/**
 * deploy/gke — Korifi on Google Kubernetes Engine (INSTALL.md + GAR).
 *
 * Creates the GKE cluster, Artifact Registry repository, registry pull
 * secret, dependencies (incl. Knative), and Korifi with knative-runner.
 */
import * as k8s from "@pulumi/kubernetes";
import {
	ContourGateway,
	KorifiDependencies,
	KorifiNamespaces,
	KorifiRelease,
	registryPullSecret,
} from "@korifi/deploy-lib";
import { GkeRegistry } from "./artifact-registry";
import { GkeCluster } from "./cluster";
import {
	adminUserName,
	apiUrl,
	appDomain,
	artifactRegistryLocation,
	artifactRegistryRepo,
	clusterName,
	gatewayClassName,
	gatewayNamespace,
	location,
	nodeCount,
	nodeMachineType,
	pinned,
	project,
	rootNamespace,
} from "./config";

const cluster = new GkeCluster("gke", {
	project,
	location,
	clusterName,
	nodeMachineType,
	nodeCount,
});

const registry = new GkeRegistry(
	"gar",
	{
		project,
		location: artifactRegistryLocation,
		repositoryId: artifactRegistryRepo,
	},
	{ dependsOn: [cluster] },
);

const namespaces = new KorifiNamespaces(
	"ns",
	{
		provider: cluster.provider,
		rootNamespace,
		gatewayNamespace,
		installerNamespace: true,
	},
	{ dependsOn: [cluster] },
);

const dependencies = new KorifiDependencies(
	"deps",
	{
		provider: cluster.provider,
		knativeDomain: appDomain,
		clusterType: "GKE",
		installerImage: pinned.installerImage,
		installerNamespace: namespaces.installer!.metadata.name,
		dependsOn: [namespaces.installer!],
	},
	{ dependsOn: [namespaces] },
);

/**
 * Bind the configured CF admin identity (typically a GCP user email that
 * already authenticates to the cluster via gcloud) as cluster-admin so
 * `cf login` can select it. Override `adminUserName` to your Google account.
 */
new k8s.rbac.v1.ClusterRoleBinding(
	"cf-admin-binding",
	{
		metadata: { name: `${adminUserName}-korifi-admin` },
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

const cfPullSecret = registryPullSecret(
	"cf-registry-credentials",
	{
		provider: cluster.provider,
		namespace: namespaces.root.metadata.name,
		server: registry.dockerServer,
		username: "_json_key",
		password: registry.serviceAccountKey,
	},
	{ dependsOn: [namespaces.root, registry] },
);

const korifi = new KorifiRelease(
	"korifi",
	{
		provider: cluster.provider,
		namespace: namespaces.korifi.metadata.name,
		chartVersion: pinned.korifi,
		values: {
			platform: "gke",
			adminUserName,
			apiUrl,
			appDomain,
			rootNamespace,
			containerRepositoryPrefix: registry.repositoryPrefix,
			kpackBuilderRepository: registry.kpackBuilderRepository,
			networking: {
				gatewayClass: gatewayClassName,
				gatewayNamespace,
			},
		},
		dependsOn: [
			dependencies.job,
			cfPullSecret,
			namespaces.gateway,
			registry.repository,
		],
	},
	{ dependsOn: [dependencies, registry] },
);

new ContourGateway(
	"contour",
	{
		provider: cluster.provider,
		gatewayClassName,
		publishType: "LoadBalancerService",
		dependsOn: [korifi.release],
	},
	{ dependsOn: [korifi] },
);

export const kubeconfig = cluster.kubeconfig;
export const cfApiUrl = `https://${apiUrl}`;
export const appsDomain = `*.${appDomain}`;
export const containerRepositoryPrefix = registry.repositoryPrefix;
export const kpackBuilderRepository = registry.kpackBuilderRepository;
export const artifactRegistryServer = registry.dockerServer;
export const registryServiceAccount = registry.serviceAccountEmail;
export const dnsHint =
	"Point api.<baseDomain> and *.apps.<baseDomain> at the Contour envoy-korifi LoadBalancer IP (A records).";
