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

You don't need to export the environment variables. We wil use the following values:

- `cf` as our root namespace;
- `cf-admin` as our admin username: you will need to create this (check [`scripts/create-new-user.sh`](./scripts/create-new-user.sh) for an example of how we do this in our development environments);
- `vcap.me` as our base domain: it will conveniently [resolve to `127.0.0.1`](https://www.nslookup.io/domains/vcap.me/dns-records), which is exactly what we need.

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

As per the instructions, create the following:

```yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name: cf
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: default-admin-binding
  namespace: cf
  annotations:
    cloudfoundry.org/propagate-cf-role: "true"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-admin
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: User
    name: cf-admin
```

### Container registry credentials

We recommend you [create an access token](https://hub.docker.com/settings/security?generateToken=true) on DockerHub. Then run the following command, as described in the instructions:

```sh
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="<your-dockerhub-username>" \
    --docker-password="<your-dockerhub-access-token>" \
    --docker-server="https://index.docker.io/v1/" \
    --namespace cf
```

## Dependencies

No changes here, follow the instructions.

## DNS

You can skip this section.

## Deploy Korifi

No changes here, follow the instructions.

## TLS certificates

Given you don't have any DNS records, you want to use the following values for `CN`/`SAN`:

- for the API, use `localhost`;
- for the apps, use `*.vcap.me`.

## Default CF Domain

As per the instructions:

```sh
cat <<EOF | kubectl apply --namespace cf -f -
apiVersion: korifi.cloudfoundry.org/v1alpha1
kind: CFDomain
metadata:
  name: default-domain
  namespace: cf
spec:
  name: apps.vcap.me
EOF
```

## Test Korifi

```sh
cf api https://api.vcap.me --skip-ssl-validation
cf login # select the cf-admin entry
cf create-org org1
cf create-space -o org1 space1
cf target -o org1
cd <directory of a test cf app>
cf push test-app
```
