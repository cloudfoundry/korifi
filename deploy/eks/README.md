# deploy/eks — Korifi on Amazon EKS

Pulumi program for [INSTALL.EKS.md](../../INSTALL.EKS.md). Shared install
logic lives in [`../lib`](../lib).

## Quick start

```sh
cd deploy/lib && bun install
cd ../eks && bun install
export PULUMI_CONFIG_PASSPHRASE=<stack passphrase>
pulumi stack init prod
pulumi config set aws:region us-west-1
pulumi config set clusterName my-cluster
pulumi config set baseDomain korifi.example.org
pulumi up
```

Prerequisites: AWS credentials with rights to create EKS/IAM/ECR, `pulumi`,
`kubectl`, `cf` CLI v8+.

After apply, configure a CLI profile with the stack outputs
`cfAdminAccessKeyId` / `cfAdminSecretAccessKey`, point DNS (CNAME) at the
Contour `envoy-korifi` LoadBalancer hostname, then:

```sh
export AWS_PROFILE=<clusterName>-cf-admin
cf api https://api.<baseDomain> --skip-ssl-validation
cf login   # select the cf-admin entry
```

## What it creates

| Component | INSTALL.EKS.md section |
| --- | --- |
| `EksCluster` | Cluster + OIDC + EBS CSI addon |
| `EksRegistry` | ECR policy/role (IRSA), kpack-builder repo, CF admin IAM user |
| `KorifiDependencies` | cert-manager, kpack, Contour, Knative, metrics-server |
| `EcrKpackIrsa` | Annotate kpack controller SA + restart |
| `KorifiRelease` | Helm install with `eksContainerRegistryRoleARN` |
| `ContourGateway` | LoadBalancer GatewayClass |

## Configuration

| Key | Required | Meaning |
| --- | --- | --- |
| `clusterName` | yes | EKS cluster name |
| `baseDomain` | yes | Base for `api.` / `apps.` hosts |
| `aws:region` | recommended | AWS region |
| `adminUserName` | no (default `cf-admin`) | Kubernetes username for CF admin |
| `nodeInstanceType` | no | Worker instance type |
| `desiredCapacity` | no | Node group size |
