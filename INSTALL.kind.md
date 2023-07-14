> **Warning**
> Make sure you are using the correct version of these instructions by using the link in the release notes for the version you're trying to install. If you're not sure, check our [latest release](https://github.com/cloudfoundry/korifi/releases/latest).

# Install Korifi on kind

This document integrates our [install instructions](./INSTALL.md) with specific tips to install Korifi locally using [kind](https://kind.sigs.k8s.io/).

## Initial setup

Export the following environment variables:

```sh
ROOT_NAMESPACE="cf"
KORIFI_NAMESPACE="korifi-system"
ADMIN_USERNAME="kubernetes-admin"
BASE_DOMAIN="apps-127-0-0-1.nip.io"
```

`apps-127-0-0-1.nip.io` will conveniently resolve to `127.0.0.1` using [nip.io](https://nip.io/), which is exactly what we need.

### Cluster creation

In order to access the Korifi API, we'll need to [expose the cluster ingress locally](https://kind.sigs.k8s.io/docs/user/ingress/). To do it, create your kind cluster using a command like this:

```sh
cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
EOF
```

### Container registry

We recommend you use [DockerHub](https://hub.docker.com/) as your container registry.

## Dependencies

Follow the [common instructions](./INSTALL.md#dependencies), with the exception of Metrics Server.

### Metrics Server

Make sure you pass the following flags to the Metrics Server container (see [_Configuration_](https://github.com/kubernetes-sigs/metrics-server#configuration)):

-   `--kubelet-insecure-tls`
-   `--kubelet-preferred-address-types=InternalIP`

## Pre-install configuration

No changes here, follow the [common instructions](./INSTALL.md#pre-install-configuration).
For the container registry credentials `Secret`, we recommend you [create an access token](https://hub.docker.com/settings/security?generateToken=true) on DockerHub.
Remember to set `global.generateIngressCertificates` to `true` if you want to use self-signed TLS certificates.

## Install Korifi

No changes here, follow the [common instructions](./INSTALL.md#install-korifi).
If using DockerHub as recommended above, set the following values:

-   `kpackImageBuilder.builderRepository`: `index.docker.io/<username>/kpack-builder`;
-   `global.containerRepositoryPrefix`: `index.docker.io/<username>/`;

If `$KORIFI_NAMESPACE` doesn't exist yet, you can add the `--create-namespace` flag to the `helm` invocation.

## Post-install Configuration

Yon can skip this section.

## Test Korifi

No changes here, follow the [common instructions](./INSTALL.md#test-korifi).
When running `cf login`, make sure you select the entry associated to your kind cluster (`kind-kind` by default).
