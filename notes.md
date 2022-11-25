# TODO

## kpack-controller service account needs to have the magic role association

Document it

# EXPLORE NOTES:

# Stories

1. Use service accounts in config throughout
  * Omit service account creation if a value passed in (i.e. user takes care of it for ECR perms)
  * No longer refer to secrets directly
    - When creating a service account, put secret in `imagePullSecrets`
  * Applies to api, kpack-image-builder and kpack builder service account at least.
  * Don't set source image pull secrets in kpack Image resource

2. Switch to concrete repositories for droplets and packages
  * No longer use prefixes.

3. Make registry secrets optional in korifi
  * When used, they should be references in service accounts only
  * E.g. it is possible to use registry credential helpers alone on EKS
    - api, kpack-image-builder, kpack-service-account service accounts all have ECR role
    - EKS nodes have ECR pull perms.
  * Note that cf-org/space controller is not happy when there aren't registry secrets

4. Docs
  * IAM mapping (general user thing)
  * Requirement for PVC driver in EKS
  * IAM service account creation with ECR perms for EKS

?. Would service account propagation work for IAM-enabled service accounts?

# Users

- The user creating the cluster will work, but the username will be `kubernetes-admin` and won't match the auth-info in $KUBECONFIG.

- How to use a regular IAM user?
  - Create a user with no permissions in IAM
  - Map the user in the EKS configmap, e.g.
  ```
  eksctl create iamidentitymapping \
  --cluster my-cluster \
  --region=my-region \
  --username=my-k8s-username \
  --arn arn:aws:iam::111122223333:user/my-iam-username \
  --no-duplicate-arns
  ```

- Other type of users?
  - EKS will _not_ sign client certs, so that method does not work

# ECR credentials expiring

- Check https://github.com/pivotal/kpack/issues/511

- We can associate a service account with an AWS IAM role
  - We must activate OIDC, if not done already:
	  ```
		eksctl utils associate-iam-oidc-provider \
			--cluster=my-cluster \
			--region my-region \
			--approve
		```
	- Create a policy allowing push/pull on ECR
	  ```
		cat <<EOF > ecr-policy.json
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
                "ecr:UploadLayerPart"
            ],
            "Resource": "*"
        }
    ]
}
EOF
    aws iam create-policy --policy-name ecr-policy --policy-document file://ecr-policy.json --region eu-west-3
    ```
	- Create the IAM role and associate with the service account
	  ```
		eksctl create iamserviceaccount \
		  --name korifi-api-system-serviceaccount \
			--namespace korifi-system \
			--cluster korifi-test \
			--region eu-west-3 \
			--role-name "ecr-push-puller" \
      --attach-policy-arn arn:aws:iam::649758297924:policy/ecr-policy \
			--approve
		```

Then follows steps in https://docs.aws.amazon.com/eks/latest/userguide/associate-service-account-role.html

# ECR doesn't create on push

- Need to stop appending package/droplet GUID onto repository prefix
- Create three repos in ECR: packages, droplets, kpack-image-builder
- Packages and droplets will be different versions of the same image in their repositories

# Kpack requires PVCs

- EKS does not come with a default PVC driver - you need to specify one
- For example:
  ```
  # Create a policy
  eksctl create iamserviceaccount \
    --name ebs-csi-controller-sa \
    --namespace kube-system \
    --cluster my-cluster \
    --region my-region \
    --attach-policy-arn arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy \
    --approve \
    --role-only \
    --role-name AmazonEKS_EBS_CSI_DriverRole

  # Create the EBS add-on
  eksctl create addon \
    --name aws-ebs-csi-driver \
    --cluster my-cluster \
    --service-account-role-arn arn:aws:iam::111122223333:role/AmazonEKS_EBS_CSI_DriverRole \
    --force \
    --region my-region
  ```

- You might need to enable OIDC:
  ```
  eksctl utils associate-iam-oidc-provider \
    --cluster=my-cluster \
    --region my-region \
    --approve
	  ```

# Secrets Mess

## API
* Needs write access to some source container registry
* kpack builder pod needs to read this registry
* Writes pull secret into CFPackage spec

## Controllers
* needs to propagate the packageRegistrySecret
* copies package registry details to CFBuild source
* copies CFBuild source to BuildWorkload

## kpack
* reads source (will use service account image pull secrets)
* writes droplet (will use service account secrets)

## kpack-image-builder
* needs to read image to extract process list
* copies imagePullSecrets from `kpack-service-account` (hard-coded) into droplet status

## statefulset/ task runner
* needs to be able to pull image
    - either node perm, or image pull secret
* copies imagePullSecrets from droplet status into pod spec

# Package secret

* Set in API configmap
* Injected in image handler
    * Used to write source package image
* Injected in package handler
    * Used to set image pull secret in CFPackage
* Eventually copied to BuildWorkload

# Droplet secret

* Set in kpack-service-account secrets and image pull secrets
  * Value from secrets used by kpack to push droplet image
  * Value from image pull secrets set in droplet status of CFBuild
      * also used to list processes on image and store the result in CFBuild status

* Value from CFBuild status used as pod image pull secret in RunWorkload / TaskWorkload

# Service accounts only

Config only - do not pass around in packages / droplets etc.

## Package sources:
* API config knows service account that can push to package repository
* Service account lives in the cf-space, so it can be varied if necessary

## kpack
Uses a single service account with:
1. secrets to push droplets
2. image pull secrets to pull packages
if using secrets.

Otherwise the service account IAM perms should do the trick.

## kpack-image-builder
* Config the single service account name that can pull from packages and push to droplets
    * (Use above to get process details too)

## Runners
* Config service account to pull droplet images (cf-space level)

Service accounts can be set up by admin to use IAM, or literal secrets if required. Korifi doesn't care.
