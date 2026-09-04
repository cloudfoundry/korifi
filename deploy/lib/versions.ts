/**
 * Pinned versions shared by every deploy stack.
 *
 * `korifi` and `korifiInstallerImage` MUST move together: the installer image
 * carries `scripts/install-dependencies.sh` and the vendor manifests that the
 * matching Korifi release was tested against.
 */
export const versions = {
	korifi: "0.18.0",
	/** Digest-pinned installer from the v0.18.0 release. */
	korifiInstallerImage:
		"index.docker.io/cloudfoundry/korifi-installer@sha256:dfe1680d13550dfd5ff2abefbe778c180cafa07ff055a5a82802df0f2551aa30",
	/** twuni/docker-registry chart (kind local registry). */
	registryChart: "3.0.0",
	/** Knative Operator Helm chart (https://knative.github.io/operator). */
	knativeOperatorChart: "v1.23.1",
	/** KnativeServing CR spec.version — must stay in the Operator's support range. */
	knativeServing: "1.23.0",
	/** Shared Postgres image for ServiceBrokerServices (OSB broker backend). */
	postgresImage: "postgres:16-alpine",
} as const;

/** Helm chart URL for a released Korifi version. */
export function korifiChartUrl(version: string = versions.korifi): string {
	return `https://github.com/cloudfoundry/korifi/releases/download/v${version}/korifi-${version}.tgz`;
}
