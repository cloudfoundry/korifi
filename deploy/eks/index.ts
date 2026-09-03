/**
 * deploy/eks — Korifi on Amazon EKS (INSTALL.EKS.md).
 *
 * Creates the cluster (OIDC), EBS CSI, ECR IRSA role, CF admin IAM user,
 * kpack-builder repository, dependencies (incl. Knative), annotates the
 * kpack controller SA, and installs Korifi with knative-runner.
 */
import * as aws from "@pulumi/aws";
import * as pulumi from "@pulumi/pulumi";
import {
	ContourGateway,
	EcrKpackIrsa,
	KorifiDependencies,
	KorifiNamespaces,
	KorifiRelease,
	buildKorifiValues,
} from "@korifi/deploy-lib";
import { EksCluster } from "./cluster";
import {
	adminUserName,
	apiUrl,
	appDomain,
	clusterName,
	desiredCapacity,
	gatewayClassName,
	gatewayNamespace,
	nodeInstanceType,
	pinned,
	region,
	rootNamespace,
} from "./config";
import { EksRegistry } from "./ecr";

const caller = aws.getCallerIdentityOutput({});
const accountId = caller.accountId;

const cluster = new EksCluster("eks", {
	clusterName,
	region,
	instanceType: nodeInstanceType,
	desiredCapacity,
});

const registry = new EksRegistry(
	"ecr",
	{
		clusterName,
		region,
		accountId,
		oidcProviderArn: cluster.oidcProviderArn,
		oidcIssuer: cluster.oidcIssuer,
		adminUserName,
	},
	{ dependsOn: [cluster] },
);

const namespaces = new KorifiNamespaces(
	"ns",
	{
		provider: cluster.provider,
		rootNamespace,
		gatewayNamespace,
		installerNamespace: true,
	},
	{ dependsOn: [cluster] },
);

const dependencies = new KorifiDependencies(
	"deps",
	{
		provider: cluster.provider,
		knativeDomain: appDomain,
		clusterType: "EKS",
		installerImage: pinned.installerImage,
		installerNamespace: namespaces.installer!.metadata.name,
		dependsOn: [namespaces.installer!],
	},
	{ dependsOn: [namespaces] },
);

const kpackIrsa = new EcrKpackIrsa(
	"kpack-irsa",
	{
		provider: cluster.provider,
		roleArn: registry.roleArn,
		dependsOn: [dependencies.job],
	},
	{ dependsOn: [dependencies, registry] },
);

const korifi = new KorifiRelease(
	"korifi",
	{
		provider: cluster.provider,
		namespace: namespaces.korifi.metadata.name,
		chartVersion: pinned.korifi,
		values: pulumi
			.all([
				registry.repositoryPrefix,
				registry.builderRepositoryUrl,
				registry.roleArn,
			])
			.apply(([prefix, builderRepo, roleArn]) =>
				buildKorifiValues({
					platform: "eks",
					adminUserName,
					apiUrl,
					appDomain,
					rootNamespace,
					containerRepositoryPrefix: prefix,
					kpackBuilderRepository: builderRepo,
					eksContainerRegistryRoleARN: roleArn,
					networking: {
						gatewayClass: gatewayClassName,
						gatewayNamespace,
					},
				}),
			),
		dependsOn: [
			dependencies.job,
			kpackIrsa.restart,
			namespaces.gateway,
		],
	},
	{ dependsOn: [dependencies, kpackIrsa, registry] },
);

new ContourGateway(
	"contour",
	{
		provider: cluster.provider,
		gatewayClassName,
		publishType: "LoadBalancerService",
		dependsOn: [korifi.release],
	},
	{ dependsOn: [korifi] },
);

export const kubeconfig = cluster.kubeconfig;
export const cfApiUrl = `https://${apiUrl}`;
export const appsDomain = `*.${appDomain}`;
export const ecrRoleArn = registry.roleArn;
export const kpackBuilderRepository = registry.builderRepositoryUrl;
export const containerRepositoryPrefix = registry.repositoryPrefix;
export const cfAdminAccessKeyId = registry.adminAccessKeyId;
export const cfAdminSecretAccessKey = pulumi.secret(registry.adminSecretAccessKey);
export const dnsHint =
	"Point api.<baseDomain> and *.apps.<baseDomain> at the Contour envoy-korifi LoadBalancer hostname (CNAME).";
