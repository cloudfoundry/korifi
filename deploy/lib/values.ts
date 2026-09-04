/**
 * Pure builders for Korifi Helm values.
 *
 * Keeping value assembly out of ComponentResources makes platform differences
 * unit-testable without a cluster or Pulumi engine.
 */

export type KorifiPlatform = "kind" | "eks" | "gke";

export interface KorifiNetworkingInput {
	gatewayClass?: string;
	gatewayNamespace?: string;
	/** NodePorts for kind; omit on cloud (Contour LoadBalancer defaults). */
	gatewayPorts?: { http: number; https: number };
}

export interface KorifiValuesInput {
	platform: KorifiPlatform;
	adminUserName: string;
	apiUrl: string;
	appDomain: string;
	containerRepositoryPrefix: string;
	kpackBuilderRepository: string;
	rootNamespace?: string;
	generateIngressCertificates?: boolean;
	networking?: KorifiNetworkingInput;
	/** EKS only — IRSA role ARN for ECR push/pull. */
	eksContainerRegistryRoleARN?: string;
	/** When true (kind installer defaults), enable experimental managed services. */
	managedServices?: boolean;
	logLevel?: string;
	/**
	 * Override Korifi component images. Kind builds these from the checkout
	 * and `kind load`s them — Hub `*:latest` does not include knative-runner.
	 */
	images?: {
		controllers?: string;
		api?: string;
		migration?: string;
	};
	extraValues?: Record<string, unknown>;
}

/**
 * Build the Helm values object for a Korifi release.
 * Mirrors INSTALL.md / INSTALL.kind.md / INSTALL.EKS.md.
 */
export function buildKorifiValues(
	input: KorifiValuesInput,
): Record<string, unknown> {
	const networking = input.networking ?? {};
	const values: Record<string, unknown> = {
		adminUserName: input.adminUserName,
		rootNamespace: input.rootNamespace ?? "cf",
		defaultAppDomainName: input.appDomain,
		generateIngressCertificates: input.generateIngressCertificates ?? true,
		api: { apiServer: { url: input.apiUrl } },
		containerRepositoryPrefix: input.containerRepositoryPrefix,
		kpackImageBuilder: {
			builderRepository: input.kpackBuilderRepository,
		},
		networking: {
			gatewayClass: networking.gatewayClass ?? "contour",
			gatewayNamespace: networking.gatewayNamespace ?? "korifi-gateway",
			...(networking.gatewayPorts
				? { gatewayPorts: networking.gatewayPorts }
				: {}),
		},
		knativeRunner: { include: true },
		reconcilers: { run: "knative-runner" },
	};

	if (input.logLevel) {
		values.logLevel = input.logLevel;
	}

	if (input.platform === "kind") {
		values.logLevel = input.logLevel ?? "debug";
		values.stagingRequirements = { buildCacheMB: 1024 };
		values.controllers = { taskTTL: "5s" };
		values.jobTaskRunner = { jobTTL: "5s" };
		if (input.managedServices !== false) {
			values.experimental = {
				managedServices: {
					enabled: true,
					trustInsecureBrokers: true,
				},
			};
		}
	}

	if (input.platform === "eks") {
		// ECR uses IRSA; do not mount a dockerconfigjson secret.
		values.containerRegistrySecrets = {};
		if (!input.eksContainerRegistryRoleARN) {
			throw new Error(
				"eksContainerRegistryRoleARN is required when platform is eks",
			);
		}
		values.eksContainerRegistryRoleARN = input.eksContainerRegistryRoleARN;
	}

	if (input.extraValues) {
		deepMerge(values, input.extraValues);
	}

	if (input.images?.controllers) {
		setImage(values, "controllers", input.images.controllers);
	}
	if (input.images?.api) {
		setImage(values, "api", input.images.api);
	}
	if (input.images?.migration) {
		setImage(values, "migration", input.images.migration);
	}

	return values;
}

function setImage(
	values: Record<string, unknown>,
	key: string,
	image: string,
): void {
	const current = values[key];
	const obj =
		current !== null &&
		typeof current === "object" &&
		!Array.isArray(current)
			? (current as Record<string, unknown>)
			: {};
	values[key] = { ...obj, image, imagePullPolicy: "IfNotPresent" };
}

/** Kind NodePort mappings from INSTALL.kind.md / kind-config.yaml. */
export const kindGatewayPorts = { http: 32080, https: 32443 } as const;

/** In-cluster registry address used by the kind installer. */
export const kindRegistry = {
	clusterHost: "localregistry-docker-registry.default.svc.cluster.local:30050",
	hostAddress: "localhost:30050",
	nodePort: 30050,
} as const;

export function kindRegistryPrefix(): string {
	return `${kindRegistry.clusterHost}/`;
}

export function kindKpackBuilderRepository(): string {
	return `${kindRegistry.clusterHost}/kpack-builder`;
}

/** ECR repository prefix: `<account>.dkr.ecr.<region>.amazonaws.com/<cluster>/`. */
export function eksRepositoryPrefix(
	accountId: string,
	region: string,
	clusterName: string,
): string {
	return `${accountId}.dkr.ecr.${region}.amazonaws.com/${clusterName}/`;
}

export function eksKpackBuilderRepository(
	accountId: string,
	region: string,
	clusterName: string,
): string {
	return `${accountId}.dkr.ecr.${region}.amazonaws.com/${clusterName}/kpack-builder`;
}

/**
 * Artifact Registry prefix:
 * `<region>-docker.pkg.dev/<project>/<repository>/`.
 */
export function gkeRepositoryPrefix(
	region: string,
	projectId: string,
	repository: string,
): string {
	return `${region}-docker.pkg.dev/${projectId}/${repository}/`;
}

export function gkeKpackBuilderRepository(
	region: string,
	projectId: string,
	repository: string,
): string {
	return `${region}-docker.pkg.dev/${projectId}/${repository}/kpack-builder`;
}

function deepMerge(
	target: Record<string, unknown>,
	source: Record<string, unknown>,
): void {
	for (const [key, value] of Object.entries(source)) {
		if (
			value !== null &&
			typeof value === "object" &&
			!Array.isArray(value) &&
			typeof target[key] === "object" &&
			target[key] !== null &&
			!Array.isArray(target[key])
		) {
			deepMerge(
				target[key] as Record<string, unknown>,
				value as Record<string, unknown>,
			);
		} else {
			target[key] = value;
		}
	}
}
