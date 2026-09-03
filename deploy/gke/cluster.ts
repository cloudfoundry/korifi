/**
 * GKE cluster (INSTALL.md prerequisites on GKE).
 */
import * as gcp from "@pulumi/gcp";
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";

export interface GkeClusterArgs {
	project: string;
	location: string;
	clusterName: string;
	nodeMachineType?: string;
	nodeCount?: number;
}

export class GkeCluster extends pulumi.ComponentResource {
	readonly cluster: gcp.container.Cluster;
	readonly nodePool: gcp.container.NodePool;
	readonly kubeconfig: pulumi.Output<string>;
	readonly provider: k8s.Provider;
	readonly clusterName: string;

	constructor(
		name: string,
		args: GkeClusterArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:GkeCluster", name, {}, opts);

		this.clusterName = args.clusterName;

		this.cluster = new gcp.container.Cluster(
			`${name}-cluster`,
			{
				name: args.clusterName,
				location: args.location,
				project: args.project,
				// Node pool is managed separately (remove default pool).
				removeDefaultNodePool: true,
				initialNodeCount: 1,
				networkingMode: "VPC_NATIVE",
				releaseChannel: { channel: "REGULAR" },
				workloadIdentityConfig: {
					workloadPool: `${args.project}.svc.id.goog`,
				},
			},
			{ parent: this },
		);

		this.nodePool = new gcp.container.NodePool(
			`${name}-nodes`,
			{
				name: `${args.clusterName}-pool`,
				location: args.location,
				project: args.project,
				cluster: this.cluster.name,
				nodeCount: args.nodeCount ?? 3,
				nodeConfig: {
					machineType: args.nodeMachineType ?? "e2-standard-4",
					oauthScopes: [
						"https://www.googleapis.com/auth/cloud-platform",
					],
					workloadMetadataConfig: { mode: "GKE_METADATA" },
				},
				management: { autoRepair: true, autoUpgrade: true },
			},
			{ parent: this },
		);

		this.kubeconfig = pulumi
			.all([
				this.cluster.name,
				this.cluster.endpoint,
				this.cluster.masterAuth,
				args.project,
				args.location,
			])
			.apply(([cname, endpoint, auth, project, location]) => {
				const context = `gke_${project}_${location}_${cname}`;
				return `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: ${auth.clusterCaCertificate}
    server: https://${endpoint}
  name: ${context}
contexts:
- context:
    cluster: ${context}
    user: ${context}
  name: ${context}
current-context: ${context}
kind: Config
preferences: {}
users:
- name: ${context}
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: gke-gcloud-auth-plugin
      installHint: Install gke-gcloud-auth-plugin for use with kubectl
      provideClusterInfo: true
`;
			});

		this.provider = new k8s.Provider(
			`${name}-k8s`,
			{ kubeconfig: this.kubeconfig, enableServerSideApply: true },
			{ parent: this, dependsOn: [this.cluster, this.nodePool] },
		);

		this.registerOutputs({
			clusterName: this.clusterName,
			endpoint: this.cluster.endpoint,
		});
	}
}
