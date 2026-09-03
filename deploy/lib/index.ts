/**
 * Shared, reusable Korifi deploy building blocks.
 *
 * Stacks under deploy/{kind,eks,gke} compose these ComponentResources;
 * pure helpers in values.ts / versions.ts are unit-tested without a cluster.
 */
export { ContourGateway, type ContourGatewayArgs, type ContourPublishType } from "./contour-gateway";
export {
	KorifiDependencies,
	type KorifiDependenciesArgs,
} from "./dependencies";
export { EcrKpackIrsa, type EcrKpackIrsaArgs } from "./ecr-kpack-irsa";
export {
	KorifiRelease,
	type KorifiReleaseArgs,
} from "./korifi-release";
export {
	LocalRegistry,
	registryPullSecret,
	type LocalRegistryArgs,
} from "./local-registry";
export {
	KorifiNamespaces,
	type KorifiNamespacesArgs,
} from "./namespaces";
export {
	buildKorifiValues,
	eksKpackBuilderRepository,
	eksRepositoryPrefix,
	gkeKpackBuilderRepository,
	gkeRepositoryPrefix,
	kindGatewayPorts,
	kindKpackBuilderRepository,
	kindRegistry,
	kindRegistryPrefix,
	type KorifiPlatform,
	type KorifiValuesInput,
} from "./values";
export { korifiChartUrl, versions } from "./versions";
