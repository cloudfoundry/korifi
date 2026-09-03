/**
 * Korifi Helm release (INSTALL.md § Install Korifi).
 *
 * Values are assembled by `buildKorifiValues` so platform differences stay
 * unit-testable; this component only owns the Release lifecycle.
 */
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import {
	buildKorifiValues,
	type KorifiValuesInput,
} from "./values";
import { korifiChartUrl, versions } from "./versions";

export interface KorifiReleaseArgs {
	provider: k8s.Provider;
	namespace: pulumi.Input<string>;
	values: KorifiValuesInput | pulumi.Input<Record<string, unknown>>;
	/** Override chart source; default is the GitHub release tarball. */
	chart?: string;
	chartVersion?: string;
	timeoutSeconds?: number;
	dependsOn?: pulumi.Input<pulumi.Resource>[];
}

function isValuesInput(v: unknown): v is KorifiValuesInput {
	return (
		typeof v === "object" &&
		v !== null &&
		"platform" in v &&
		"adminUserName" in v &&
		"apiUrl" in v
	);
}

export class KorifiRelease extends pulumi.ComponentResource {
	readonly release: k8s.helm.v3.Release;

	constructor(
		name: string,
		args: KorifiReleaseArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:KorifiRelease", name, {}, opts);

		const chartVersion = args.chartVersion ?? versions.korifi;
		const chart = args.chart ?? korifiChartUrl(chartVersion);

		const values: pulumi.Input<Record<string, unknown>> = isValuesInput(
			args.values,
		)
			? buildKorifiValues(args.values)
			: (args.values as pulumi.Input<Record<string, unknown>>);

		this.release = new k8s.helm.v3.Release(
			`${name}-release`,
			{
				name: "korifi",
				chart,
				namespace: args.namespace,
				timeout: args.timeoutSeconds ?? 1800,
				values,
			},
			{
				parent: this,
				provider: args.provider,
				dependsOn: args.dependsOn,
				customTimeouts: { create: "35m", update: "35m" },
			},
		);

		this.registerOutputs({
			releaseName: this.release.name,
			status: this.release.status,
		});
	}
}
