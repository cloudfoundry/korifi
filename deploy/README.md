# deploy/ — Pulumi installers for Korifi

TypeScript Pulumi programs that automate the platform install guides:

| Stack | Guide | Cluster |
| --- | --- | --- |
| [`kind/`](./kind) | [INSTALL.kind.md](../INSTALL.kind.md) | kind (local Docker) |
| [`eks/`](./eks) | [INSTALL.EKS.md](../INSTALL.EKS.md) | Amazon EKS + ECR |
| [`gke/`](./gke) | [INSTALL.md](../INSTALL.md) on GKE | GKE + Artifact Registry |

Shared, unit-tested building blocks live in [`lib/`](./lib):

- **Pure helpers** — `buildKorifiValues`, repository prefix builders, pinned versions
- **ComponentResources** — `KorifiNamespaces`, `KorifiDependencies`, `LocalRegistry`,
  `KorifiRelease`, `ContourGateway`, `EcrKpackIrsa`, `ServiceBrokerServices`,
  `KindOsbBrokerImage`, `OsbServiceBroker`

```sh
cd deploy/lib && bun install && bun test
cd ../kind && bun install && bun test   # same for eks / gke
```

Each stack README documents `pulumi up` and required config.
