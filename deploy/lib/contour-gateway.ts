/**
 * Contour GatewayClass + ContourDeployment publishing params.
 *
 * install-dependencies.sh already creates GatewayClass "contour" without
 * parametersRef. Kind needs NodePort publishing; EKS/GKE use the default
 * LoadBalancer Envoy service (dynamic Contour provisioner).
 */
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";

export type ContourPublishType = "NodePortService" | "LoadBalancerService";

export interface ContourGatewayArgs {
	provider: k8s.Provider;
	gatewayClassName?: string;
	publishType: ContourPublishType;
	/** Namespace that owns the ContourDeployment params object. */
	contourNamespace?: string;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

export class ContourGateway extends pulumi.ComponentResource {
	readonly gatewayClass: k8s.apiextensions.CustomResourcePatch;
	readonly contourParams?: k8s.apiextensions.CustomResource;

	constructor(
		name: string,
		args: ContourGatewayArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:ContourGateway", name, {}, opts);

		const className = args.gatewayClassName ?? "contour";
		const contourNs = args.contourNamespace ?? "projectcontour";
		const childOpts: pulumi.CustomResourceOptions = {
			parent: this,
			provider: args.provider,
			dependsOn: args.dependsOn,
		};

		let parametersRef:
			| {
					kind: string;
					group: string;
					name: string;
					namespace: string;
			  }
			| undefined;

		if (args.publishType === "NodePortService") {
			this.contourParams = new k8s.apiextensions.CustomResource(
				`${name}-params`,
				{
					apiVersion: "projectcontour.io/v1alpha1",
					kind: "ContourDeployment",
					metadata: {
						name: "contour-nodeport-params",
						namespace: contourNs,
					},
					spec: {
						envoy: {
							networkPublishing: { type: "NodePortService" },
						},
					},
				},
				childOpts,
			);
			parametersRef = {
				kind: "ContourDeployment",
				group: "projectcontour.io",
				name: "contour-nodeport-params",
				namespace: contourNs,
			};
		}

		// Patch the GatewayClass created by install-dependencies.sh.
		this.gatewayClass = new k8s.apiextensions.CustomResourcePatch(
			`${name}-gateway-class`,
			{
				apiVersion: "gateway.networking.k8s.io/v1beta1",
				kind: "GatewayClass",
				metadata: {
					name: className,
					annotations: { "pulumi.com/patchForce": "true" },
				},
				spec: {
					controllerName: "projectcontour.io/gateway-controller",
					...(parametersRef ? { parametersRef } : {}),
				},
			},
			{
				...childOpts,
				dependsOn: this.contourParams
					? [...(args.dependsOn ?? []), this.contourParams]
					: args.dependsOn,
			},
		);

		this.registerOutputs({ gatewayClassName: className });
	}
}
