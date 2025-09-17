<h1 align="center" style="border: none">
  <img alt="Korifi" src="/logo/color/Korifi-logo-color.svg" width="50%" />
</h1>

<p align="center">
  <a href="https://ci.korifi.cf-app.com/teams/main/pipelines/main">
      <img alt="Build Status" src="https://ci.korifi.cf-app.com/api/v1/teams/main/pipelines/main/badge" />
  </a>
</p>

This repository contains an implementation of the [Cloud Foundry V3 API](http://v3-apidocs.cloudfoundry.org) that is backed entirely by Kubernetes [custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).

For more information about what we're building, check out the [_Vision for CF on Kubernetes_](https://docs.google.com/document/d/1rG814raI5UfGUsF_Ycrr8hKQMo1RH9TRMxuvkgHSdLg/edit) document.

## Differences with Cloud Foundry for VMs
Korifi is very different from CF for VMs architecturally. Most core CF components have been replaced by more Kubernetes native equivalents. Check out our [architecture docs](docs/architecture.md) to learn more.

Although we aim to preserve the core Cloud Foundry developer experience, there are some key behavior differences. We've listed some of these in our [differences between Korifi and CF-for-VMs](https://github.com/cloudfoundry/korifi/blob/main/docs/known-differences-with-cf-for-vms.md) doc.

Additionally, we do not currently support all V3 CF APIs or filters. What we do support is tracked in our [API endpoint docs](docs/api.md).

## Installation

Check our [installation instructions](./INSTALL.md).

## Contributing

Please check our [contributing guidelines](/CONTRIBUTING.md) and our [good first issues](https://github.com/cloudfoundry/korifi/contribute).

This project follows [Cloud Foundry Code of Conduct](https://www.cloudfoundry.org/code-of-conduct/).

## Hacking

Our [hacking guide](./HACKING.md) has instructions on how to work on the project locally.

## License

This project is licensed under the [Apache License, Version 2.0](/LICENSE).

When using the Korifi or other Cloud Foundry logos be sure to follow the [guidelines](https://www.cloudfoundry.org/logo/).
