# Terse Install Instructions

These are to be run in the root of the a cloned korifi repository, in the latest version of the main branch.

## General Setup

### Create a cluster

(Or just target an existing cluster)

E.g. to create a GKE cluster

```sh
CLUSTER_NAME=<YOUR CLUSTER NAME>
GCP_PROJECT=<YOUR GCP PROJECT ID>
GCP_ZONE=<GCP ZONE>
gcloud container clusters create $CLUSTER_NAME --zone=$GCP_ZONE --project $GCP_PROJECT
```

### Create the root namespace

Note, if you decide to call it something other than `cf`, check the `included-namespace-regex` configuration in HNC!
See `dependencies/hnc/cf/deployment.yaml`.

```sh
ROOT_NAMESPACE=cf
kubectl create namespace $ROOT_NAMESPACE
```

### Create a rolebinding for the administrator user

Note, this should not be your GCP user - that already has root permissions on the cluster.
Instead, create a dedicated user.

```sh
ADMIN_USERNAME=cf-admin
./scripts/create-new-user.sh $ADMIN_USERNAME
kubectl create rolebinding --namespace=$ROOT_NAMESPACE default-admin-binding --clusterrole=korifi-controllers-admin --user=$ADMIN_USERNAME
```

### Create a secret for container registry credentials

You need a container registry where you have write permissions. Any will work.
Just set up the credentials correctly in the following secret and make sure the
references to the registry in the API config, controllers config and
cluster-builder.yaml are consistent.

#### Dockerhub example

```sh
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="<DOCKER_USERNAME>" \
    --docker-password="<DOCKER_PASSWORD>" \
    --docker-server="https://index.docker.io/v1/" \
    --namespace $ROOT_NAMESPACE
```

#### GCP Artifact Registry example

Use the following for GCP's artifact registry, using service account credentials,
with the credential key optionally base64 encoded

```sh
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="_json_key" \
    --docker-password="<SERVICE_ACCOUNT_KEY_JSON>" \
    --docker-server="<ARTIFACT_REGISTRY_REGION>-docker.pkg.dev" \
    --namespace $ROOT_NAMESPACE
```

or, if base64 encoding the key json

```sh
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="_json_key_base64" \
    --docker-password="<BASE64_ENCODED_SERVICE_ACCOUNT_KEY_JSON>" \
    --docker-server="<ARTIFACT_REGISTRY_REGION>-docker.pkg.dev" \
    --namespace $ROOT_NAMESPACE
```

### Edit domain details in the API config

Edit `api/config/base/apiconfig/korifi_api_config.yaml`

```sh
sed -i "s/externalFQDN.*/externalFQDN: api.$BASE_DOMAIN/" api/config/base/apiconfig/korifi_api_config.yaml
sed -i "s/defaultDomainName.*/defaultDomainName: apps.$BASE_DOMAIN/" api/config/base/apiconfig/korifi_api_config.yaml
```

## Dependency Installation

### Install cert-manager

```sh
kubectl apply -f dependencies/cert-manager.yaml
```

### Install kpack

```sh
kubectl apply -f dependencies/kpack-release-0.5.2.yaml
```

#### Edit the cluster builder to set the container registry

Edit `dependencies/kpack/cluster_builder.yaml` to set `spec.tag`.
This must point to your container registry and needs 4 path segments!
E.g. for GCP artifact registry

```yaml
apiVersion: kpack.io/v1alpha2
kind: ClusterBuilder
metadata:
    name: cf-kpack-cluster-builder
spec:
    serviceAccountRef:
        name: kpack-service-account
        namespace: cf
    tag: europe-west6-docker.pkg.dev/cf-on-k8s-wg/installation-test/kpack
    stack: ...
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

### Install contour

```sh
kubectl apply -f dependencies/contour-1.19.1.yaml
```

### Install eirini

Deploy eirini

```sh
EIRINI_VERSION=0.2.0
./scripts/generate-eirini-certs-secret.sh "*.eirini-controller.svc"
EIRINI_WEBHOOK_CA_BUNDLE="$(kubectl get secret -n eirini-controller eirini-webhooks-certs -o jsonpath="{.data['tls\.ca']}")"
helm install eirini-controller https://github.com/cloudfoundry-incubator/eirini-controller/releases/download/v$EIRINI_VERSION/eirini-controller-$EIRINI_VERSION.tgz \
  --set "webhooks.ca_bundle=$EIRINI_WEBHOOK_CA_BUNDLE" \
  --set "workloads.default_namespace=$ROOT_NAMESPACE" \
  --set "controller.registry_secret_name=image-registry-credentials" \
  --namespace "eirini-controller"
