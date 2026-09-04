/**
 * Shared backends that OSB service brokers provision against.
 *
 * Each enabled service installs into its own namespace, generates admin
 * credentials, and exports connection facts brokers consume — nothing is
 * copied by hand between YAML files.
 *
 * Postgres ships first (hand-rolled StatefulSet; no dependable upstream
 * chart since the Bitnami catalog lock-down). To add another service, follow
 * the worked template in `custom-broker-service.example.ts`:
 *
 *   1. Write `installFoo(...)` returning a `ServiceBrokerServiceConnection`
 *   2. Add `enable.foo?: boolean` (+ optional `foo?: FooArgs`) on Args
 *   3. Assign `this.foo` when enabled
 *   4. Wire the export into stacks / brokers the same way as postgres
 */
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import * as random from "@pulumi/random";
import { versions } from "./versions";

/** Connection + credential facts a service broker needs. */
export interface ServiceBrokerServiceConnection {
	/** In-cluster DNS hostname (e.g. postgres.postgres.svc.cluster.local). */
	host: string;
	port: number;
	adminUser: string;
	adminPassword: pulumi.Output<string>;
	/** URI form when the service has one (postgres://…). */
	adminUrl?: pulumi.Output<string>;
	/** Resources brokers / stacks should `dependsOn`. */
	resources: pulumi.Resource[];
}

export interface PostgresServiceArgs {
	/** PVC size for PGDATA. Default 2Gi. */
	storage?: string;
	/** Container image. Default pinned in versions.ts. */
	image?: string;
	adminUser?: string;
	database?: string;
}

export interface ServiceBrokerServicesArgs {
	provider: k8s.Provider;
	/**
	 * Which services to install. Defaults to `{ postgres: true }`.
	 * Flip flags (and add installers) to grow the set without reshaping stacks.
	 */
	enable?: {
		postgres?: boolean;
		// nats?: boolean;
		// memgraph?: boolean;
		// minio?: boolean;
	};
	postgres?: PostgresServiceArgs;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

function randomPassword(
	name: string,
	parent: pulumi.Resource,
): random.RandomPassword {
	return new random.RandomPassword(
		name,
		{ length: 24, special: false },
		{ parent },
	);
}

function serviceNamespace(
	name: string,
	nsName: string,
	provider: k8s.Provider,
	parent: pulumi.Resource,
	dependsOn?: pulumi.Input<pulumi.Resource>[],
): k8s.core.v1.Namespace {
	return new k8s.core.v1.Namespace(
		name,
		{ metadata: { name: nsName } },
		{ parent, provider, dependsOn },
	);
}

/**
 * Shared single-node Postgres. A postgres OSB broker creates a database +
 * role per service instance using the admin credentials exported here.
 */
function installPostgres(
	name: string,
	args: {
		provider: k8s.Provider;
		parent: pulumi.Resource;
		dependsOn?: pulumi.Input<pulumi.Resource>[];
		opts?: PostgresServiceArgs;
	},
): ServiceBrokerServiceConnection {
	const opts = args.opts ?? {};
	const adminUser = opts.adminUser ?? "postgres";
	const database = opts.database ?? "postgres";
	const image = opts.image ?? versions.postgresImage;
	const storage = opts.storage ?? "2Gi";
	const nsName = "postgres";
	const child = {
		parent: args.parent,
		provider: args.provider,
		dependsOn: args.dependsOn,
	};

	const ns = serviceNamespace(
		`${name}-ns`,
		nsName,
		args.provider,
		args.parent,
		args.dependsOn,
	);
	const password = randomPassword(`${name}-admin-password`, args.parent);

	const secret = new k8s.core.v1.Secret(
		`${name}-credentials`,
		{
			metadata: { name: "postgres-credentials", namespace: ns.metadata.name },
			stringData: {
				POSTGRES_USER: adminUser,
				POSTGRES_PASSWORD: password.result,
				POSTGRES_DB: database,
			},
		},
		{ ...child, dependsOn: [ns] },
	);

	const pvc = new k8s.core.v1.PersistentVolumeClaim(
		`${name}-data`,
		{
			metadata: { name: "postgres-data", namespace: ns.metadata.name },
			spec: {
				accessModes: ["ReadWriteOnce"],
				resources: { requests: { storage } },
			},
		},
		{ ...child, dependsOn: [ns] },
	);

	const sts = new k8s.apps.v1.StatefulSet(
		`${name}-sts`,
		{
			metadata: { name: "postgres", namespace: ns.metadata.name },
			spec: {
				serviceName: "postgres",
				replicas: 1,
				selector: { matchLabels: { app: "postgres" } },
				template: {
					metadata: { labels: { app: "postgres" } },
					spec: {
						containers: [
							{
								name: "postgres",
								image,
								ports: [{ containerPort: 5432, name: "postgres" }],
								envFrom: [{ secretRef: { name: secret.metadata.name } }],
								env: [
									{
										name: "PGDATA",
										value: "/var/lib/postgresql/data/pgdata",
									},
								],
								volumeMounts: [
									{
										name: "data",
										mountPath: "/var/lib/postgresql/data",
									},
								],
								readinessProbe: {
									exec: {
										command: ["pg_isready", "-U", adminUser],
									},
									initialDelaySeconds: 5,
									periodSeconds: 5,
								},
								resources: {
									requests: { cpu: "100m", memory: "256Mi" },
									limits: { memory: "512Mi" },
								},
							},
						],
						volumes: [
							{
								name: "data",
								persistentVolumeClaim: { claimName: pvc.metadata.name },
							},
						],
					},
				},
			},
		},
		{ ...child, dependsOn: [ns, secret, pvc] },
	);

	const service = new k8s.core.v1.Service(
		`${name}-svc`,
		{
			metadata: { name: "postgres", namespace: ns.metadata.name },
			spec: {
				type: "ClusterIP",
				selector: { app: "postgres" },
				ports: [{ name: "postgres", port: 5432, targetPort: 5432 }],
			},
		},
		{ ...child, dependsOn: [ns] },
	);

	const host = `postgres.${nsName}.svc.cluster.local`;
	const port = 5432;

	return {
		host,
		port,
		adminUser,
		adminPassword: password.result,
		adminUrl: pulumi.interpolate`postgres://${adminUser}:${password.result}@${host}:${port}/${database}`,
		resources: [sts, service],
	};
}

/**
 * Installs the enabled broker backends and exposes connection facts.
 * Stacks compose this once; brokers take `this.postgres` (etc.) as env —
 * the seam where "workload values" become "broker env".
 */
export class ServiceBrokerServices extends pulumi.ComponentResource {
	readonly postgres?: ServiceBrokerServiceConnection;

	constructor(
		name: string,
		args: ServiceBrokerServicesArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:ServiceBrokerServices", name, {}, opts);

		const enable = {
			postgres: true,
			...args.enable,
		};

		if (enable.postgres) {
			this.postgres = installPostgres(`${name}-postgres`, {
				provider: args.provider,
				parent: this,
				dependsOn: args.dependsOn,
				opts: args.postgres,
			});
		}

		this.registerOutputs({
			postgresHost: this.postgres?.host,
		});
	}
}

/** Default enable map — useful in tests and docs. */
export function defaultServiceBrokerServiceEnable(): Required<
	Pick<NonNullable<ServiceBrokerServicesArgs["enable"]>, "postgres">
> {
	return { postgres: true };
}
