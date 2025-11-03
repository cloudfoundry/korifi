> **Warning**
> Make sure you are using the correct version of these instructions by using the link in the release notes for the version you're trying to install. If you're not sure, check our [latest release](https://github.com/cloudfoundry/korifi/releases/latest).

# Korifi installation guide

The following lines will guide you through the process of deploying a [released version](https://github.com/cloudfoundry/korifi/releases) of [Korifi](https://github.com/cloudfoundry/korifi).

## Prerequisites

-   Tools:
    -   [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl);
    -   [cf](https://docs.cloudfoundry.org/cf-cli/install-go-cli.html) CLI version 8.5 or greater;
    -   [Helm](https://helm.sh/docs/intro/install/).
-   Resources:
    -   Kubernetes cluster of one of the [upstream releases](https://kubernetes.io/releases/);
    -   Container Registry on which you have write permissions.

This document was tested on:

-   [EKS](https://aws.amazon.com/eks/), using AWS' [Elastic Container Registry (ECR)](https://aws.amazon.com/ecr/) (see [_Install Korifi on EKS_](./INSTALL.EKS.md));
-   [GKE](https://cloud.google.com/kubernetes-engine), using GCP's [Artifact Registry](https://cloud.google.com/artifact-registry);
-   [kind](https://kind.sigs.k8s.io/): see [_Install Korifi on kind_](./INSTALL.kind.md).

## Initial setup

The following environment variables will be needed throughout this guide:

-   `ROOT_NAMESPACE`: the namespace at the root of the Korifi org and space hierarchy. The default value is `cf`.
-   `KORIFI_NAMESPACE`: the namespace in which Korifi will be installed.
-   `ADMIN_USERNAME`: the name of the Kubernetes user who will have CF admin privileges on the Korifi installation. For security reasons, you should choose or create a user that is different from your cluster admin user. To provision new users, follow the user management instructions specific for your cluster's [authentication configuration](https://kubernetes.io/docs/reference/access-authn-authz/authentication/) or create a [new (short-lived) client certificate for user authentication](https://kubernetes.io/docs/reference/access-authn-authz/certificate-signing-requests/#normal-user).
-   `BASE_DOMAIN`: the base domain used by both the Korifi API and, by default, all apps running on Korifi.
-   `GATEWAY_CLASS_NAME`: the name of the Gateway API gatewayclass ([see contour section](#contour)).

Here are the example values we'll use in this guide:

```sh
export ROOT_NAMESPACE="cf"
export KORIFI_NAMESPACE="korifi"
export KORIFI_GATEWAY_NAMESPACE="korifi-gateway"
export ADMIN_USERNAME="cf-admin"
export BASE_DOMAIN="korifi.example.org"
export GATEWAY_CLASS_NAME="contour"
```

### Free Dockerhub accounts

DockerHub allows only one private repository per free account. In case the DockerHub account you configure Korifi with has the `private` [default repository privacy](https://hub.docker.com/settings/default-privacy) enabled, then Korifi would only be able to create a single repository and would get `UNAUTHORIZED: authentication required` error when trying to push to a subsequent repository. This could either cause build errors during `cf push`, or the Kpack cluster builder may never become ready. Therefore you should either set the default repository privacy to `public`, or upgrade your DockerHub subscription plan. As of today, the `Pro` subscription plan provides unlimited private repositories.

## Dependencies

### Cert-Manager (optional)

[Cert-Manager](https://cert-manager.io) allows us to automatically generate certificates within the cluster. It is required if you want Korifi to generate self-signed certificates for API and workload ingress or for internal use, like webhooks. In order to do this you have to set the `generateIngressCertificates` and/or `generateInternalCertificates` values to `true`. Follow the [instructions](https://cert-manager.io/docs/installation/) to install the latest version.

### Kpack

[Kpack](https://github.com/buildpacks-community/kpack) is used to build runnable applications from source code using [Cloud Native Buildpacks](https://buildpacks.io/). Follow the [instructions](https://github.com/buildpacks-community/kpack/blob/main/docs/install.md) to install the [latest version](https://github.com/buildpacks-community/kpack/releases/latest).

The Helm chart will create an example Kpack `ClusterBuilder` (with the associated `ClusterStore` and `ClusterStack`) by default. To use your own `ClusterBuilder`, specify the `kpackImageBuilder.clusterBuilderName` value. See the [Kpack documentation](https://github.com/buildpacks-community/kpack/blob/main/docs/builders.md) for details on how to set up your own `ClusterBuilder`.

### Contour

[Contour](https://projectcontour.io/) is our [ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/) controller. Contour implements the [Gateway API](https://gateway-api.sigs.k8s.io/). There are two ways to deploy Contour with Gateway API support: static provisioning and dynamic provisioning.

#### Static Provisioning

Follow the static provisioning [instructions](https://projectcontour.io/docs/1.26/config/gateway-api/#static-provisioning) from the Gateway API support guide to install the latest version. Note that as part of the Contour installation you have to create a gatewayclass with name `$GATEWAY_CLASS_NAME`:

```bash
kubectl apply -f - <<EOF
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: $GATEWAY_CLASS_NAME
spec:
  controllerName: projectcontour.io/gateway-controller
EOF
```

This gatewayclass name is a parameter of the helm chart installing korifi. The helm chart is going to define a gateway that will be used for all korifi ingress traffic.

#### Dynamic Provisioning

Follow the dynamic provisioning [instructions](https://projectcontour.io/docs/1.26/config/gateway-api/#dynamic-provisioning) from the Gateway API support guide to install the latest version.

  - Note that as part of the Contour installation you have to create a gatewayclass with name `$GATEWAY_CLASS_NAME`:
    ```bash
    kubectl apply -f - <<EOF
    kind: GatewayClass
    apiVersion: gateway.networking.k8s.io/v1beta1
    metadata:
      name: $GATEWAY_CLASS_NAME
    spec:
      controllerName: projectcontour.io/gateway-controller
    EOF
    ```
  - You DO NOT need to create a gateway as per the instructions. The Korifi helm chart defines a gateway that will be used for all korifi ingress traffic. The gateway will be created in the `$KORIFI_GATEWAY_NAMESPACE` namespace.

### Metrics Server

We use the [Kubernetes Metrics Server](https://github.com/kubernetes-sigs/metrics-server) to implement [process stats](https://v3-apidocs.cloudfoundry.org/#get-stats-for-a-process).
Most Kubernetes distributions will come with `metrics-server` already installed. If yours does not, you should follow the [instructions](https://github.com/kubernetes-sigs/metrics-server#installation) to install it.

## Pre-install configuration

### Namespace creation

Create the root, korifi and gateway namespaces:

```sh
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: $ROOT_NAMESPACE
  labels:
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/enforce: restricted
EOF

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: $KORIFI_NAMESPACE
  labels:
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/enforce: restricted
EOF

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: $KORIFI_GATEWAY_NAMESPACE
EOF
```

### Container registry credentials `Secret`

> **Warning**
> This is not required when using ECR on an EKS deployment.

Use the following command to create a `Secret` that Korifi and Kpack will use to connect to your container registry:

```sh
kubectl --namespace "$ROOT_NAMESPACE" create secret docker-registry image-registry-credentials \
    --docker-username="<your-container-registry-username>" \
    --docker-password="<your-container-registry-password>" \
    --docker-server="<your-container-registry-hostname-and-port>"
```

Make sure the value of `--docker-server` is a valid [URI authority](https://datatracker.ietf.org/doc/html/rfc3986#section-3.2).

-   If using **DockerHub**:
    -   `--docker-server` should be omitted;
    -   `--docker-username` should be your DockerHub user;
    -   `--docker-password` can be either your DockerHub password or a [generated personal access token](https://hub.docker.com/settings/security?generateToken=true).
-   If using **Google Artifact Registry**:
    -   `--docker-server` should be `<region>-docker.pkg.dev`;
    -   `--docker-username` should be `_json_key`;
    -   `--docker-password` should be the JSON-formatted access token for a service account that has permission to manage images in Google Artifact Registry.

### TLS certificates

Korifi uses two kinds of TLS certificates:
1. Ingress certificates: if `generateIngressCertificates` is set to `true`, Korifi will generate self-signed certificates and use them for API and workloads ingress.
1. Internal certificates: if `generateInternalCertificates` is set to `true` (the default value), Korifi will generate self-signed certificates and use them for securing internal communication, like webhooks.

A running cert-manager is a prerequisite for generating self-signed certificates.

If you want to generate certificates yourself, set `generateIngressCertificates` and `generateInternalCertificates` to `false` and point to your own certificates using the following values:

1. `api.apiServer.ingressCertSecret`: the name of the `Secret` in the `$KORIFI_NAMESPACE` namespace containing the API ingress certificate; defaults to `korifi-api-ingress-cert`.
1. `api.apiServer.internalCertSecret`: the name of the `Secret` in the `$KORIFI_NAMESPACE` namespace containing a certificate that is valid for `korifi-api-svc.korifi.svc.cluster.local` (the internal dns of the korifi api); defaults to `korifi-api-internal-cert`.
1. `controllers.workloadsTLSSecret`: the name of the `Secret` in the `$KORIFI_NAMESPACE` namespace containing the workload ingress certificate; defaults to `korifi-workloads-ingress-cert`.
1. `controllers.webhookCertSecret`: the name of the `Secret` in the `$KORIFI_NAMESPACE` namespace containing the webhook certificate for the controllers deployment; defaults to `korifi-controllers-webhook-cert`.


### Container registry Certificate Authority

Korifi can be configured to use a custom Certificate Authority when contacting the container registry. To do so, first create a `Secret` containing the CA certificate:

```sh
kubectl --namespace "$KORIFI_NAMESPACE" create secret generic <registry-ca-secret-name> \
    --from-file=ca.crt=</path/to/ca-certificate>
```

You can then specify the `<registry-ca-secret-name>` using the `containerRegistryCACertSecret`.

> **Warning**
> Kpack does not support self-signed/internal CA configuration out of the box (see [buildpacks-community/kpack#207](https://github.com/buildpacks-community/kpack/issues/207)).
> In order to make Kpack trust your CA certificate, you will have to inject it in both the Kpack controller and the Kpack build pods.
> * The [`kpack-controller` `Deployment`](https://github.com/buildpacks-community/kpack/blob/main/config/controller.yaml) can be modified to mount a `Secret` similar to the one created above: see the [Korifi API `Deployment`](https://github.com/cloudfoundry/korifi/blob/main/helm/korifi/api/deployment.yaml) for an example of how to do this.
> * For the build pods you can use the [cert-injection-webhook](https://github.com/vmware-tanzu/cert-injection-webhook), configured on the `kpack.io/build` label.

## Install Korifi

Korifi is distributed as a [Helm chart](https://helm.sh). See [_Customizing the Chart Before Installing_](https://helm.sh/docs/intro/using_helm/#customizing-the-chart-before-installing) for details on how to specify values when installing a Helm chart.

For example:

```sh
helm install korifi https://github.com/cloudfoundry/korifi/releases/download/v<VERSION>/korifi-<VERSION>.tgz \
    --namespace="$KORIFI_NAMESPACE" \
    --set=generateIngressCertificates=true \
    --set=rootNamespace="$ROOT_NAMESPACE" \
    --set=adminUserName="$ADMIN_USERNAME" \
    --set=api.apiServer.url="api.$BASE_DOMAIN" \
    --set=defaultAppDomainName="apps.$BASE_DOMAIN" \
    --set=containerRepositoryPrefix=europe-docker.pkg.dev/my-project/korifi/ \
    --set=kpackImageBuilder.builderRepository=europe-docker.pkg.dev/my-project/korifi/kpack-builder \
    --set=networking.gatewayClass=$GATEWAY_CLASS_NAME \
    --set=networking.gatewayNamespace=$KORIFI_GATEWAY_NAMESPACE \
    --wait
```

`containerRepositoryPrefix` is used to determine the container repository for the package and droplet images produced by Korifi.
In particular, the app GUID and image type (`packages` or `droplets`) are appended to form the name of the repository.
For example:

| Registry                          | containerRepositoryPrefix                                    | Resultant Image Ref                                                            | Notes                                                                                                    |
| --------------------------------- | ------------------------------------------------------------ | ------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------- |
| Azure Container Registry          | `<projectID>.azurecr.io/foo/bar/korifi-`                     | `<projectID>.azurecr.io/foo/bar/korifi-<appGUID>-packages`                     | Repositories are created dynamically during push by ACR                                                  |
| DockerHub                         | `index.docker.io/<dockerOrganisation>/`                      | `index.docker.io/<dockerOrganisation>/<appGUID>-packages`                      | Docker does not support nested repositories                                                              |
| Amazon Elastic Container Registry | `<projectID>.dkr.ecr.<region>.amazonaws.com/foo/bar/korifi-` | `<projectID>.dkr.ecr.<region>.amazonaws.com/foo/bar/korifi-<appGUID>-packages` | Korifi will create the repository before pushing, as dynamic repository creation is not posssible on ECR |
| Google Artifact Registry          | `<region>-docker.pkg.dev/<projectID>/foo/bar/korifi-`        | `<region>-docker.pkg.dev/<projectID>/foo/bar/korifi-<appGUID>-packages`        | The `foo` repository must already exist in GAR                                                           |
| Google Container Registry         | `gcr.io/<projectID>/foo/bar/korifi-`                         | `gcr.io/<projectID>/foo/bar/korifi-<appGUID>-packages`                         | Repositories are created dynamically during push by GCR                                                  |
| GitHub Container Registry         | `ghcr.io/<githubUserName>/foo/bar/korifi-`                   | `ghcr.io/<githubUserName>/foo/bar/korifi-<appGUID>-package`                    | Repositories are created dynamically during push by GHCR                                                 |

The chart provides various other values that can be set. See [`README.helm.md`](./README.helm.md) for details.

### Configure an Authentication Proxy (optional)

If you are using an authentication proxy with your cluster to enable SSO, you must set the following chart values:

-   `api.authProxy.host`: IP address of your cluster's auth proxy;
-   `api.authProxy.caCert`: CA certificate of your cluster's auth proxy.

### Using a Custom Ingress Controller

Korifi leverages the Gateway API for networking. This means that it should be easy to switch to any Gateway API compatible Ingress Controller implementation (e.g. Istio).

## Post-install Configuration

### DNS

Create DNS entries for the Korifi API and for the apps running on Korifi. They should match the Helm values used to [deploy Korifi](#deploy-korifi):

-   The Korifi API entry should match the `api.apiServer.url` value. In our example, that would be `api.korifi.example.org`.
-   The apps entry should be a wildcard matching the `defaultAppDomainName` value. In our example, `*.apps.korifi.example.org`.

The DNS entries should point to the load balancer endpoint created by Contour when installed.

If you used static provisioning of a Contour gateway, discover your endpoint with:

```sh
kubectl get service envoy -n projectcontour -ojsonpath='{.status.loadBalancer.ingress[0]}'
```

If you used dynamic provisioning of a Contour gateway, discover your endpoint with:

```sh
kubectl get service envoy-korifi -n $KORIFI_GATEWAY_NAMESPACE -ojsonpath='{.status.loadBalancer.ingress[0]}'
```

It may take some time before the address is available. Retry this until you see a result.

The type of DNS records to create will differ based on the type of the endpoint: `ip` endpoints (e.g. the ones created by GKE) will need an `A` record, while `hostname` endpoints (e.g. on EKS) a `CNAME` record.

## Test Korifi

```sh
cf api https://api.$BASE_DOMAIN --skip-ssl-validation
cf login # choose the entry in the list associated to $ADMIN_USERNAME
cf create-org org1
cf create-space -o org1 space1
cf target -o org1
cd <directory of a test cf app>
cf push test-app
```
