/**
 * Knative Operator + Serving (Kourier ClusterIP) as first-class Pulumi
 * resources — the scale-to-zero runtime under CF routes.
 *
 * Contour remains north-south ingress on :80/:443. This layer does not
 * expose Knative as a developer API; Korifi's knative-runner reconciles
 * AppWorkloads onto Knative Services.
 *
 * The KorifiDependencies Job uses the release installer image, which (as of
 * 0.18.0) does not install Knative. Stacks must compose this component so the
 * operator shows up in `pulumi up` and in cluster state.
 */
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import { versions } from "./versions";

export interface KnativeServingArgs {
	provider: k8s.Provider;
	/** Must match Korifi `defaultAppDomainName`. */
	domain: string;
	operatorChartVersion?: string;
	servingVersion?: string;
	operatorNamespace?: string;
	servingNamespace?: string;
	/**
	 * When set, also install knative-runner ClusterRole/Binding + RunnerInfo
	 * (the 0.18.0 Helm chart has neither). Requires Korifi already installed.
	 */
	korifiNamespace?: pulumi.Input<string>;
	rootNamespace?: pulumi.Input<string>;
	/**
	 * Install knative-runner ClusterRole/Binding + RunnerInfo. Default true
	 * (needed with the 0.18.0 chart). Set false when the Helm chart already
	 * includes `knativeRunner` templates to avoid duplicate names.
	 */
	installRunnerSupport?: boolean;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

export class KnativeServing extends pulumi.ComponentResource {
	readonly operator: k8s.helm.v3.Release;
	readonly serving: k8s.apiextensions.CustomResource;
	readonly ready: k8s.batch.v1.Job;

