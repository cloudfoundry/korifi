/**
 * Build the in-tree `osb-service` broker image and `kind load` it.
 */
import * as fs from "node:fs";
import * as command from "@pulumi/command";
import * as pulumi from "@pulumi/pulumi";
import { hashOsbBrokerImageSources } from "./image-source-hash";

export interface KindOsbBrokerImageArgs {
	clusterName: string;
	/** Directory that contains `image/Dockerfile` (`osb-service` by default). */
	sourcePath: string;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

export class KindOsbBrokerImage extends pulumi.ComponentResource {
	readonly fingerprint: string;
	readonly tag: string;
	readonly image: string;
	readonly loaded: command.local.Command;

	constructor(
		name: string,
		args: KindOsbBrokerImageArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:KindOsbBrokerImage", name, {}, opts);

		if (!fs.existsSync(args.sourcePath)) {
			throw new Error(
				`osb-service sources not found at ${args.sourcePath}`,
			);
		}
		const dockerfile = `${args.sourcePath}/image/Dockerfile`;
		if (!fs.existsSync(dockerfile)) {
			throw new Error(`osb-service Dockerfile missing: ${dockerfile}`);
		}

		this.fingerprint = hashOsbBrokerImageSources(args.sourcePath);
		this.tag = `kind-${this.fingerprint.slice(0, 12)}`;
		this.image = `osb-service:${this.tag}`;

		const script = [
			`set -euo pipefail`,
			`cd "$OSB_ROOT"`,
			`docker build -f image/Dockerfile -t "$BROKER_IMAGE" .`,
			`kind load docker-image "$BROKER_IMAGE" --name "$KIND_CLUSTER"`,
			`echo "loaded $BROKER_IMAGE"`,
		].join("\n");

		this.loaded = new command.local.Command(
			`${name}-build-load`,
			{
				create: script,
				update: script,
				triggers: [this.fingerprint, args.clusterName, args.sourcePath],
				environment: {
					DOCKER_BUILDKIT: "1",
					OSB_ROOT: args.sourcePath,
					KIND_CLUSTER: args.clusterName,
					BROKER_IMAGE: this.image,
				},
				logging: command.local.Logging.Stderr,
			},
			{
				parent: this,
				dependsOn: args.dependsOn,
				customTimeouts: { create: "15m", update: "15m" },
			},
		);

		this.registerOutputs({
			fingerprint: this.fingerprint,
			image: this.image,
		});
	}
}
