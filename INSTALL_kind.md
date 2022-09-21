> **Warning**
> Make sure you are using the correct version of these instructions by using the link in the release notes for the version you're trying to install. If you're not sure, check our [latest release](https://github.com/cloudfoundry/korifi/releases/latest).

# Install Korifi on kind

This document integrates our [install instructions](./INSTALL.md) with specific tips to install Korifi locally using [kind](https://kind.sigs.k8s.io/).

## Cluster creation

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

## Initial setup

Export the following environment variables:

```sh
export ROOT_NAMESPACE="cf"
export ADMIN_USERNAME="cf-admin"
export BASE_DOMAIN="vcap.me"
```

You will need to create the `cf-admin` user: check [`scripts/create-new-user.sh`](./scripts/create-new-user.sh) for an example of how we do this in our development environments.

`vcap.me` will conveniently [resolve to `127.0.0.1`](https://www.nslookup.io/domains/vcap.me/dns-records), which is exactly what we need.

## Configuration

We recommend you use [DockerHub](https://hub.docker.com/) as your image registry. As specified by the instructions, you should use the following values in the configuration files:

- `korifi-kpack-image-builder.yml`

  ```yaml
  kpackImageTag: index.docker.io/<username>
  ```

- `korifi-api.yml`

  - `korifi-api-config-*` `ConfigMap`

    ```yaml
    packageRegistryBase: index.docker.io/<username>
    externalFQDN: api.vcap.me
    defaultDomainName: apps.vcap.me
    ```

  - `korifi-api-proxy` `HTTPProxy`

    ```yaml
    apiVersion: projectcontour.io/v1
    kind: HTTPProxy
    metadata:
      # ...
      name: korifi-api-proxy
      namespace: korifi-api-system
    spec:
      # ...
      virtualhost:
        fqdn: api.vcap.me
        # ...
    ```

## Root namespace setup

No changes here, follow the instructions.

## Dependencies

No changes here, follow the instructions. For the container registry credentials `Secret`, we recommend you [create an access token](https://hub.docker.com/settings/security?generateToken=true) on DockerHub.

## DNS

You can skip this section.

## Deploy Korifi

No changes here, follow the instructions.

## TLS certificates

No changes here, follow the instructions.

## Default CF Domain

No changes here, follow the instructions.

## Test Korifi

No changes here, follow the instructions.
