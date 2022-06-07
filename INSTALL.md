> **Warning**
> Make sure you are using the correct version of these instructions by using the link in the release notes for the version you're trying to install. If you're not sure, check our [latest release](https://github.com/cloudfoundry/korifi/releases/latest).

# Introduction

The following lines will guide you through the process of deploying a [released version](https://github.com/cloudfoundry/korifi/releases) of [Korifi](https://github.com/cloudfoundry/korifi). This document is written with the intent to act both as a runbook as well as a starting point in understanding basic concepts of Korifi and its dependencies.

# Prerequisites

-   Tools
    -   [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
    -   [cf](https://docs.cloudfoundry.org/cf-cli/install-go-cli.html) CLI version 8.1 or greater
    -   [Helm](https://helm.sh/docs/intro/install/)
-   Resources
    -   Kubernetes cluster of one of the [upstream releases](https://kubernetes.io/releases/)
    -   Container Registry on which you have write permissions

This document was tested on both [EKS](https://aws.amazon.com/eks/) and [GKE](https://cloud.google.com/kubernetes-engine) using GCP's [Artifact Registry](https://cloud.google.com/artifact-registry).

# Initial setup

## Environment Variables

The following environment variables will be needed throughout this guide:

-   `ROOT_NAMESPACE`: the namespace at the root of the Korifi org and space hierarchy. The default value is `cf`.
-   `ADMIN_USERNAME`: the name of the Kubernetes user who will have admin privileges on the Korifi installation. For security reasons, you should create a dedicated user.
-   `BASE_DOMAIN`: the base domain used by both the Korifi API and, by default, all apps running on Korifi.

Here are the example values we'll use in this guide:

```sh
export ROOT_NAMESPACE="cf"
export ADMIN_USERNAME="cf-admin"
export BASE_DOMAIN="korifi.example.org"
```

## Configuration

You will need to update the following configuration files:

-   `dependencies/kpack/cluster_builder.yaml`

    `spec.tag` specifies the tag for the [CNB builder](https://buildpacks.io/docs/concepts/components/builder/) image used by Korifi. Its hostname should point to your container registry and its path should be [valid for the registry](#image-registry-configuration).

    ```yaml
    spec:
        tag: us-east4-docker.pkg.dev/vigilant-card-347116/korifi/kpack
    ```

-   `korifi-kpack-image-builder.yml`

    Change the value of `kpackImageTag` in the `korifi-kpack-build-config` `ConfigMap`. `kpackImageTag` specifies the tag prefix used for the images built by Korifi. Its hostname should point to your container registry and its path should be [valid for the registry](#image-registry-configuration).

    ```yaml
    kpackImageTag: us-east4-docker.pkg.dev/vigilant-card-347116/korifi/droplets
    ```

-   `korifi-api.yml`

    -   Change the following values in the `korifi-api-config-*` `ConfigMap`.

        -   `packageRegistryBase` specifies the tag prefix used for the source packages uploaded to Korifi. Its hostname should point to your container registry and its path should be [valid for the registry](#image-registry-configuration).
        -   `externalFQDN` is the domain name that will be used by the Korifi API, and is usually of the format `api.$BASE_DOMAIN`.
        -   `defaultDomainName` is the default base domain name for the apps deployed by Korifi, and is usually of the format `apps.$BASE_DOMAIN`.

        ```yaml
        packageRegistryBase: us-east4-docker.pkg.dev/vigilant-card-347116/korifi/packages
        externalFQDN: api.korifi.example.org
        defaultDomainName: apps.korifi.example.org
        ```

    -   Change `spec.virtualhost.fqdn` in the `korifi-api-proxy` `HTTPProxy`. It should be the same as `externalFQDN` above.

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

### Image Registry Configuration

#### DockerHub

- The cluster_builder.yaml `spec.tag` should be in the format `index.docker.io/<Username>/korifi-cluster-builder`.
- The korifi-kpack-image-builder.yml `kpackImageTag` should be in the format `index.docker.io/<Username>`.
  - It will be combined with the GUID of the Kpack Image resource to create the full URI of the droplet image for an application.
- The korifi-api.yml `packageRegistryBase` should be in the format `index.docker.io/<Username>`.
    - It will be combined with the GUID of the CFPackage resource to create the full URI of the package image for an application.

Note: DockerHub does not support nested directories so `kpackImageTag` and `packageRegistryBase` must point to the user's directory

#### GCR

- The cluster_builder.yaml `spec.tag` should be in the format `gcr.io/<project-id>/korifi-cluster-builder`.
- The korifi-kpack-image-builder.yml `kpackImageTag` should be in the format `gcr.io/<project-id>/droplets`.
    - It will be combined with the GUID of the Kpack Image resource to create the full URI of the droplet image for an application.
- The korifi-api.yml `packageRegistryBase` should be in the format `gcr.io/<project-id>/packages`.
    - It will be combined with the GUID of the CFPackage resource to create the full URI of the package image for an application.

Note: GCR supports nested directories, so you can organize your images as desired.

## Root namespace and admin role binding

Create the root namespace:

```sh
kubectl create namespace $ROOT_NAMESPACE
```

Bind `$ADMIN_USERNAME` to the admin role:

```sh
kubectl create rolebinding --namespace=$ROOT_NAMESPACE default-admin-binding --clusterrole=korifi-controllers-admin --user=$ADMIN_USERNAME
```

### Container registry credentials

Use the following command to create a `Secret` that Korifi will use to connect to your container registry:

```sh
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="<your-container-registry-username>" \
    --docker-password="<your-container-registry-password>" \
    --docker-server="<your-container-registry-hostname-and-port>" \
    --namespace "$ROOT_NAMESPACE"
```

Make sure the value of `--docker-server` is a valid [URI authority](https://datatracker.ietf.org/doc/html/rfc3986#section-3.2).

#### DockerHub

When using DockerHub,

- the docker-server should be https://index.docker.io/v1/
- the docker-username should be your DockerHub user
- the docker-password can be either your DockerHub password or a generated personal access token

#### GCR

When using GCR,

- the docker-server should be gcr.io
- the docker-username should be `"_json_key"`
- the docker-password should be the JSON-formatted access token for a service account that has permission to manage images in GCR

# Dependencies

## `cert-manager`

```sh
kubectl apply -f dependencies/cert-manager.yaml
```

## kpack

[kpack](https://github.com/pivotal/kpack) powers our source-to-image build process.

```sh
kubectl apply -f dependencies/kpack-release-0.5.2.yaml
```

Ensure the kpack CRDs are ready:

```sh
kubectl -n kpack wait --for condition=established --timeout=60s crd/clusterbuilders.kpack.io
kubectl -n kpack wait --for condition=established --timeout=60s crd/clusterstores.kpack.io
kubectl -n kpack wait --for condition=established --timeout=60s crd/clusterstacks.kpack.io
```

And then apply the kpack configuration for Korifi:

```sh
kubectl apply -f dependencies/kpack/service_account.yaml \
    -f dependencies/kpack/cluster_stack.yaml \
    -f dependencies/kpack/cluster_store.yaml \
    -f dependencies/kpack/cluster_builder.yaml
```

## Contour

[Contour](https://projectcontour.io/docs/v1.20.1/) is our [ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/) controller.

```sh
kubectl apply -f dependencies/contour-1.19.1.yaml
```

## eirini-controller

[`eirini-controller`](https://github.com/cloudfoundry/eirini-controller#what-is-eirini-controller) is responsible for running Korifi's workloads.

```sh
EIRINI_VERSION=0.3.0
helm install eirini-controller https://github.com/cloudfoundry/eirini-controller/releases/download/v$EIRINI_VERSION/eirini-controller-$EIRINI_VERSION.tgz \
  --set "workloads.default_namespace=$ROOT_NAMESPACE" \
  --set "controller.registry_secret_name=image-registry-credentials" \
  --create-namespace \
  --namespace "eirini-controller"
```

## Optional: Service Bindings Controller

We use the [Service Binding Specification for Kubernetes](https://github.com/servicebinding/spec) and its [controller reference implementation](https://github.com/servicebinding/service-binding-controller) to implement [Cloud Foundry service bindings](https://docs.cloudfoundry.org/devguide/services/application-binding.html) ([see this issue](https://github.com/cloudfoundry/cf-k8s-controllers/issues/462)).

```sh
kubectl apply -f dependencies/service-bindings-0.7.1.yaml
```

# DNS

Create DNS entries for the Korifi API and for the apps running on Korifi. They should match the [configuration](#configuration):

-   The Korifi API entry should match `externalFQDN`. In our example, that would be `api.korifi.example.org`.
-   The apps entry should be a wildcard matching `defaultDomainName`: In our example, `\*.apps.korifi.example.org`.

The DNS entries should point to the load balancer endpoint created by Contour when installed. To discover your endpoint, run:

```sh
kubectl get service envoy -n projectcontour -ojsonpath='{.status.loadBalancer.ingress[0]}'
```

It may take some time before the address is available. Retry this until you see a result.

The type of DNS records to create will differ based on the type of the endpoint: `ip` endpoints (e.g. the ones created by GKE) will need an A record, while `hostname` endpoints (e.g. on EKS) a CNAME record.

# Deploy Korifi

Just apply the files in the release:

```sh
kubectl apply -f korifi-api.yml
kubectl apply -f korifi-controllers.yml
kubectl apply -f korifi-kpack-image-builder.yml
```

## TLS certificates

Generate TLS certificates for both the Korifi API and the apps running on Korifi, associated to [the DNS entries you created above](#dns).

Provide them to Korifi by creating the `korifi-api-ingress-cert` and the `korifi-workloads-ingress-cert` secrets:

```sh
kubectl -n korifi-api-system create secret tls korifi-api-ingress-cert --cert=<your-api-tls-cert-file> --key=<your-api-tls-key-file>
kubectl -n korifi-controllers-system create secret tls korifi-workloads-ingress-cert --cert=<your-workloads-tls-cert-file> --key=<your-workloads-tls-key-file>
```

The [`create_tls_secret` function in `scripts/common.sh`](https://github.com/cloudfoundry/korifi/blob/fd1aed6a8f406cb8d67cb5c214280e55db59901e/scripts/common.sh#L48-L91) shows how we do this on our development environments.

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

# Test Korifi

```
cf api https://api.$BASE_DOMAIN --skip-ssl-validation
cf login # select the $ADMIN_USERNAME entry
cf create-org org1
cf create-space -o org1 space1
cf target -o org1
cd <directory of a test cf app>
cf push test-app
```
