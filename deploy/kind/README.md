# deploy/kind — Korifi on kind

One `pulumi up` applies [INSTALL.kind.md](../../INSTALL.kind.md): a kind
cluster with ingress NodePorts, an in-cluster registry, Korifi dependencies
(cert-manager, kpack, Contour), the **Knative Operator** (Helm) plus a
`KnativeServing` CR (Kourier ClusterIP), **UAA in a vcluster** (OIDC for
`cf login`), and the Korifi Helm release with `reconcilers.run=knative-runner`
plus `experimental.uaa`.

Korifi **controllers**, **api**, and **migration** images are built from this
checkout and `kind load`ed. Helm is pinned to those tags (not Docker Hub
`*:latest`, which does not include knative-runner). Changing those sources
rebuilds and reloads on the next `pulumi up`. The in-cluster registry is for
apps/kpack only.

Reusable pieces live in [`../lib`](../lib) (`KorifiDependencies`,
`LocalRegistry`, `KorifiRelease`, `ContourGateway`, `ServiceBrokerServices`,
`KindOsbBrokerImage`, `OsbServiceBroker`, …) and are unit-tested there.

After `pulumi up` the stack builds [`osb-service/`](../../osb-service),
`kind load`s it, deploys it over HTTPS (cert-manager self-signed; Korifi
`trustInsecureBrokers` skips verify), and registers a `CFServiceBroker`.
OpenEverest runs in a vcluster; `cf create-service postgres dedicated`
creates one DatabaseCluster per instance. Stack outputs include
`postgres` admin facts, `osbBrokerUrl`, and `marketplaceHint`.

## Quick start

```sh
cd deploy/lib && bun install
cd ../kind && bun install
export PULUMI_CONFIG_PASSPHRASE=<stack passphrase>
pulumi stack init dev   # once
pulumi up --stack dev
```

Prerequisites: Docker, [kind](https://kind.sigs.k8s.io/), `pulumi`, `kubectl`,
`cf` CLI v8+. First `pulumi up` compiles the Korifi Go images (a few minutes).

Afterwards:

```sh
pulumi stack output
cf api https://localhost --skip-ssl-validation
cf login -u "$(pulumi stack output uaaAdminEmail)" \
  -p "$(pulumi stack output uaaAdminPassword --show-secrets)"
cf enable-service-access postgres
cf marketplace
```

UAA is published at `https://127.0.0.1:30443/uaa` (NodePort). The kind
kube-apiserver is configured with that issuer for OIDC. Authorization remains
Kubernetes RBAC (`adminUserName` is `uaa:<email>`).

When granting CF roles to other users, pass `--origin uaa`.

**Existing clusters:** if a kind cluster already exists without the UAA OIDC
flags, the stack deletes and recreates it on the next `pulumi up`.

## Configuration

| Key | Default | Meaning |
| --- | --- | --- |
| `clusterName` | `korifi` | kind cluster name |
| `appDomain` | `apps-127-0-0-1.nip.io` | CF apps wildcard domain |
| `apiUrl` | `localhost` | Korifi API host |
| `adminEmail` | `admin@korifi.local` | UAA admin / `cf login` user |
| `oidcPrefix` | `uaa` | OIDC username prefix |
| `registryUser` | `user` | In-cluster registry username |
| `kubeconfigPath` | `~/.kube/kind-<clusterName>.config` | Written by the stack |
| `korifiVersion` | pinned in `../lib/versions.ts` | Helm chart release |
| `installerImage` | pinned digest | Dependencies Job image |

## Teardown

```sh
pulumi destroy --stack dev
```
