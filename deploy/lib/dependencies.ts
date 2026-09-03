/**
 * Korifi cluster dependencies via the release-tested installer image.
 *
 * Replaces running `scripts/install-dependencies.sh` by hand (INSTALL.md
 * § Dependencies). The Job installs cert-manager, kpack, Contour (Gateway
 * provisioner), Knative Operator + Serving (Kourier ClusterIP), and
 * metrics-server when missing — versions pinned inside the installer image.
 */
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import { versions } from "./versions";

export interface KorifiDependenciesArgs {
	provider: k8s.Provider;
	/** Must match Korifi `defaultAppDomainName`. */
	knativeDomain: string;
	/**
	 * When "EKS", skips create-new-user.sh (IRSA / IAM admin is provisioned
	 * outside the Job). Use "GKE" or omit for clusters that need cf-admin.
	 */
	clusterType?: "EKS" | "GKE" | "kind";
	/** kind / minikube need insecure metrics-server TLS args. */
	insecureTlsMetricsServer?: boolean;
	/** Optional Calico CNI (deploy-on-kind.sh); kind installer YAML omits it. */
	installVendoredCalico?: boolean;
	installerImage?: string;
	installerNamespace?: pulumi.Input<string>;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

export class KorifiDependencies extends pulumi.ComponentResource {
	readonly job: k8s.batch.v1.Job;
	readonly serviceAccount: k8s.core.v1.ServiceAccount;

	constructor(
		name: string,
		args: KorifiDependenciesArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:Dependencies", name, {}, opts);

		const ns = args.installerNamespace ?? "korifi-installer";
		const childOpts: pulumi.CustomResourceOptions = {
			parent: this,
			provider: args.provider,
			dependsOn: args.dependsOn,
		};

		this.serviceAccount = new k8s.core.v1.ServiceAccount(
			`${name}-sa`,
			{
				metadata: {
					name: "korifi-installer",
					namespace: ns,
				},
			},
			childOpts,
		);

		const binding = new k8s.rbac.v1.ClusterRoleBinding(
			`${name}-binding`,
			{
				metadata: { name: `${name}-korifi-installer` },
				roleRef: {
					apiGroup: "rbac.authorization.k8s.io",
					kind: "ClusterRole",
					name: "cluster-admin",
				},
				subjects: [
					{
						kind: "ServiceAccount",
						name: this.serviceAccount.metadata.name,
						namespace: ns,
					},
				],
			},
			{ ...childOpts, dependsOn: [this.serviceAccount] },
		);

		const flags: string[] = [];
		if (args.insecureTlsMetricsServer) {
			flags.push("--insecure-tls-metrics-server");
		}
		if (args.installVendoredCalico) {
			flags.push("--install-vendored-calico");
		}

		const env: k8s.types.input.core.v1.EnvVar[] = [
			{ name: "KNATIVE_DOMAIN", value: args.knativeDomain },
		];
		// create-new-user.sh only mutates the in-pod kubeconfig; skip it when the
		// platform provisions admin identity outside the Job (EKS IAM / GKE IAM).
		if (args.clusterType === "EKS" || args.clusterType === "GKE") {
			env.push({ name: "CLUSTER_TYPE", value: "EKS" });
		}

		this.job = new k8s.batch.v1.Job(
			`${name}-job`,
			{
				metadata: {
					name: "install-korifi-dependencies",
					namespace: ns,
				},
				spec: {
					backoffLimit: 2,
					ttlSecondsAfterFinished: 86400,
					template: {
						spec: {
							serviceAccountName: this.serviceAccount.metadata.name,
							restartPolicy: "Never",
							containers: [
								{
									name: "install-dependencies",
									image:
										args.installerImage ?? versions.korifiInstallerImage,
									env,
									command: [
										"bash",
										"-c",
										`set -euo pipefail\nscripts/install-dependencies.sh ${flags.join(" ")}`.trimEnd(),
									],
								},
							],
						},
					},
				},
			},
			{
				...childOpts,
				dependsOn: [binding],
				customTimeouts: { create: "25m", update: "25m" },
			},
		);

		this.registerOutputs({ jobName: this.job.metadata.name });
	}
}
