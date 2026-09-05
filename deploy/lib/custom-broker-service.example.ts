/**
 * EXAMPLE — adding a custom OSB broker backend.
 *
 * This module is **not** imported by deploy stacks. It is a copy-paste /
 * import-ready template that mirrors Postgres in `service-broker-services.ts`.
 *
 * To ship a real service:
 *
 *   1. Rename `example` → your service id (`nats`, `minio`, …)
 *   2. Adjust image / ports / chart values below
 *   3. In `service-broker-services.ts`:
 *        - add `enable.example?: boolean` (+ `example?: ExampleServiceArgs`)
 *        - call `installExampleService` when enabled and assign `this.example`
 *   4. In each stack: `enable: { postgres: true, example: true }` and pass
 *      the connection on `OsbServiceBroker` `backends` (same as `postgres`)
 *
 * Prefer a Helm `Release` when an upstream chart exists (Postgres is an
 * OpenEverest `DatabaseCluster` in a vcluster); use a Deployment /
 * StatefulSet when it does not.
 */
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import * as random from "@pulumi/random";
import type { ServiceBrokerServiceConnection } from "./service-broker-services";

export interface ExampleServiceArgs {
	/** Kubernetes namespace (and DNS label). Default `example`. */
	namespace?: string;
	/** Container image for the shared backend. */
	image?: string;
	/** Service port apps / brokers dial. Default 8080. */
	port?: number;
	adminUser?: string;
}

/**
 * Minimal Deployment + Service backend. Returns the same connection shape
 * brokers already expect from Postgres — host, port, admin credentials,
 * and `resources` for `dependsOn`.
 */
export function installExampleService(
	name: string,
	args: {
		provider: k8s.Provider;
		parent: pulumi.Resource;
		dependsOn?: pulumi.Input<pulumi.Resource>[];
		opts?: ExampleServiceArgs;
	},
): ServiceBrokerServiceConnection {
	const opts = args.opts ?? {};
	const nsName = opts.namespace ?? "example";
	const adminUser = opts.adminUser ?? "example";
	const image = opts.image ?? "example/example:latest";
	const port = opts.port ?? 8080;
	const child = {
		parent: args.parent,
		provider: args.provider,
		dependsOn: args.dependsOn,
	};

	const ns = new k8s.core.v1.Namespace(
		`${name}-ns`,
		{ metadata: { name: nsName } },
		child,
	);

	const password = new random.RandomPassword(
		`${name}-admin-password`,
		{ length: 24, special: false },
		{ parent: args.parent },
	);

	const secret = new k8s.core.v1.Secret(
		`${name}-credentials`,
		{
			metadata: { name: `${nsName}-credentials`, namespace: ns.metadata.name },
			stringData: {
				ADMIN_USER: adminUser,
				ADMIN_PASSWORD: password.result,
			},
		},
		{ ...child, dependsOn: [ns] },
	);

	const deployment = new k8s.apps.v1.Deployment(
		`${name}-deploy`,
		{
			metadata: { name: nsName, namespace: ns.metadata.name },
			spec: {
				replicas: 1,
				selector: { matchLabels: { app: nsName } },
				template: {
					metadata: { labels: { app: nsName } },
					spec: {
						containers: [
							{
								name: nsName,
								image,
								ports: [{ containerPort: port, name: "api" }],
								envFrom: [{ secretRef: { name: secret.metadata.name } }],
								readinessProbe: {
									tcpSocket: { port },
									initialDelaySeconds: 5,
									periodSeconds: 5,
								},
								resources: {
									requests: { cpu: "50m", memory: "64Mi" },
									limits: { memory: "256Mi" },
								},
							},
						],
					},
				},
			},
		},
		{ ...child, dependsOn: [ns, secret] },
	);

	const service = new k8s.core.v1.Service(
		`${name}-svc`,
		{
			metadata: { name: nsName, namespace: ns.metadata.name },
			spec: {
				type: "ClusterIP",
				selector: { app: nsName },
				ports: [{ name: "api", port, targetPort: port }],
			},
		},
		{ ...child, dependsOn: [ns] },
	);

	const host = `${nsName}.${nsName}.svc.cluster.local`;

	return {
		host,
		port,
		adminUser,
		adminPassword: password.result,
		resources: [deployment, service],
	};
}

/**
 * EXAMPLE — Helm-based backend (preferred when a chart exists).
 *
 * Swap chart / repo / values for NATS, MinIO, etc. Credentials stay
 * Pulumi-generated and flow into the returned connection facts.
 */
export function installExampleHelmService(
	name: string,
	args: {
		provider: k8s.Provider;
		parent: pulumi.Resource;
		dependsOn?: pulumi.Input<pulumi.Resource>[];
		opts?: ExampleServiceArgs & {
			chart?: string;
			chartVersion?: string;
			repository?: string;
		};
	},
): ServiceBrokerServiceConnection {
	const opts = args.opts ?? {};
	const nsName = opts.namespace ?? "example";
	const adminUser = opts.adminUser ?? "example";
	const port = opts.port ?? 8080;
	const child = {
		parent: args.parent,
		provider: args.provider,
		dependsOn: args.dependsOn,
	};

	const ns = new k8s.core.v1.Namespace(
		`${name}-ns`,
		{ metadata: { name: nsName } },
		child,
	);

	const password = new random.RandomPassword(
		`${name}-admin-password`,
		{ length: 24, special: false },
		{ parent: args.parent },
	);

	const release = new k8s.helm.v3.Release(
		`${name}-release`,
		{
			name: nsName,
			chart: opts.chart ?? "example",
			version: opts.chartVersion ?? "1.0.0",
			repositoryOpts: {
				repo: opts.repository ?? "https://example.invalid/charts",
			},
			namespace: ns.metadata.name,
			values: {
				auth: {
					username: adminUser,
					password: password.result,
				},
				service: { port },
			},
		},
		{ ...child, dependsOn: [ns] },
	);

	return {
		host: `${nsName}.${nsName}.svc.cluster.local`,
		port,
		adminUser,
		adminPassword: password.result,
		resources: [release],
	};
}

/**
 * EXAMPLE — wiring snippet for `ServiceBrokerServices` (do not paste blindly;
 * merge into the real class / Args types).
 *
 * ```ts
 * export interface ServiceBrokerServicesArgs {
 *   enable?: { postgres?: boolean; example?: boolean };
 *   example?: ExampleServiceArgs;
 *   // ...
 * }
 *
 * // inside the constructor:
 * if (enable.example) {
 *   this.example = installExampleService(`${name}-example`, {
 *     provider: args.provider,
 *     parent: this,
 *     dependsOn: args.dependsOn,
 *     opts: args.example,
 *   });
 * }
 *
 * // kind stack:
 * const brokerServices = new ServiceBrokerServices("broker-services", {
 *   provider: cluster.provider,
 *   enable: { postgres: true, example: true },
 * });
 * export const example = brokerServices.example && {
 *   host: brokerServices.example.host,
 *   port: brokerServices.example.port,
 *   adminUser: brokerServices.example.adminUser,
 *   adminPassword: pulumi.secret(brokerServices.example.adminPassword),
 * };
 * ```
 */
export const exampleWiringDocs = true;
