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
export {
	hashKorifiImageSources,
	hashOsbBrokerImageSources,
	hashSourceTree,
	korifiImageSourceEntries,
	osbBrokerImageSourceEntries,
} from "./image-source-hash";
export {
	KindKorifiImages,
	type KindKorifiImagesArgs,
} from "./kind-images";
export {
	KindOsbBrokerImage,
	type KindOsbBrokerImageArgs,
} from "./kind-osb-broker-image";
export {
	OsbServiceBroker,
	osbServiceBrokerGuid,
	osbServicePath,
	type OsbServiceBrokerArgs,
} from "./osb-service-broker";
export {
	KnativeServing,
	type KnativeServingArgs,
} from "./knative";
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
	ServiceBrokerServices,
	defaultServiceBrokerServiceEnable,
	type PostgresServiceArgs,
	type ServiceBrokerServiceConnection,
	type ServiceBrokerServicesArgs,
} from "./service-broker-services";
export { UaaCerts, type UaaCertsArgs } from "./uaa-certs";
export {
	UaaVcluster,
	kindUaaNodePort,
	type UaaVclusterArgs,
} from "./uaa-vcluster";
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
export { korifiChartUrl, kindUaaHostname, versions } from "./versions";
