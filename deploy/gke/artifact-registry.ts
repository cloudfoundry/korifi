/**
 * Artifact Registry + registry credentials for Korifi/Kpack (INSTALL.md
 * § Container registry credentials — Google Artifact Registry).
 */
import * as gcp from "@pulumi/gcp";
import * as pulumi from "@pulumi/pulumi";
import {
	gkeKpackBuilderRepository,
	gkeRepositoryPrefix,
} from "@korifi/deploy-lib";

export interface GkeRegistryArgs {
	project: string;
	/** Artifact Registry location, e.g. `us` or `europe`. */
	location: string;
	repositoryId: string;
}

export class GkeRegistry extends pulumi.ComponentResource {
	readonly repository: gcp.artifactregistry.Repository;
	readonly repositoryPrefix: string;
	readonly kpackBuilderRepository: string;
	readonly dockerServer: string;
	/** JSON key for `_json_key` docker auth (secret). */
	readonly serviceAccountKey: pulumi.Output<string>;
	readonly serviceAccountEmail: pulumi.Output<string>;

	constructor(
		name: string,
		args: GkeRegistryArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:GkeRegistry", name, {}, opts);

		this.dockerServer = `${args.location}-docker.pkg.dev`;
		this.repositoryPrefix = gkeRepositoryPrefix(
			args.location,
			args.project,
			args.repositoryId,
		);
		this.kpackBuilderRepository = gkeKpackBuilderRepository(
			args.location,
			args.project,
			args.repositoryId,
		);

		this.repository = new gcp.artifactregistry.Repository(
			`${name}-repo`,
			{
				repositoryId: args.repositoryId,
				location: args.location,
				project: args.project,
				format: "DOCKER",
				description: "Korifi packages, droplets, and kpack builder",
			},
			{ parent: this },
		);

		const sa = new gcp.serviceaccount.Account(
			`${name}-sa`,
			{
				accountId: `${args.repositoryId}-writer`.slice(0, 30),
				displayName: "Korifi Artifact Registry writer",
				project: args.project,
			},
			{ parent: this },
		);
		this.serviceAccountEmail = sa.email;

		new gcp.artifactregistry.RepositoryIamMember(
			`${name}-sa-writer`,
			{
				project: args.project,
				location: args.location,
				repository: this.repository.name,
				role: "roles/artifactregistry.writer",
				member: pulumi.interpolate`serviceAccount:${sa.email}`,
			},
			{ parent: this },
		);

		const key = new gcp.serviceaccount.Key(
			`${name}-sa-key`,
			{ serviceAccountId: sa.name },
			{ parent: this, additionalSecretOutputs: ["privateKey"] },
		);

		// GCP returns a base64-encoded JSON key; docker login wants the JSON body.
		this.serviceAccountKey = key.privateKey.apply((encoded) =>
			Buffer.from(encoded, "base64").toString("utf8"),
		);

		this.registerOutputs({
			repositoryPrefix: this.repositoryPrefix,
			dockerServer: this.dockerServer,
			serviceAccountEmail: this.serviceAccountEmail,
		});
	}
}
