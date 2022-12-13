# Installing Korifi on a New Amazon EKS Cluster

## Prerequisites

* Tools:
  * [`aws`](https://docs.aws.amazon.com/cli)
  * [`eksctl`](https://github.com/weaveworks/eksctl)

## Initial setup

The following environment variables will be needed throughout this guide:

* CLUSTER_NAME: the name of the EKS cluster to create/use
* AWS_REGION: the desired AWS region
* ACCOUNT_ID: the 12-digit ID of your AWS account

Here are the example values we'll use in this guide:

```sh
CLUSTER_NAME=my-cluster
AWS_REGION=us-west-1
ACCOUNT_ID="$(aws sts get-caller-identity --query "Account" --output text)"
```

### Cluster creation

Create the cluster with OIDC enabled (used for service account / role association and the EBS CSI addon):
```sh
eksctl create cluster \
  --name "${CLUSTER_NAME}" \
  --region "${AWS_REGION}" \
  --with-oidc
```

Your kubeconfig will be updated to be targetting the new cluster when this command completes.

### Install the EBS CSI addon

Create the EBS CSI addon to allow PVCs (https://docs.aws.amazon.com/eks/latest/userguide/ebs-csi.html).

First, set up the service account and role for the addon:
```sh
eksctl create iamserviceaccount \
  --name ebs-csi-controller-sa \
  --namespace kube-system \
  --region "${AWS_REGION}" \
  --cluster "${CLUSTER_NAME}" \
  --attach-policy-arn arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy \
  --approve \
  --role-only \
  --role-name AmazonEKS_EBS_CSI_DriverRole
```

Next, install the addon:
```sh
eksctl create addon \
  --name aws-ebs-csi-driver \
  --region "${AWS_REGION}" \
  --cluster "${CLUSTER_NAME}" \
  --service-account-role-arn "arn:aws:iam::${ACCOUNT_ID}:role/AmazonEKS_EBS_CSI_DriverRole"
```

### Setup IAM role for access to ECR

The main difference when installing Korifi on EKS with ECR, as opposed to other container registries,
is that ECR tokens issued by the AWS CLI expire in 12 hours.
This means they are not a good option for storing in Kubernetes secrets for Korifi to use.
Instead, we must grant the ECR push and pull permissions in an IAM role
and allow that role to be associated with various service accounts used by Korifi and Kpack.

The [AWS docs](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)
have more details about associating IAM roles with Kubernetes service accounts.

First, create the ECR policy:
```
cat >ecr-policy.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ecr:BatchCheckLayerAvailability",
        "ecr:BatchGetImage",
        "ecr:CompleteLayerUpload",
        "ecr:GetAuthorizationToken",
        "ecr:GetDownloadUrlForLayer",
        "ecr:InitiateLayerUpload",
        "ecr:PutImage",
        "ecr:UploadLayerPart",
        "ecr:CreateRepository"
      ],
      "Resource": "*"
    }
  ]
}
EOF
aws iam create-policy \
    --policy-name korifi-ecr-push-pull-policy \
    --region "${AWS_REGION}" \
    --policy-document file://ecr-policy.json
rm ecr-policy.json
```

Next, define the trust relationships with the service accounts:
```
OIDC_PROVIDER="$(aws eks describe-cluster \
  --name "${CLUSTER_NAME}" \
  --region "${AWS_REGION}" \
  --query "cluster.identity.oidc.issuer" \
  --output text |
  sed -e "s/^https:\/\///"
)"
cat >trust-relationships.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::${ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringLike": {
          "${OIDC_PROVIDER}:aud": "sts.amazonaws.com",
          "${OIDC_PROVIDER}:sub": [
            "system:serviceaccount:kpack:controller",
            "system:serviceaccount:korifi:korifi-api-system-serviceaccount",
            "system:serviceaccount:korifi:korifi-kpack-build-controller-manager",
            "system:serviceaccount:*:kpack-service-account"
          ]
        }
      }
    }
  ]
}
EOF
```

Last, create the role and associate it with the trust-relationships and the ECR access policy:
```
aws iam create-role \
  --role-name korifi-ecr-service-account-role \
  --region "${AWS_REGION}" \
  --description "allows korifi service accounts to access ECR" \
  --assume-role-policy-document file://trust-relationships.json
rm trust-relationships.json
aws iam attach-role-policy \
  --role-name korifi-ecr-service-account-role \
  --region "${AWS_REGION}" \
  --policy-arn=arn:aws:iam::${ACCOUNT_ID}:policy/korifi-ecr-push-pull-policy
```

You will need the role ARN for subsequent steps.
```
ECR_ROLE_ARN="$(aws iam get-role \
  --role-name korifi-ecr-service-account-role \
  --query "Role.Arn" \
  --output text
)"
```

### Setup admin user

You must supply a user to be the Korifi admin user.
It is not recommended to use a privileged user, such as the one that created the cluster.
A plain IAM user with no permissions is sufficient.

Create the user and extract the ARN, key and secret:
```sh
aws iam create-user \
  --user-name "${CLUSTER_NAME}-cf-admin" \
  --region "${AWS_REGION}"
USER_ARN="$(aws iam get-user \
  --user-name "${CLUSTER_NAME}-cf-admin" \
  --region "${AWS_REGION}" \
  --query "User.Arn" \
  --output text
)"
aws iam create-access-key \
  --user-name "${CLUSTER_NAME}-cf-admin" \
  --region "${AWS_REGION}" > creds.json
USER_ACCESS_KEY_ID="$(jq -r .AccessKey.AccessKeyId creds.json)"
USER_SECRET_ACCESS_KEY="$(jq -r .AccessKey.SecretAccessKey creds.json)"
```

To identify this user to Kubernetes, modify the kube-system/aws-auth configmap using `eksctl`:
```
eksctl create iamidentitymapping \
  --cluster "${CLUSTER_NAME}" \
  --region "${AWS_REGION}" \
  --arn "${USER_ARN}" \
  --username cf-admin
```

### Create a Kpack builder ECR repository

Ensure we have an ECR repository to store the Kpack builder images:
```
aws ecr create-repository \
  --region "${AWS_REGION}" \
  --repository-name "${CLUSTER_NAME}/kpack-builder"
KPACK_BUILDER_REPO="$(aws ecr describe-repositories \
  --region "${AWS_REGION}" \
  --repository-names "${CLUSTER_NAME}/kpack-builder" \
  --query "repositories[0].repositoryUri" \
  --output text
)"
```

The droplets and package repositories will be created on demand by Korifi, on a per-app basis.

## Install Korifi and dependencies

Now the system is ready to install Korifi.
Follow the instructions in the [main installation guide](INSTALL.md), making the following changes:
* Do _not_ create the [image registry secret](INSTALL.md#container-registry-credentials-secret)
* After installing the [Kpack dependency](INSTALL.md#kpack), run the following commands to associate the controller service account with the ECR access role:
  ```sh
  kubectl -n kpack annotate serviceaccount controller eks.amazonaws.com/role-arn="${ECR_ROLE_ARN}"
  kubectl -n kpack rollout restart deployment kpack-controller
  ```
* Use the following helm command to install Korifi:
  ```sh
  helm install korifi https://github.com/cloudfoundry/korifi/releases/download/v<VERSION>/korifi-<VERSION>.tgz \
    --namespace="$KORIFI_NAMESPACE" \
    --set global.containerRegistrySecret="" \
    --set global.eksContainerRegistryRoleARN="${ECR_ROLE_ARN}" \
    --set=global.generateIngressCertificates=true \
    --set=global.rootNamespace="${ROOT_NAMESPACE}" \
    --set=adminUserName="${ADMIN_USERNAME}" \
    --set=api.apiServer.url="api.${BASE_DOMAIN}" \
    --set=global.defaultAppDomainName="apps.${BASE_DOMAIN}" \
    --set=api.packageRepository="${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/${CLUSTER_NAME}/packages" \
    --set=kpack-image-builder.builderRepository="${KPACK_BUILDER_REPO}" \
    --set=kpack-image-builder.dropletRepository=${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/${CLUSTER_NAME}/droplets
  ```
