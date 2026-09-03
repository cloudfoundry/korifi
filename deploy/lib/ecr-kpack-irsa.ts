/**
 * Annotate the kpack controller ServiceAccount with an EKS IRSA role ARN
 * (INSTALL.EKS.md § Dependencies) and restart the controller.
 */
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";

export interface EcrKpackIrsaArgs {
	provider: k8s.Provider;
	roleArn: pulumi.Input<string>;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

export class EcrKpackIrsa extends pulumi.ComponentResource {
	readonly serviceAccount: k8s.core.v1.ServiceAccountPatch;
	readonly restart: k8s.batch.v1.Job;

	constructor(
		name: string,
		args: EcrKpackIrsaArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:EcrKpackIrsa", name, {}, opts);

		const childOpts: pulumi.CustomResourceOptions = {
			parent: this,
			provider: args.provider,
			dependsOn: args.dependsOn,
		};

		this.serviceAccount = new k8s.core.v1.ServiceAccountPatch(
			`${name}-sa`,
			{
				metadata: {
					name: "controller",
					namespace: "kpack",
					annotations: {
						"pulumi.com/patchForce": "true",
						"eks.amazonaws.com/role-arn": args.roleArn,
					},
				},
			},
			childOpts,
		);

		const sa = new k8s.core.v1.ServiceAccount(
			`${name}-restart-sa`,
			{
				metadata: {
					name: "restart-kpack-controller",
					namespace: "kpack",
				},
			},
			childOpts,
		);

		const binding = new k8s.rbac.v1.RoleBinding(
			`${name}-restart-binding`,
			{
				metadata: {
					name: "restart-kpack-controller",
					namespace: "kpack",
				},
				roleRef: {
					apiGroup: "rbac.authorization.k8s.io",
					kind: "ClusterRole",
					name: "edit",
				},
				subjects: [
					{
						kind: "ServiceAccount",
						name: sa.metadata.name,
						namespace: "kpack",
					},
				],
			},
			{ ...childOpts, dependsOn: [sa] },
		);

		this.restart = new k8s.batch.v1.Job(
			`${name}-restart`,
			{
				metadata: {
					name: "restart-kpack-controller",
					namespace: "kpack",
				},
				spec: {
					backoffLimit: 1,
					ttlSecondsAfterFinished: 600,
					template: {
						spec: {
							serviceAccountName: sa.metadata.name,
							restartPolicy: "Never",
							containers: [
								{
									name: "restart",
									image: "alpine/k8s:1.36.4",
									command: [
										"bash",
										"-c",
										"kubectl -n kpack rollout restart deployment kpack-controller",
									],
								},
							],
						},
					},
				},
			},
			{
				...childOpts,
				dependsOn: [this.serviceAccount, binding],
				customTimeouts: { create: "5m", update: "5m" },
			},
		);

		this.registerOutputs({});
	}
}
