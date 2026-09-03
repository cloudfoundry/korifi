/**
 * EKS cluster + EBS CSI addon (INSTALL.EKS.md § Cluster creation / EBS CSI).
 */
import * as aws from "@pulumi/aws";
import * as eks from "@pulumi/eks";
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";

export interface EksClusterArgs {
	clusterName: string;
	region: string;
	instanceType?: string;
	desiredCapacity?: number;
}

export class EksCluster extends pulumi.ComponentResource {
	readonly cluster: eks.Cluster;
	readonly kubeconfig: pulumi.Output<string>;
	readonly provider: k8s.Provider;
	readonly oidcIssuer: pulumi.Output<string>;
	readonly oidcProviderArn: pulumi.Output<string>;
	readonly clusterName: string;

	constructor(
		name: string,
		args: EksClusterArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:EksCluster", name, {}, opts);

		this.clusterName = args.clusterName;

		this.cluster = new eks.Cluster(
			`${name}-cluster`,
			{
				name: args.clusterName,
				version: "1.31",
				instanceType: args.instanceType ?? "m5.xlarge",
				desiredCapacity: args.desiredCapacity ?? 3,
				minSize: 2,
				maxSize: 5,
				createOidcProvider: true,
				// AccessEntry for the CF admin IAM user (INSTALL.EKS.md).
				authenticationMode: eks.AuthenticationMode.ApiAndConfigMap,
				endpointPrivateAccess: false,
				endpointPublicAccess: true,
			},
			{ parent: this },
		);

		this.kubeconfig = this.cluster.kubeconfig.apply((kc) =>
			typeof kc === "string" ? kc : JSON.stringify(kc),
		);

		this.provider = new k8s.Provider(
			`${name}-k8s`,
			{ kubeconfig: this.kubeconfig, enableServerSideApply: true },
			{ parent: this, dependsOn: [this.cluster] },
		);

		const oidcProviderOutput = this.cluster.core.oidcProvider;
		if (!oidcProviderOutput) {
			throw new Error(
				"EKS OIDC provider missing; createOidcProvider must be true",
			);
		}

		const oidc = oidcProviderOutput.apply((provider) => {
			if (!provider) {
				throw new Error(
					"EKS OIDC provider missing; createOidcProvider must be true",
				);
			}
			return provider;
		});

		this.oidcIssuer = oidc.apply((provider) =>
			pulumi.output(provider.url).apply((url) => url.replace(/^https:\/\//, "")),
		);
		this.oidcProviderArn = oidc.apply((provider) => provider.arn);

		// EBS CSI addon — INSTALL.EKS.md
		const ebsRole = new aws.iam.Role(
			`${name}-ebs-csi`,
			{
				name: `${args.clusterName}-ebs-csi-driver`,
				assumeRolePolicy: pulumi
					.all([this.oidcProviderArn, this.oidcIssuer])
					.apply(([oidcArn, issuer]) =>
						JSON.stringify({
							Version: "2012-10-17",
							Statement: [
								{
									Effect: "Allow",
									Principal: { Federated: oidcArn },
									Action: "sts:AssumeRoleWithWebIdentity",
									Condition: {
										StringEquals: {
											[`${issuer}:aud`]: "sts.amazonaws.com",
											[`${issuer}:sub`]:
												"system:serviceaccount:kube-system:ebs-csi-controller-sa",
										},
									},
								},
							],
						}),
					),
			},
			{ parent: this },
		);

		new aws.iam.RolePolicyAttachment(
			`${name}-ebs-csi-policy`,
			{
				role: ebsRole.name,
				policyArn:
					"arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy",
			},
			{ parent: this },
		);

		new aws.eks.Addon(
			`${name}-ebs-csi-addon`,
			{
				clusterName: this.cluster.eksCluster.name,
				addonName: "aws-ebs-csi-driver",
				serviceAccountRoleArn: ebsRole.arn,
			},
			{ parent: this, dependsOn: [this.cluster] },
		);

		this.registerOutputs({
			clusterName: this.clusterName,
			oidcIssuer: this.oidcIssuer,
			oidcProviderArn: this.oidcProviderArn,
		});
	}
}
