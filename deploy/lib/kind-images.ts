/**
 * Build Korifi images from this checkout and `kind load` them.
 *
 * Hub `cloudfoundry/korifi-*:latest` is the last release tarball (no
 * knative-runner). Kind `pulumi up` must ship the tree that the local Helm
 * chart expects, including scale-to-zero route wiring.
 */
import * as command from "@pulumi/command";
import * as pulumi from "@pulumi/pulumi";
import { hashKorifiImageSources } from "./image-source-hash";

export interface KindKorifiImagesArgs {
	clusterName: string;
	/** Korifi repo root (directory that contains `controllers/Dockerfile`). */
	repoRoot: string;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

export class KindKorifiImages extends pulumi.ComponentResource {
	readonly fingerprint: string;
	readonly tag: string;
	readonly controllersImage: string;
	readonly apiImage: string;
	readonly migrationImage: string;
	readonly loaded: command.local.Command;

	constructor(
		name: string,
		args: KindKorifiImagesArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:KindKorifiImages", name, {}, opts);

		this.fingerprint = hashKorifiImageSources(args.repoRoot);
		this.tag = `kind-${this.fingerprint.slice(0, 12)}`;
		this.controllersImage = `korifi-controllers:${this.tag}`;
		this.apiImage = `korifi-api:${this.tag}`;
		this.migrationImage = `korifi-migration:${this.tag}`;

		const script = [
			`set -euo pipefail`,
			`cd "$KORIFI_ROOT"`,
			`docker build -f controllers/Dockerfile -t "$CONTROLLERS_IMAGE" --build-arg "version=$VERSION" .`,
			`docker build -f api/Dockerfile -t "$API_IMAGE" --build-arg "version=$VERSION" .`,
			`docker build -f migration/Dockerfile -t "$MIGRATION_IMAGE" --build-arg "version=$VERSION" .`,
			`kind load docker-image "$CONTROLLERS_IMAGE" --name "$KIND_CLUSTER"`,
			`kind load docker-image "$API_IMAGE" --name "$KIND_CLUSTER"`,
			`kind load docker-image "$MIGRATION_IMAGE" --name "$KIND_CLUSTER"`,
			`echo "loaded $CONTROLLERS_IMAGE $API_IMAGE $MIGRATION_IMAGE"`,
		].join("\n");

		this.loaded = new command.local.Command(
			`${name}-build-load`,
			{
				create: script,
				update: script,
				triggers: [this.fingerprint, args.clusterName, args.repoRoot],
				environment: {
					DOCKER_BUILDKIT: "1",
					KORIFI_ROOT: args.repoRoot,
					KIND_CLUSTER: args.clusterName,
					VERSION: this.tag,
					CONTROLLERS_IMAGE: this.controllersImage,
					API_IMAGE: this.apiImage,
					MIGRATION_IMAGE: this.migrationImage,
				},
				logging: command.local.Logging.Stderr,
			},
			{
				parent: this,
				dependsOn: args.dependsOn,
				customTimeouts: { create: "25m", update: "25m" },
			},
		);

		this.registerOutputs({
			fingerprint: this.fingerprint,
			controllersImage: this.controllersImage,
			apiImage: this.apiImage,
			migrationImage: this.migrationImage,
		});
	}
}
