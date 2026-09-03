/**
 * Korifi system namespaces (INSTALL.md § Namespace creation).
 */
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";

const restricted = {
	"pod-security.kubernetes.io/audit": "restricted",
	"pod-security.kubernetes.io/enforce": "restricted",
};

export interface KorifiNamespacesArgs {
	provider: k8s.Provider;
	rootNamespace?: string;
	korifiNamespace?: string;
	gatewayNamespace?: string;
	/** When true, also create korifi-installer for the dependencies Job. */
	installerNamespace?: boolean;
}

export class KorifiNamespaces extends pulumi.ComponentResource {
	readonly root: k8s.core.v1.Namespace;
	readonly korifi: k8s.core.v1.Namespace;
	readonly gateway: k8s.core.v1.Namespace;
	readonly installer?: k8s.core.v1.Namespace;

	readonly rootName: string;
	readonly korifiName: string;
	readonly gatewayName: string;

	constructor(
		name: string,
		args: KorifiNamespacesArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:Namespaces", name, {}, opts);

		const childOpts: pulumi.CustomResourceOptions = {
			parent: this,
			provider: args.provider,
		};

		this.rootName = args.rootNamespace ?? "cf";
		this.korifiName = args.korifiNamespace ?? "korifi";
		this.gatewayName = args.gatewayNamespace ?? "korifi-gateway";

		this.root = new k8s.core.v1.Namespace(
			`${name}-root`,
			{ metadata: { name: this.rootName, labels: restricted } },
			childOpts,
		);
		this.korifi = new k8s.core.v1.Namespace(
			`${name}-korifi`,
			{ metadata: { name: this.korifiName, labels: restricted } },
			childOpts,
		);
		this.gateway = new k8s.core.v1.Namespace(
			`${name}-gateway`,
			{ metadata: { name: this.gatewayName } },
			childOpts,
		);

		if (args.installerNamespace) {
			this.installer = new k8s.core.v1.Namespace(
				`${name}-installer`,
				{ metadata: { name: "korifi-installer" } },
				childOpts,
			);
		}

		this.registerOutputs({
			rootName: this.rootName,
			korifiName: this.korifiName,
			gatewayName: this.gatewayName,
		});
	}
}