```

### Install HNC

```sh
kubectl apply -k dependencies/hnc/cf
kubectl rollout status deployment/hnc-controller-manager -w -n hnc-system
```

and configure it to propagate secrets (in addition to the default roles and rolebindings)

```sh
kubectl patch hncconfigurations.hnc.x-k8s.io config --type=merge -p '{"spec":{"resources":[{"mode":"Propagate", "resource": "secrets"}]}}'
```

### Install the service bindings controller

```sh
kubectl apply -f dependencies/service-bindings-0.7.1.yaml
```

## Configuring DNS

We need DNS entries for the CF API, and for apps running on CF. They should not overlap.
So you can use entries like:

-   api.my-korifi.example.org for the API, and
-   \*.apps.my-korifi.example.org for the apps.

Contour creates a load balancer endpoint. We can find the external IP for that endpoint,
and configure two DNS entries for it appropriately. This might be a CNAME for a URL that EKS provides,
or an A record for the IP address provided by GKE.

It may take some time before the address is available. Retry this until you see a result.

```sh
EXTERNAL_IP="$(kubectl get service envoy -n projectcontour -ojsonpath='{.status.loadBalancer.ingress[0].ip}')"
echo $EXTERNAL_IP
```

To set up DNS entries in GCP Cloud DNS, for example, for this IP address

```sh
ZONE_NAME=<YOUR DNS ZONE>
ZONE_DOMAIN=<YOUR ZONE FULL DOMAIN> // omitting the trailing dot
BASE_DOMAIN=$CLUSTER_NAME.$ZONE_DOMAIN
gcloud dns record-sets create "api.$BASE_DOMAIN." --type=A --rrdatas=$EXTERNAL_IP --zone=$ZONE_NAME --project=$GCP_PROJECT
gcloud dns record-sets create "*.apps.$BASE_DOMAIN." --type=A --rrdatas=$EXTERNAL_IP --zone=$ZONE_NAME --project=$GCP_PROJECT
```

## Install Korifi

### Install controllers and api

Edit `controllers/config/base/controllersconfig/korifi_controllers_config.yaml`

-   Update `kpackImageTag` to set the prefix for droplet (staged app) images (e.g. `europe-west6-docker.pkg.dev/cf-on-k8s-wg/installation-test/droplets`)

Edit `api/config/base/apiconfig/korifi_api_config.yaml`

-   Update `packageRegistryBase` to set the prefix for package (app source code) images (e.g. `europe-west6-docker.pkg.dev/cf-on-k8s-wg/installation-test/packages`)

Edit `api/config/base/api_url_patch.yaml`

-   Update `value` to the api domain name (e.g. the value of api.$BASE_DOMAIN)

Then deploy

```sh
make deploy
```

Create secrets for the TLS certificates

```sh
source scripts/common.sh
create_tls_secret "korifi-workloads-ingress-cert" "korifi-controllers-system" "*.apps.$BASE_DOMAIN"
create_tls_secret "korifi-api-ingress-cert" "korifi-api-system" "api.$BASE_DOMAIN"
```

### Create the default CF Domain

```sh
cat <<EOF | kubectl apply --namespace cf -f -
apiVersion: networking.cloudfoundry.org/v1alpha1
kind: CFDomain
metadata:
  name: default-domain
  namespace: cf
spec:
  name: apps.$BASE_DOMAIN
EOF
```

## Test the installation

```
cf api https://api.$BASE_DOMAIN --skip-ssl-validation
cf login # and select the cf-admin entry
cf create-org org1
cf create-space -o org1 space1
cf target -o org1
cd <directory of a test cf app>
cf push test-app
```
