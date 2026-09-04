# deploy/kind — Korifi on kind

One `pulumi up` applies [INSTALL.kind.md](../../INSTALL.kind.md): a kind
cluster with ingress NodePorts, an in-cluster registry, Korifi dependencies
(cert-manager, kpack, Contour), the **Knative Operator** (Helm) plus a
`KnativeServing` CR (Kourier ClusterIP), and the Korifi Helm release with
`reconcilers.run=knative-runner`.

Korifi **controllers**, **api**, and **migration** images are built from this
checkout and `kind load`ed. Helm is pinned to those tags (not Docker Hub
`*:latest`, which does not include knative-runner). Changing those sources
rebuilds and reloads on the next `pulumi up`. The in-cluster registry is for
apps/kpack only.

Reusable pieces live in [`../lib`](../lib) (`KorifiDependencies`,
`LocalRegistry`, `KorifiRelease`, `ContourGateway`, …) and are unit-tested
there.

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
cf auth kubernetes-admin   # or the username from stack config adminUserName
```

## Configuration

| Key | Default | Meaning |
| --- | --- | --- |
| `clusterName` | `korifi` | kind cluster name |
| `appDomain` | `apps-127-0-0-1.nip.io` | CF apps wildcard domain |
| `apiUrl` | `localhost` | Korifi API host |
| `adminUserName` | `kubernetes-admin` | CF admin (must match kubeconfig user CN) |
| `registryUser` | `user` | In-cluster registry username |
| `kubeconfigPath` | `~/.kube/kind-<clusterName>.config` | Written by the stack |
| `korifiVersion` | pinned in `../lib/versions.ts` | Helm chart release |
| `installerImage` | pinned digest | Dependencies Job image |

## Teardown

```sh
pulumi destroy --stack dev
```
