/**
 * ECR access for Korifi + Kpack via IRSA (INSTALL.EKS.md § Setup IAM role).
 *
 * Also creates the kpack-builder repository and a least-privilege CF admin IAM
 * user mapped into the cluster.
 */
import * as aws from "@pulumi/aws";
import * as pulumi from "@pulumi/pulumi";

export interface EksRegistryArgs {
	clusterName: string;
	region: string;
	accountId: pulumi.Input<string>;
	oidcProviderArn: pulumi.Input<string>;
	oidcIssuer: pulumi.Input<string>;
	adminUserName: string;
}

export class EksRegistry extends pulumi.ComponentResource {
	readonly roleArn: pulumi.Output<string>;
	readonly builderRepositoryUrl: pulumi.Output<string>;
	readonly repositoryPrefix: pulumi.Output<string>;
	readonly adminUserArn: pulumi.Output<string>;
	readonly adminAccessKeyId: pulumi.Output<string>;
	readonly adminSecretAccessKey: pulumi.Output<string>;

	constructor(
		name: string,
		args: EksRegistryArgs,
		opts?: pulumi.ComponentResourceOptions,
	) {
		super("korifi:deploy:EksRegistry", name, {}, opts);

		const ecrPolicy = new aws.iam.Policy(
			`${name}-ecr-policy`,
			{
				name: `${args.clusterName}-korifi-ecr-push-pull`,
				policy: JSON.stringify({
					Version: "2012-10-17",
					Statement: [
						{
							Effect: "Allow",
							Action: [
								"ecr:BatchCheckLayerAvailability",
								"ecr:BatchGetImage",
								"ecr:CompleteLayerUpload",
								"ecr:GetAuthorizationToken",
								"ecr:GetDownloadUrlForLayer",
								"ecr:InitiateLayerUpload",
								"ecr:PutImage",
								"ecr:UploadLayerPart",
								"ecr:CreateRepository",
								"ecr:ListImages",
								"ecr:BatchDeleteImage",
							],
							Resource: "*",
						},
					],
				}),
			},
			{ parent: this },
		);

		const role = new aws.iam.Role(
			`${name}-ecr-role`,
			{
				name: `${args.clusterName}-korifi-ecr-sa`,
				description: "allows korifi service accounts to access ECR",
				assumeRolePolicy: pulumi
					.all([args.oidcProviderArn, args.oidcIssuer])
					.apply(([oidcArn, issuer]) =>
						JSON.stringify({
							Version: "2012-10-17",
							Statement: [
								{
									Effect: "Allow",
									Principal: { Federated: oidcArn },
									Action: "sts:AssumeRoleWithWebIdentity",
									Condition: {
										StringLike: {
											[`${issuer}:aud`]: "sts.amazonaws.com",
											[`${issuer}:sub`]: [
												"system:serviceaccount:kpack:controller",
												"system:serviceaccount:korifi:korifi-api-system-serviceaccount",
												"system:serviceaccount:korifi:korifi-controllers-controller-manager",
												"system:serviceaccount:*:kpack-service-account",
											],
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
			`${name}-ecr-attach`,
			{ role: role.name, policyArn: ecrPolicy.arn },
			{ parent: this },
		);

		this.roleArn = role.arn;

		const builderRepo = new aws.ecr.Repository(
			`${name}-kpack-builder`,
			{
				name: `${args.clusterName}/kpack-builder`,
				forceDelete: true,
			},
			{ parent: this },
		);
		this.builderRepositoryUrl = builderRepo.repositoryUrl;

		this.repositoryPrefix = pulumi.interpolate`${args.accountId}.dkr.ecr.${args.region}.amazonaws.com/${args.clusterName}/`;

		// Plain IAM user for CF admin (INSTALL.EKS.md § Setup admin user).
		const adminUser = new aws.iam.User(
			`${name}-cf-admin`,
			{ name: `${args.clusterName}-cf-admin` },
			{ parent: this },
		);
		this.adminUserArn = adminUser.arn;

		const accessKey = new aws.iam.AccessKey(
			`${name}-cf-admin-key`,
			{ user: adminUser.name },
			{ parent: this, additionalSecretOutputs: ["secret"] },
		);
		this.adminAccessKeyId = accessKey.id;
		this.adminSecretAccessKey = accessKey.secret;

		// Map the IAM user into the cluster as the Korifi admin username.
		new aws.eks.AccessEntry(
			`${name}-cf-admin-access`,
			{
				clusterName: args.clusterName,
				principalArn: adminUser.arn,
				userName: args.adminUserName,
				type: "STANDARD",
			},
			{ parent: this },
		);

		new aws.eks.AccessPolicyAssociation(
			`${name}-cf-admin-policy`,
			{
				clusterName: args.clusterName,
				principalArn: adminUser.arn,
				policyArn:
					"arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy",
				accessScope: { type: "cluster" },
			},
			{ parent: this },
		);

		this.registerOutputs({
			roleArn: this.roleArn,
			builderRepositoryUrl: this.builderRepositoryUrl,
			repositoryPrefix: this.repositoryPrefix,
		});
	}
}
