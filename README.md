<h1 align="center" style="border: none">
  <img alt="Korifi" src="/logo/color/Korifi-logo-color.svg" width="50%" />
</h1>

<p align="center">
  <a href="https://ci.korifi.cf-app.com/teams/main/pipelines/main">
      <img alt="Build Status" src="https://ci.korifi.cf-app.com/api/v1/teams/main/pipelines/main/badge" />
  </a>
  <a href="https://codeclimate.com/github/cloudfoundry/korifi/maintainability">
    <img alt="Maintainability" src="https://api.codeclimate.com/v1/badges/1112ab5cfa6a0654cfd2/maintainability" />
  </a>
  <a href="https://codeclimate.com/github/cloudfoundry/korifi/test_coverage">
    <img alt="Test Coverage" src="https://api.codeclimate.com/v1/badges/1112ab5cfa6a0654cfd2/test_coverage" />
  </a>
</p>

This repository contains an experimental implementation of the [Cloud Foundry V3 API](http://v3-apidocs.cloudfoundry.org) that is backed entirely by Kubernetes [custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).

For more information about what we're building, check out the [_Vision for CF on Kubernetes_](https://docs.google.com/document/d/1rG814raI5UfGUsF_Ycrr8hKQMo1RH9TRMxuvkgHSdLg/edit) document.

## Differences with Cloud Foundry for VMs

Check our document about [differences between Korifi and CF-for-VMs](https://github.com/cloudfoundry/korifi/blob/main/docs/known-differences-with-cf-for-vms.md).

Our [API endpoint docs](docs/api.md) track the Cloud Foundry V3 API endpoints that we currently support.

## Installation

Check our [installation instructions](./INSTALL.md).

## Contributing

Please check our [contributing guidelines](/CONTRIBUTING.md) and our [good first issues](https://github.com/cloudfoundry/korifi/contribute).

This project follows [Cloud Foundry Code of Conduct](https://www.cloudfoundry.org/code-of-conduct/).

## Hacking

Our [hacking guide](./HACKING.md) has instructions on how to work on the project locally.

## License

This project is licensed under the [Apache License, Version 2.0](/LICENSE).

When using the Korifi or other Cloud Foundry logos, be sure to follow the [guidelines](https://www.cloudfoundry.org/logo/).