	constructor(
		name: string,
		args: KnativeServingArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:KnativeServing", name, {}, opts);

		const operatorNsName = args.operatorNamespace ?? "knative-operator";
		const servingNsName = args.servingNamespace ?? "knative-serving";
		const servingVersion = args.servingVersion ?? versions.knativeServing;
		const chartVersion =
			args.operatorChartVersion ?? versions.knativeOperatorChart;
		const korifiNs = args.korifiNamespace ?? "korifi";
		const rootNs = args.rootNamespace ?? "cf";

		const childOpts: pulumi.CustomResourceOptions = {
			parent: this,
			provider: args.provider,
			dependsOn: args.dependsOn,
		};

		const operatorNs = new k8s.core.v1.Namespace(
			`${name}-operator-ns`,
			{ metadata: { name: operatorNsName } },
			childOpts,
		);

		this.operator = new k8s.helm.v3.Release(
			`${name}-operator`,
			{
				name: "knative-operator",
				chart: "knative-operator",
				version: chartVersion,
				repositoryOpts: { repo: "https://knative.github.io/operator" },
				namespace: operatorNs.metadata.name,
				timeout: 600,
			},
			{
				...childOpts,
				dependsOn: [...(args.dependsOn ?? []), operatorNs],
				customTimeouts: { create: "15m", update: "15m" },
			},
		);

		const servingNs = new k8s.core.v1.Namespace(
			`${name}-serving-ns`,
			{ metadata: { name: servingNsName } },
			{
				...childOpts,
				dependsOn: [...(args.dependsOn ?? []), this.operator],
			},
		);

		this.serving = new k8s.apiextensions.CustomResource(
			`${name}-serving`,
			{
				apiVersion: "operator.knative.dev/v1beta1",
				kind: "KnativeServing",
				metadata: {
					name: "knative-serving",
					namespace: servingNs.metadata.name,
				},
				spec: {
					version: servingVersion,
					ingress: {
						kourier: {
							enabled: true,
							"service-type": "ClusterIP",
						},
					},
					config: {
						network: {
							"ingress-class":
								"kourier.ingress.networking.knative.dev",
						},
						domain: {
							[args.domain]: "",
						},
					},
				},
			},
			{
				...childOpts,
				dependsOn: [...(args.dependsOn ?? []), servingNs, this.operator],
				customTimeouts: { create: "20m", update: "20m" },
			},
		);

		const awaitSa = new k8s.core.v1.ServiceAccount(
			`${name}-await-sa`,
			{
				metadata: {
					name: "await-knative-serving",
					namespace: servingNs.metadata.name,
				},
			},
			{
				...childOpts,
				dependsOn: [...(args.dependsOn ?? []), servingNs],
			},
		);

		const awaitBinding = new k8s.rbac.v1.ClusterRoleBinding(
			`${name}-await-binding`,
			{
				metadata: { name: `${name}-await-knative-serving` },
				roleRef: {
					apiGroup: "rbac.authorization.k8s.io",
					kind: "ClusterRole",
					name: "cluster-admin",
				},
				subjects: [
					{
						kind: "ServiceAccount",
						name: awaitSa.metadata.name,
						namespace: servingNs.metadata.name,
					},
				],
			},
			{
				...childOpts,
				dependsOn: [...(args.dependsOn ?? []), awaitSa],
			},
		);

		this.ready = new k8s.batch.v1.Job(
			`${name}-await`,
			{
				metadata: {
					name: "await-knative-serving",
					namespace: servingNs.metadata.name,
				},
				spec: {
					backoffLimit: 2,
					ttlSecondsAfterFinished: 86400,
					template: {
						spec: {
							serviceAccountName: awaitSa.metadata.name,
							restartPolicy: "Never",
							containers: [
								{
									name: "wait",
									image: "alpine/k8s:1.36.4",
									command: [
										"bash",
										"-c",
										[
											"set -euo pipefail",
											"kubectl wait --for=condition=Ready knativeserving.operator.knative.dev/knative-serving -n knative-serving --timeout=15m",
											"kubectl get knativeserving.operator.knative.dev knative-serving -n knative-serving",
										].join("\n"),
									],
								},
							],
						},
					},
				},
			},
			{
				...childOpts,
				dependsOn: [
					...(args.dependsOn ?? []),
					this.serving,
					awaitBinding,
				],
				customTimeouts: { create: "20m", update: "20m" },
			},
		);

		if (args.installRunnerSupport === false) {
			this.registerOutputs({
				operatorRelease: this.operator.name,
				servingName: this.serving.metadata.name,
			});
			return;
		}

		const runnerRole = new k8s.rbac.v1.ClusterRole(
			`${name}-runner-role`,
			{
				metadata: { name: "korifi-knative-runner-appworkload-manager-role" },
				rules: [
					{
						apiGroups: ["serving.knative.dev"],
						resources: [
							"services",
							"services/status",
							"revisions",
							"configurations",
						],
						verbs: [
							"get",
							"list",
							"watch",
							"create",
							"patch",
							"update",
							"delete",
							"deletecollection",
						],
					},
					{
						apiGroups: [""],
						resources: ["pods"],
						verbs: ["get", "list", "watch"],
					},
					{
						apiGroups: ["apps"],
						resources: ["statefulsets"],
						verbs: ["delete", "get", "list", "watch"],
					},
					{
						apiGroups: ["korifi.cloudfoundry.org"],
						resources: ["appworkloads", "runnerinfos"],
						verbs: [
							"create",
							"delete",
							"get",
							"list",
							"patch",
							"watch",
						],
					},
					{
						apiGroups: ["korifi.cloudfoundry.org"],
						resources: ["appworkloads/status", "runnerinfos/status"],
						verbs: ["get", "patch"],
					},
				],
			},
			{
				...childOpts,
				dependsOn: [...(args.dependsOn ?? []), this.ready],
			},
		);

		new k8s.rbac.v1.ClusterRoleBinding(
			`${name}-runner-binding`,
			{
				metadata: { name: "korifi-knative-runner-manager-rolebinding" },
				roleRef: {
					apiGroup: "rbac.authorization.k8s.io",
					kind: "ClusterRole",
					name: runnerRole.metadata.name,
				},
				subjects: [
					{
						kind: "ServiceAccount",
						name: "korifi-controllers-controller-manager",
						namespace: korifiNs,
					},
				],
			},
			{
				...childOpts,
				dependsOn: [...(args.dependsOn ?? []), runnerRole],
			},
		);

		new k8s.apiextensions.CustomResource(
			`${name}-runner-info`,
			{
				apiVersion: "korifi.cloudfoundry.org/v1alpha1",
				kind: "RunnerInfo",
				metadata: { name: "knative-runner", namespace: rootNs },
				spec: { runnerName: "knative-runner" },
			},
			{
				...childOpts,
				dependsOn: [...(args.dependsOn ?? []), this.ready],
			},
		);

		this.registerOutputs({
			operatorRelease: this.operator.name,
			servingName: this.serving.metadata.name,
		});
	}
}
