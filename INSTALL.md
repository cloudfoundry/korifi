# Introduction
The following lines will guide you through the process of deploying [Korifi](https://github.com/cloudfoundry/korifi) using the documentation already in place i.e. the [readme](README.md) as well as external links to dependencies and their respective repositories/docs. This document is written with the intent to act both as a runbook as well as a starting point in understanding basic concepts of Korifi and its dependencies.

# Prerequisites

* Tools
  * [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
  * [cf](https://docs.cloudfoundry.org/cf-cli/install-go-cli.html) cli version 8.1 or greater
  * [Helm](https://helm.sh/docs/intro/install/)
* (Cloud) resources
  * Kubernetes cluster of one of the [upstream releases](https://kubernetes.io/releases/)
  * A Container Registry
  * Linux/mac local machine
* Code
  * `git clone https://github.com/cloudfoundry/korifi` (and `cd` into it)

Today, if anything, we are blessed (or cursed for that matter) with a number of options as far as the cloud resources go, from local/emulated to put-your-cloud-provider-of-choice-here. For brevity, this document was tested and based on the flavors bellow:

* **Kuberenetes Cluster**: AWS EKS/GCP GKE
* **Container Registry**: GCP's artifact registry
* **Local Machine**: linux/mac

# Dependencies

## Container registry

For GCP's artifactory container registry you wil need to either have or [create a Service Account](https://cloud.google.com/iam/docs/creating-managing-service-accounts) with the **Artifact Registry Writer** role. Visit that service account and through the **KEYS** tab, click **ADD KEY**, select json and it should download your service account details in a `.json` format.


## Variables

For quick reference you would like to set the following variables that will be used throughout the doc.

### GCP artifactory registry (update GCP_PASSWORD file location and GCP_REGION accordingly)

```sh
GCP_USERNAME=_json_key_base64
GCP_PASSWORD=$(base64 -b 0 /service/account/location/file.json)
GCP_REGION=us-east4
GCP_SERVER=$REGISTRY_REGION-docker.pkg.dev
```

### Cloudfoundry related:

```sh
ROOT_NAMESPACE=cf
ADMIN_USERNAME=cf-admin
```

Note: if you decide to call it something other than `cf`, check the `included-namespace-regex` configuration in HNC!
See `dependencies/hnc/cf/deployment.yaml`.

### Container Registry (either use the GCP variables already in place or replace with your own)

```sh
REGISTRY_USERNAME=$GCP_USERNAME
REGISTRY_PASSWORD=$GCP_PASSWORD
REGISTRY_SERVER=$GCP_SERVER
```

### Base Domain
This is the base domain of your cloudfoundry deployment

```sh
BASE_DOMAIN=korifi.example.org
```

### Important: 
Before your continue, make sure that the echo from your variables return a valid value configured:

```sh
echo $ROOT_NAMESPACE
echo $ADMIN_USERNAME
echo $REGISTRY_USERNAME
echo $REGISTRY_PASSWORD
echo $REGISTRY_SERVER
echo $BASE_DOMAIN
```
## Configuration Files

* `dependencies/kpack/cluster_builder.yaml`

Edit to set the `tag` line. This must point to your container registry and needs 4 path segments:

```yml
  tag: us-east4-docker.pkg.dev/vigilant-card-347116/korifi/kpack
```

* `controllers/config/base/controllersconfig/korifi_controllers_config.yaml`

Edit to set the `kpackImageTag` line. The 4 path segment rule applies:

```yml
kpackImageTag: us-east4-docker.pkg.dev/vigilant-card-347116/korifi/droplets
```

* `api/config/base/apiconfig/korifi_api_config.yaml`

Edit to set the `packageRegistryBase` line. The 4 path segment rule applies:

```yml
packageRegistryBase: us-east4-docker.pkg.dev/vigilant-card-347116/korifi/packages
```

Update the `externalFQDN` and `defaultDomainName` using the `$BASE_DOMAIN` set earlier

```sh
sed -i".bak" -e "s/externalFQDN.*/externalFQDN: api.$BASE_DOMAIN/" api/config/base/apiconfig/korifi_api_config.yaml
sed -i".bak" -e "s/defaultDomainName.*/defaultDomainName: apps.$BASE_DOMAIN/" api/config/base/apiconfig/korifi_api_config.yaml
```

* `api/config/base/api_url_patch.yaml`

edit to set `value` line to the api url.

```yml
value: api.korifi.example.org
```


# Update your k8s cluster

### Create the root namespace

```sh
kubectl create namespace $ROOT_NAMESPACE
```

### Create a rolebinding for the administrator user

```sh
./scripts/create-new-user.sh $ADMIN_USERNAME
kubectl create rolebinding --namespace=$ROOT_NAMESPACE default-admin-binding --clusterrole=korifi-controllers-admin --user=$ADMIN_USERNAME
```

Note, if that hangs, exit and you can try the rolebinding for your k8s admin user when the cluster has been deployed.

### Create a secret for container registry credentials

You need a container registry where you have write permissions. Any will work. Just set up the credentials correctly in the following secret and make sure the references to the registry in the API config, controllers config and cluster-builder.yaml are consistent.

```sh
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="$REGISTRY_USERNAME" \
    --docker-password="$REGISTRY_PASSWORD" \
    --docker-server="$REGISTRY_SERVER" \
    --namespace $ROOT_NAMESPACE
```

# Install dependencies

## Cert Manager
---
[cert-manager](https://cert-manager.io/docs/) _adds certificates and certificate issuers as resource types in Kubernetes clusters, and simplifies the process of obtaining, renewing and using those certificates._

```sh
kubectl apply -f dependencies/cert-manager.yaml
```

## kpack
---
[kpack](https://github.com/pivotal/kpack) _extends Kubernetes and utilizes unprivileged Kubernetes primitives to provide builds of OCI images as a platform implementation of Cloud Native Buildpacks (CNB)._

```sh
kubectl apply -f dependencies/kpack-release-0.5.2.yaml
```

Ensure the kpack CRDs are ready.

```sh
kubectl -n kpack wait --for condition=established --timeout=60s crd/clusterbuilders.kpack.io
kubectl -n kpack wait --for condition=established --timeout=60s crd/clusterstores.kpack.io
kubectl -n kpack wait --for condition=established --timeout=60s crd/clusterstacks.kpack.io
```

And then apply the kpack configuration for korifi

```sh
kubectl apply -f dependencies/kpack/service_account.yaml \
    -f dependencies/kpack/cluster_stack.yaml \
    -f dependencies/kpack/cluster_store.yaml \
    -f dependencies/kpack/cluster_builder.yaml
```

## contour
---
[Contour](https://projectcontour.io/docs/v1.20.1/) _is an Ingress controller for Kubernetes that works by deploying the Envoy proxy as a reverse proxy and load balancer._

```sh
kubectl apply -f dependencies/contour-1.19.1.yaml
```

## Eirini-Controller
---
[Eirini Controller](https://github.com/cloudfoundry/eirini-controller#what-is-eirini-controller) _is a Kubernetes controller that aims to enable Cloud Foundry to deploy applications as Pods on a Kubernetes cluster. It brings the CF model to Kubernetes by definig well known Diego abstractions such as Long Running Processes (LRPs) and Tasks as custom Kubernetes resources._

```sh
EIRINI_VERSION=0.2.0
./scripts/generate-eirini-certs-secret.sh "*.eirini-controller.svc"
EIRINI_WEBHOOK_CA_BUNDLE="$(kubectl get secret -n eirini-controller eirini-webhooks-certs -o jsonpath="{.data['tls\.ca']}")"
helm install eirini-controller https://github.com/cloudfoundry-incubator/eirini-controller/releases/download/v$EIRINI_VERSION/eirini-controller-$EIRINI_VERSION.tgz \
  --set "webhooks.ca_bundle=$EIRINI_WEBHOOK_CA_BUNDLE" \
  --set "workloads.default_namespace=$ROOT_NAMESPACE" \
  --set "controller.registry_secret_name=image-registry-credentials" \
  --create-namespace \
  --namespace "eirini-controller"
```

## Hierarchical Namespaces Controller
---
[Hierarchical namespaces](https://github.com/kubernetes-sigs/hierarchical-namespaces#the-hierarchical-namespace-controller-hnc)
>Mmake it easier to share your cluster by making namespaces more powerful. For example, you can create additional namespaces under your team's namespace, even if you don't have cluster-level permission to create namespaces, and easily apply policies like RBAC and Network Policies across all namespaces in your team (e.g. a set of related microservices).

```sh
kubectl apply -k dependencies/hnc/cf
kubectl rollout status deployment/hnc-controller-manager -w -n hnc-system
```

and configure it to propagate secrets (in addition to the default roles and rolebindings)

```sh
kubectl patch hncconfigurations.hnc.x-k8s.io config --type=merge -p '{"spec":{"resources":[{"mode":"Propagate", "resource": "secrets"}]}}'
```

## Optional: Install Service Bindings Controller
---
Cloud Native Buildpacks and other app frameworks (such as [Spring Cloud Bindings](https://github.com/spring-cloud/spring-cloud-bindings)) are adopting the [K8s ServiceBinding spec](https://github.com/servicebinding/spec#workload-projection) model of volume mounted secrets.
We currently are providing apps access to these via the `VCAP_SERVICES` environment variable ([see this issue](https://github.com/cloudfoundry/cf-k8s-controllers/issues/462)) for backwards compatibility reasons.
We would also want to support the newer developments in the ServiceBinding ecosystem as well.

```sh
kubectl apply -f dependencies/service-bindings-0.7.1.yaml
```

# DNS

We need DNS entries for the CF API, and for apps running on CF. They should not overlap. So you can use entries like:

- api.my-korifi.example.org for the API, and
- \*.apps.my-korifi.example.org for the apps.

Contour creates a load balancer endpoint. We can find the external IP for that endpoint, and configure two DNS entries for it appropriately. This might be a CNAME for a URL that EKS provides, or an A record for the IP address provided by GKE. Use the echo output below respectively.

It may take some time before the address is available. Retry this until you see a result.

For GCP

```sh
EXTERNAL_IP="$(kubectl get service envoy -n projectcontour -ojsonpath='{.status.loadBalancer.ingress[0].ip}')"
echo $EXTERNAL_IP
```

For AWS

```sh
EXTERNAL_HOSTNAME="$(kubectl get service envoy -n projectcontour -ojsonpath='{.status.loadBalancer.ingress[0].hostname}')"
echo $EXTERNAL_HOSTNAME
```

# Deploy Korifi

```sh
make deploy
```

## Create secrets for the TLS certificates

```sh
source scripts/common.sh
create_tls_secret "korifi-workloads-ingress-cert" "korifi-controllers-system" "*.apps.$BASE_DOMAIN"
create_tls_secret "korifi-api-ingress-cert" "korifi-api-system" "api.$BASE_DOMAIN"
```

## Create the default CF Domain

```sh
cat <<EOF | kubectl apply --namespace cf -f -
apiVersion: networking.cloudfoundry.org/v1alpha1
kind: CFDomain
metadata:
  name: default-domain
  namespace: $ROOT_NAMESPACE
spec:
  name: apps.$BASE_DOMAIN
EOF
```

## Post deploy tasks

If the rolebinding for the created administrator user failed earlier, you may use the one provided by your cloud provider. To do so, use `cf api https://api.$BASE_DOMAIN --skip-ssl-validation` and authenticate using `cf login` with the option provided from your cloud provider. Once logged in it will return the user currently in use. Update the `ADMIN_USERNAME` variable and run the rolebinding again:

```
ADMIN_USERNAME=kubernetes-admin
kubectl create rolebinding --namespace=$ROOT_NAMESPACE default-admin-binding --clusterrole=korifi-controllers-admin --user=$ADMIN_USERNAME
```

# Test Korifi

```
cf api https://api.$BASE_DOMAIN --skip-ssl-validation
cf login # and select the cf-admin entry
cf create-org org1
cf create-space -o org1 space1
cf target -o org1
cd <directory of a test cf app>
cf push test-app
```