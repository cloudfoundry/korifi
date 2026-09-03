# deploy/gke — Korifi on Google Kubernetes Engine

Pulumi program for [INSTALL.md](../../INSTALL.md) on GKE with Google Artifact
Registry. Shared install logic lives in [`../lib`](../lib).

## Quick start

```sh
cd deploy/lib && bun install
cd ../gke && bun install
export PULUMI_CONFIG_PASSPHRASE=<stack passphrase>
pulumi stack init prod
pulumi config set gcp:project my-project
pulumi config set gcp:region us-central1
pulumi config set clusterName my-cluster
pulumi config set baseDomain korifi.example.org
# Use your Google account email so cf login can select it:
pulumi config set adminUserName you@example.com
pulumi up
```

Prerequisites: `gcloud` auth, `gke-gcloud-auth-plugin`, `pulumi`, `kubectl`,
`cf` CLI v8+.

After apply, point DNS (A records) at the Contour `envoy-korifi` LoadBalancer
IP, then:

```sh
cf api https://api.<baseDomain> --skip-ssl-validation
cf login   # select the adminUserName entry
```

## What it creates

| Component | INSTALL.md section |
| --- | --- |
| `GkeCluster` | GKE cluster + node pool (Workload Identity enabled) |
| `GkeRegistry` | Artifact Registry Docker repo + writer SA/key |
| `registryPullSecret` | `image-registry-credentials` in the CF root namespace |
| `KorifiDependencies` | cert-manager, kpack, Contour, Knative, metrics-server |
| `KorifiRelease` | Helm install with GAR `containerRepositoryPrefix` |
| `ContourGateway` | LoadBalancer GatewayClass |

## Configuration

| Key | Required | Meaning |
| --- | --- | --- |
| `gcp:project` | yes | GCP project |
| `clusterName` | yes | GKE cluster name |
| `baseDomain` | yes | Base for `api.` / `apps.` hosts |
| `adminUserName` | recommended | GCP user email used as CF admin |
| `artifactRegistryRepo` | no (default `korifi`) | GAR repository id |
| `artifactRegistryLocation` | no | e.g. `us` for `us-docker.pkg.dev` |
| `location` | no | GKE location (defaults to `gcp:region`) |
| `nodeMachineType` / `nodeCount` | no | Node pool sizing |
