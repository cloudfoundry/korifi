> **Warning**
> Make sure you are using the correct version of these instructions by using the link in the release notes for the version you're trying to install. If you're not sure, check our [latest release](https://github.com/cloudfoundry/korifi/releases/latest).

# Korifi installation guide

The following lines will guide you through the process of deploying a [released version](https://github.com/cloudfoundry/korifi/releases) of [Korifi](https://github.com/cloudfoundry/korifi). This document is written with the intent to act both as a runbook as well as a starting point in understanding basic concepts of Korifi and its dependencies.

## Prerequisites

- Tools:
  - [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl);
  - [cf](https://docs.cloudfoundry.org/cf-cli/install-go-cli.html) CLI version 8.5 or greater;
  - [Helm](https://helm.sh/docs/intro/install/).
- Resources:
  - Kubernetes cluster of one of the [upstream releases](https://kubernetes.io/releases/);
  - Container Registry on which you have write permissions.

This document was tested on:

- [EKS](https://aws.amazon.com/eks/), using GCP's [Artifact Registry](https://cloud.google.com/artifact-registry);
- [GKE](https://cloud.google.com/kubernetes-engine), using GCP's [Artifact Registry](https://cloud.google.com/artifact-registry);
- [kind](https://kind.sigs.k8s.io/), using [DockerHub](https://hub.docker.com/) (see [_Install Korifi on kind_](./INSTALL_kind.md)).

## Initial setup

The following environment variables will be needed throughout this guide:

- `ROOT_NAMESPACE`: the namespace at the root of the Korifi org and space hierarchy. The default value is `cf`.
- `ADMIN_USERNAME`: the name of the Kubernetes user who will have CF admin privileges on the Korifi installation. For security reasons, you should choose or create a user that is different from your cluster admin user. To provision new users, follow the user management instructions specific for your cluster's [authentication configuration](https://kubernetes.io/docs/reference/access-authn-authz/authentication/) or create a [new (short-lived) client certificate for user authentication](https://kubernetes.io/docs/reference/access-authn-authz/certificate-signing-requests/#normal-user).
- `BASE_DOMAIN`: the base domain used by both the Korifi API and, by default, all apps running on Korifi.

Here are the example values we'll use in this guide:

```sh
export ROOT_NAMESPACE="cf"
export ADMIN_USERNAME="cf-admin"
export BASE_DOMAIN="korifi.example.org"
```

## Configuration

You will need to update the following configuration files:

- `korifi-kpack-image-builder.yml`

  Change the value of `kpackImageTag` in the `korifi-kpack-build-config` `ConfigMap`. `kpackImageTag` specifies the tag prefix used for the images built by Korifi. Its hostname should point to your container registry and its path should be valid for the registry.

  ```yaml
  kpackImageTag: us-east4-docker.pkg.dev/vigilant-card-347116/korifi/droplets
  ```

  - If using **DockerHub**, `kpackImageTag` should be `index.docker.io/<username>`.
  - If using **GCP**, `kpackImageTag` should be `gcr.io/<project-id>/droplets`.

- `korifi-api.yml`

  - Change the following values in the `korifi-api-config-*` `ConfigMap`.

    - `packageRegistryBase` specifies the tag prefix used for the source packages uploaded to Korifi. Its hostname should point to your container registry and its path should be valid for the registry.
      - If using **DockerHub**, `packageRegistryBase` should be `index.docker.io/<username>`.
      - If using **GCP**, `packageRegistryBase` should be `gcr.io/<project-id>/packages`.
    - `externalFQDN` is the domain name that will be used by the Korifi API, and is usually of the format `api.$BASE_DOMAIN`.
    - `defaultDomainName` is the default base domain name for the apps deployed by Korifi, and is usually of the format `apps.$BASE_DOMAIN`.

    ```yaml
    packageRegistryBase: us-east4-docker.pkg.dev/vigilant-card-347116/korifi/packages
    externalFQDN: api.korifi.example.org
    defaultDomainName: apps.korifi.example.org
    ```

  - Change `spec.virtualhost.fqdn` in the `korifi-api-proxy` `HTTPProxy`. It should be the same as `externalFQDN` above.

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
        fqdn: api.korifi.example.org
        # ...
    ```

### Registries with Custom CA

See [_Using container registry signed by custom CA_](docs/using-container-registry-signed-by-custom-ca.md).

## Root namespace setup

Create the following resources:

```sh
cat <<EOF | kubectl create -f -
---
apiVersion: v1
kind: Namespace
metadata:
  name: $ROOT_NAMESPACE
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: default-admin-binding
  namespace: $ROOT_NAMESPACE
  annotations:
    cloudfoundry.org/propagate-cf-role: "true"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-admin
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: User
    name: $ADMIN_USERNAME
EOF
```

## Dependencies

### Cert-Manager

[Cert-Manager](https://cert-manager.io) allows us to automatically create internal certificates within the cluster. Follow the [instructions](https://cert-manager.io/docs/installation/) to install the latest version.

### Kpack

[Kpack](https://github.com/pivotal/kpack) is used to build runnable applications from source code using [Cloud Native Buildpacks](https://buildpacks.io/). Follow the [instructions](https://github.com/pivotal/kpack/blob/main/docs/install.md) to install the latest version.

#### Container registry credentials `Secret`

Use the following command to create a `Secret` that Kpack will use to connect to your container registry:

```sh
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="<your-container-registry-username>" \
    --docker-password="<your-container-registry-password>" \
    --docker-server="<your-container-registry-hostname-and-port>" \
    --namespace "$ROOT_NAMESPACE"
```

Make sure the value of `--docker-server` is a valid [URI authority](https://datatracker.ietf.org/doc/html/rfc3986#section-3.2).

- If using **DockerHub**:
  - `--docker-server` should be `https://index.docker.io/v1/`;
  - `--docker-username` should be your DockerHub user;
  - `--docker-password` can be either your DockerHub password or a [generated personal access token](https://hub.docker.com/settings/security?generateToken=true).
- If using **GCR**:
  - `--docker-server` should be `gcr.io`;
  - `--docker-username` should be `_json_key`;
  - `--docker-password` should be the JSON-formatted access token for a service account that has permission to manage images in GCR.

#### `ServiceAccount`

Use the following command to create a `ServiceAccount` associated to the `Secret` you have just created:

```sh
cat <<EOF | kubectl create -f -
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kpack-service-account
  namespace: $ROOT_NAMESPACE
  annotations:
    cloudfoundry.org/propagate-service-account: "true"
secrets:
  - name: image-registry-credentials
imagePullSecrets:
  - name: image-registry-credentials
EOF
```

#### `ClusterStore`

Follow the [documentation](https://github.com/pivotal/kpack/blob/main/docs/store.md) to create a `ClusterStore` for your cluster.

#### `ClusterStack`:

Follow the [documentation](https://github.com/pivotal/kpack/blob/main/docs/stack.md) to create a `ClusterStack` for your cluster.

#### `ClusterBuilder`

Follow the [documentation](https://github.com/pivotal/kpack/blob/main/docs/builders.md#cluster-builders) to create a `ClusterBuilder` for your cluster. Make sure that:

- `metadata.name` matches `clusterBuilderName` in `korifi-kpack-image-builder.yml`;
- `spec.tag` points to your container registry:
  - if using **DockerHub**, it should be `index.docker.io/<username>/korifi-cluster-builder`;
  - if using **GCP**, it should be `gcr.io/<project-id>/korifi-cluster-builder`;
- `spec.stack` references to the previously created `ClusterStack`;
- `spec.store` references to the previously created `ClusterStore`;
- `spec.serviceAccountRef` references the previously created `ServiceAccount`.

### Contour

[Contour](https://projectcontour.io/) is our [ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/) controller. Follow the [instructions](https://projectcontour.io/getting-started/#install-contour-and-envoy) from the getting started guide to install the latest version.

### Metrics Server

We use the [Kubernetes Metrics Server](https://github.com/kubernetes-sigs/metrics-server) to implement [process stats](https://v3-apidocs.cloudfoundry.org/#get-stats-for-a-process).
Most Kubernetes distributions will come with `metrics-server` already installed. If yours does not, you should follow the [instructions](https://github.com/kubernetes-sigs/metrics-server#installation) to install it.

### Optional: Service Bindings Controller

We use the [Service Binding Specification for Kubernetes](https://github.com/servicebinding/spec) and its [controller reference implementation](https://github.com/servicebinding/runtime) to implement [Cloud Foundry service bindings](https://docs.cloudfoundry.org/devguide/services/application-binding.html) ([see this issue](https://github.com/cloudfoundry/cf-k8s-controllers/issues/462)). Follow the [instructions](https://github.com/servicebinding/runtime/releases/latest) to install the latest version.

## DNS

Create DNS entries for the Korifi API and for the apps running on Korifi. They should match the [configuration](#configuration):

- The Korifi API entry should match `externalFQDN`. In our example, that would be `api.korifi.example.org`.
- The apps entry should be a wildcard matching `defaultDomainName`: In our example, `*.apps.korifi.example.org`.

The DNS entries should point to the load balancer endpoint created by Contour when installed. To discover your endpoint, run:

```sh
kubectl get service envoy -n projectcontour -ojsonpath='{.status.loadBalancer.ingress[0]}'
```

It may take some time before the address is available. Retry this until you see a result.

The type of DNS records to create will differ based on the type of the endpoint: `ip` endpoints (e.g. the ones created by GKE) will need an `A` record, while `hostname` endpoints (e.g. on EKS) a `CNAME` record.

## Deploy Korifi

Just apply the files in the release:

```sh
kubectl apply -f korifi-controllers.yml
kubectl apply -f korifi-api.yml
kubectl apply -f korifi-job-task-runner.yml
kubectl apply -f korifi-kpack-image-builder.yml
kubectl apply -f korifi-statefulset-runner.yml
```

## TLS certificates

Generate TLS certificates for both the Korifi API and the apps running on Korifi, associated to [the DNS entries you created above](#dns).

Provide them to Korifi by creating the `korifi-api-ingress-cert` and the `korifi-workloads-ingress-cert` secrets:

```sh
kubectl -n korifi-api-system create secret tls korifi-api-ingress-cert --cert=<your-api-tls-cert-file> --key=<your-api-tls-key-file>
kubectl -n korifi-controllers-system create secret tls korifi-workloads-ingress-cert --cert=<your-workloads-tls-cert-file> --key=<your-workloads-tls-key-file>
```

The [`create_tls_secret` function in `scripts/common.sh`](https://github.com/cloudfoundry/korifi/blob/fd1aed6a8f406cb8d67cb5c214280e55db59901e/scripts/common.sh#L48-L91) shows how we do this for our development environments.

## Default CF Domain

Create the default `CFDomain` that will be used by all apps running on Korifi:

```sh
cat <<EOF | kubectl apply --namespace "$ROOT_NAMESPACE" -f -
apiVersion: korifi.cloudfoundry.org/v1alpha1
kind: CFDomain
metadata:
  name: default-domain
  namespace: $ROOT_NAMESPACE
spec:
  name: apps.$BASE_DOMAIN
EOF
```

## Test Korifi

```sh
cf api https://api.$BASE_DOMAIN --skip-ssl-validation
cf auth $ADMIN_USERNAME
cf create-org org1
cf create-space -o org1 space1
cf target -o org1
cd <directory of a test cf app>
cf push test-app
```
