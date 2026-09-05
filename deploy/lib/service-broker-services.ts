/**
 * Platform backends OSB offerings provision against.
 *
 * Postgres is not a shared cluster. OpenEverest (vcluster) installs the
 * operators; osb-service creates one DatabaseCluster per CF service instance.
 */
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import { EverestVcluster } from "./everest-vcluster";

/** Generic connection facts for a custom broker backend. */
export interface ServiceBrokerServiceConnection {
	host: string;
	port: number;
	adminUser: string;
	adminPassword: pulumi.Output<string>;
	adminUrl?: pulumi.Output<string>;
	resources: pulumi.Resource[];
}

/** In-cluster access to the Everest vcluster API. */
export interface EverestConnection {
	kubeconfig: pulumi.Output<string>;
	/** Namespace inside the vcluster where DatabaseClusters are created. */
	namespace: string;
	/** Host-cluster namespace that holds synced Services. */
	hostNamespace: string;
	vclusterName: string;
	resources: pulumi.Resource[];
}

export interface ServiceBrokerServicesArgs {
	provider: k8s.Provider;
	/** Kind cluster name; used to reach the Everest vcluster API. */
	kindClusterName: string;
	enable?: {
		postgres?: boolean;
	};
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

export class ServiceBrokerServices extends pulumi.ComponentResource {
	readonly everest?: EverestConnection;

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
			const everest = new EverestVcluster(
				`${name}-everest`,
				{
					provider: args.provider,
					kindClusterName: args.kindClusterName,
					dependsOn: args.dependsOn,
				},
				{ parent: this },
			);
			this.everest = {
				kubeconfig: everest.inClusterKubeconfig,
				namespace: everest.dbNamespace,
				hostNamespace: everest.namespace,
				vclusterName: everest.vclusterName,
				resources: [everest],
			};
		}

		this.registerOutputs({
			everestNamespace: this.everest?.namespace,
		});
	}
}

export function defaultServiceBrokerServiceEnable(): Required<
	Pick<NonNullable<ServiceBrokerServicesArgs["enable"]>, "postgres">
> {
	return { postgres: true };
}
