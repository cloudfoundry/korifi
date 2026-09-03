/**
 * In-cluster docker-registry for kind (INSTALL.kind.md / installer job).
 *
 * Credentials are pluggable: the same generated password feeds the chart
 * htpasswd and the `image-registry-credentials` pull secret in the CF root ns.
 */
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import * as random from "@pulumi/random";
import { kindRegistry } from "./values";
import { versions } from "./versions";

export interface LocalRegistryArgs {
	provider: k8s.Provider;
	username?: string;
	namespace?: pulumi.Input<string>;
	nodePort?: number;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

export class LocalRegistry extends pulumi.ComponentResource {
	readonly release: k8s.helm.v3.Release;
	readonly username: string;
	readonly password: pulumi.Output<string>;
	readonly clusterHost: string;
	readonly dockerConfigJson: pulumi.Output<string>;

	constructor(
		name: string,
		args: LocalRegistryArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:LocalRegistry", name, {}, opts);

		this.username = args.username ?? "user";
		this.clusterHost = kindRegistry.clusterHost;
		const nodePort = args.nodePort ?? kindRegistry.nodePort;

		const password = new random.RandomPassword(
			`${name}-password`,
			{ length: 24, special: false },
			{ parent: this },
		);
		this.password = password.result;

		this.release = new k8s.helm.v3.Release(
			`${name}-release`,
			{
				name: "localregistry",
				chart: "docker-registry",
				version: versions.registryChart,
				repositoryOpts: {
					repo: "https://twuni.github.io/docker-registry.helm",
				},
				namespace: args.namespace ?? "default",
				values: {
					service: { type: "NodePort", nodePort, port: nodePort },
					persistence: { enabled: true, deleteEnabled: true },
					secrets: {
						htpasswd: pulumi.interpolate`${this.username}:${password.bcryptHash}`,
					},
				},
			},
			{
				parent: this,
				provider: args.provider,
				dependsOn: args.dependsOn,
			},
		);

		this.dockerConfigJson = password.result.apply((pw) =>
			JSON.stringify({
				auths: {
					[this.clusterHost]: {
						username: this.username,
						password: pw,
						auth: Buffer.from(`${this.username}:${pw}`).toString("base64"),
					},
				},
			}),
		);

		this.registerOutputs({
			clusterHost: this.clusterHost,
			username: this.username,
		});
	}

	/** Standard `image-registry-credentials` secret Korifi/Kpack expect. */
	pullSecret(
		resourceName: string,
		namespace: pulumi.Input<string>,
		opts?: pulumi.CustomResourceOptions,
	): k8s.core.v1.Secret {
		return new k8s.core.v1.Secret(
			resourceName,
			{
				metadata: {
					name: "image-registry-credentials",
					namespace,
				},
				type: "kubernetes.io/dockerconfigjson",
				stringData: { ".dockerconfigjson": this.dockerConfigJson },
			},
			{
				parent: this,
				provider: opts?.provider,
				...opts,
			},
		);
	}
}

/**
 * Create a docker-registry pull secret from explicit credentials
 * (GKE Artifact Registry, DockerHub, etc.).
 */
export function registryPullSecret(
	name: string,
	args: {
		provider: k8s.Provider;
		namespace: pulumi.Input<string>;
		server: pulumi.Input<string>;
		username: pulumi.Input<string>;
		password: pulumi.Input<string>;
		secretName?: string;
	},
	opts?: pulumi.ComponentResourceOptions,
): k8s.core.v1.Secret {
	const dockerConfigJson = pulumi
		.all([args.server, args.username, args.password])
		.apply(([server, username, password]) =>
			JSON.stringify({
				auths: {
					[server]: {
						username,
						password,
						auth: Buffer.from(`${username}:${password}`).toString("base64"),
					},
				},
			}),
		);

	return new k8s.core.v1.Secret(
		name,
		{
			metadata: {
				name: args.secretName ?? "image-registry-credentials",
				namespace: args.namespace,
			},
			type: "kubernetes.io/dockerconfigjson",
			stringData: { ".dockerconfigjson": dockerConfigJson },
		},
		{ provider: args.provider, ...opts },
	);
}
