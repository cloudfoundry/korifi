# Introduction

In the following lines we will go through each step required to deploy [Korifi](https://github.com/cloudfoundry/korifi) using the documentation already in place and provide a list of commands as a runbook along with additions that may come up during the process.

# Prerequisites

* [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
* Kubernetes cluster of one of the [upstream releases](https://kubernetes.io/releases/)
* container registry - in this example we are using dockerhub
* [cf](https://docs.cloudfoundry.org/cf-cli/install-go-cli.html) cli version 8.1 or greater
* [Helm](https://helm.sh/docs/intro/install/)
* `git clone https://github.com/cloudfoundry/cf-k8s-controllers`

# Initial setup

## Create the `cf` root namespace

```sh
kubectl create namespace cf
```

## Bind your admin user

(Describe what the admin user should be)

```sh
kubectl create rolebinding --namespace=cf default-admin-binding --clusterrole=korifi-controllers-admin --user="$YOUR_CF_ADMIN_USER"
```

# Dependencies

## cert-manager

[cert-manager](https://cert-manager.io/docs/) _adds certificates and certificate issuers as resource types in Kubernetes clusters, and simplifies the process of obtaining, renewing and using those certificates._

### Install

```sh
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.5.3/cert-manager.yaml
```

<details><summary>example output</summary><p>
```
customresourcedefinition.apiextensions.k8s.io/certificaterequests.cert-manager.io created
customresourcedefinition.apiextensions.k8s.io/certificates.cert-manager.io created
customresourcedefinition.apiextensions.k8s.io/challenges.acme.cert-manager.io created
customresourcedefinition.apiextensions.k8s.io/clusterissuers.cert-manager.io created
customresourcedefinition.apiextensions.k8s.io/issuers.cert-manager.io created
customresourcedefinition.apiextensions.k8s.io/orders.acme.cert-manager.io created
namespace/cert-manager created
serviceaccount/cert-manager-cainjector created
serviceaccount/cert-manager created
serviceaccount/cert-manager-webhook created
clusterrole.rbac.authorization.k8s.io/cert-manager-cainjector created
clusterrole.rbac.authorization.k8s.io/cert-manager-controller-issuers created
clusterrole.rbac.authorization.k8s.io/cert-manager-controller-clusterissuers created
clusterrole.rbac.authorization.k8s.io/cert-manager-controller-certificates created
clusterrole.rbac.authorization.k8s.io/cert-manager-controller-orders created
clusterrole.rbac.authorization.k8s.io/cert-manager-controller-challenges created
clusterrole.rbac.authorization.k8s.io/cert-manager-controller-ingress-shim created
clusterrole.rbac.authorization.k8s.io/cert-manager-view created
clusterrole.rbac.authorization.k8s.io/cert-manager-edit created
clusterrole.rbac.authorization.k8s.io/cert-manager-controller-approve:cert-manager-io created
clusterrole.rbac.authorization.k8s.io/cert-manager-controller-certificatesigningrequests created
clusterrole.rbac.authorization.k8s.io/cert-manager-webhook:subjectaccessreviews created
clusterrolebinding.rbac.authorization.k8s.io/cert-manager-cainjector created
clusterrolebinding.rbac.authorization.k8s.io/cert-manager-controller-issuers created
clusterrolebinding.rbac.authorization.k8s.io/cert-manager-controller-clusterissuers created
clusterrolebinding.rbac.authorization.k8s.io/cert-manager-controller-certificates created
clusterrolebinding.rbac.authorization.k8s.io/cert-manager-controller-orders created
clusterrolebinding.rbac.authorization.k8s.io/cert-manager-controller-challenges created
clusterrolebinding.rbac.authorization.k8s.io/cert-manager-controller-ingress-shim created
clusterrolebinding.rbac.authorization.k8s.io/cert-manager-controller-approve:cert-manager-io created
clusterrolebinding.rbac.authorization.k8s.io/cert-manager-controller-certificatesigningrequests created
clusterrolebinding.rbac.authorization.k8s.io/cert-manager-webhook:subjectaccessreviews created
role.rbac.authorization.k8s.io/cert-manager-cainjector:leaderelection created
role.rbac.authorization.k8s.io/cert-manager:leaderelection created
role.rbac.authorization.k8s.io/cert-manager-webhook:dynamic-serving created
rolebinding.rbac.authorization.k8s.io/cert-manager-cainjector:leaderelection created
rolebinding.rbac.authorization.k8s.io/cert-manager:leaderelection created
rolebinding.rbac.authorization.k8s.io/cert-manager-webhook:dynamic-serving created
service/cert-manager created
service/cert-manager-webhook created
deployment.apps/cert-manager-cainjector created
deployment.apps/cert-manager created
deployment.apps/cert-manager-webhook created
mutatingwebhookconfiguration.admissionregistration.k8s.io/cert-manager-webhook created
validatingwebhookconfiguration.admissionregistration.k8s.io/cert-manager-webhook created
```
</p></details>

applying this manifest will create 3 pods under the `cert-manager` namespace:

```
$ kubectl get pods --namespace=cert-manager

NAMESPACE      NAME                                      READY   STATUS    RESTARTS   AGE
cert-manager   cert-manager-848f547974-xzx82             1/1     Running   0          20s
cert-manager   cert-manager-cainjector-54f4cc6b5-w9mtz   1/1     Running   0          21s
cert-manager   cert-manager-webhook-58fb868868-jz8tv     1/1     Running   0          19s
```

## Configure Container Registry

We need to define a container registry to store our builds/images. Use the command bellow and edit accordingly:

```sh
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="<DOCKER_USERNAME>" \
    --docker-password="<DOCKER_PASSWORD>" \
    --docker-server="<DOCKER_SERVER>" \
    --namespace cf
```

Below is an example of using dockerhub

```sh
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="<DOCKER_USERNAME>" \
    --docker-password="<DOCKER_PASSWORD>" \
    --docker-server="https://index.docker.io/v1/" \
    --namespace cf
```

Use the following for GCP's artifact registry, using service account credentials,
with the credential key optionally base64 encoded

```sh
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="_json_key[_base64]" \
    --docker-password="<[BASE64_ENCODED_]SERVICE_ACCOUNT_KEY_JSON>" \
    --docker-server="<ARTIFACT_REGISTRY_LOCATION>-docker.pkg.dev" \
    --namespace cf
```

## kpack

[kpack](https://github.com/pivotal/kpack) _extends Kubernetes and utilizes unprivileged Kubernetes primitives to provide builds of OCI images as a platform implementation of Cloud Native Buildpacks (CNB)._

### Install

```sh
kubectl apply -f https://github.com/pivotal/kpack/releases/download/v0.5.3/release-0.5.3.yaml
```

<details><summary>example output</summary><p>
```
namespace/kpack created
customresourcedefinition.apiextensions.k8s.io/builds.kpack.io created
customresourcedefinition.apiextensions.k8s.io/builders.kpack.io created
customresourcedefinition.apiextensions.k8s.io/clusterbuilders.kpack.io created
customresourcedefinition.apiextensions.k8s.io/clusterstacks.kpack.io created
customresourcedefinition.apiextensions.k8s.io/clusterstores.kpack.io created
configmap/build-init-image created
configmap/build-init-windows-image created
configmap/rebase-image created
configmap/lifecycle-image created
configmap/completion-image created
configmap/completion-windows-image created
deployment.apps/kpack-controller created
serviceaccount/controller created
clusterrole.rbac.authorization.k8s.io/kpack-controller-admin created
clusterrolebinding.rbac.authorization.k8s.io/kpack-controller-admin-binding created
clusterrole.rbac.authorization.k8s.io/kpack-controller-servicebindings-cluster-role created
clusterrolebinding.rbac.authorization.k8s.io/kpack-controller-servicebindings-binding created
role.rbac.authorization.k8s.io/kpack-controller-local-config created
rolebinding.rbac.authorization.k8s.io/kpack-controller-local-config-binding created
customresourcedefinition.apiextensions.k8s.io/images.kpack.io created
service/kpack-webhook created
customresourcedefinition.apiextensions.k8s.io/sourceresolvers.kpack.io created
mutatingwebhookconfiguration.admissionregistration.k8s.io/defaults.webhook.kpack.io created
validatingwebhookconfiguration.admissionregistration.k8s.io/validation.webhook.kpack.io created
secret/webhook-certs created
deployment.apps/kpack-webhook created
serviceaccount/webhook created
role.rbac.authorization.k8s.io/kpack-webhook-certs-admin created
rolebinding.rbac.authorization.k8s.io/kpack-webhook-certs-admin-binding created
clusterrole.rbac.authorization.k8s.io/kpack-webhook-mutatingwebhookconfiguration-admin created
clusterrolebinding.rbac.authorization.k8s.io/kpack-webhook-certs-mutatingwebhookconfiguration-admin-binding created
```
</p></details>

### Configure a [builder](https://buildpacks.io/docs/concepts/components/builder/)

Depending on the docker registry configured earlier, you would like to edit `cf-k8s-controllers/dependencies/kpack/cluster_builder.yaml` and include your registry. Here is our example for dockerhub:

```yml
apiVersion: kpack.io/v1alpha2
kind: ClusterBuilder
metadata:
  name: cf-kpack-cluster-builder
spec:
  serviceAccountRef:
    name: kpack-service-account
    namespace: cf
  # Replace with real docker registry
  #  tag: gcr.io/cf-relint-greengrass/cf-k8s-controllers/kpack/beta
  tag: index.docker.io/1oannis/cf-k8s-controllers/kpack
  stack:
  ...
```

Once everything has been configured, make sure that you are at the root directory of the cloned repository and issue:

```sh
kubectl apply -f dependencies/kpack/service_account.yaml \
    -f dependencies/kpack/cluster_stack.yaml \
    -f dependencies/kpack/cluster_store.yaml \
    -f dependencies/kpack/cluster_builder.yaml
```

## Contour
---
[Contour](https://projectcontour.io/docs/v1.20.1/) _is an Ingress controller for Kubernetes that works by deploying the Envoy proxy as a reverse proxy and load balancer._

### Install
```sh
kubectl apply -f dependencies/contour-1.19.1.yaml
```

<details><summary>example output</summary><p>

```
namespace/projectcontour created
serviceaccount/contour created
serviceaccount/envoy created
configmap/contour created
customresourcedefinition.apiextensions.k8s.io/contourconfigurations.projectcontour.io created
customresourcedefinition.apiextensions.k8s.io/contourdeployments.projectcontour.io created
customresourcedefinition.apiextensions.k8s.io/extensionservices.projectcontour.io created
customresourcedefinition.apiextensions.k8s.io/httpproxies.projectcontour.io created
customresourcedefinition.apiextensions.k8s.io/tlscertificatedelegations.projectcontour.io created
serviceaccount/contour-certgen created
rolebinding.rbac.authorization.k8s.io/contour created
role.rbac.authorization.k8s.io/contour-certgen created
job.batch/contour-certgen-v1.20.1 created
clusterrolebinding.rbac.authorization.k8s.io/contour created
clusterrole.rbac.authorization.k8s.io/contour created
service/contour created
service/envoy created
deployment.apps/contour created
daemonset.apps/envoy created
```

</p></details>

Here are the recently created pods:

```
NAMESPACE        NAME                                      READY   STATUS      RESTARTS   AGE
projectcontour   contour-8696cbb9-5ztz8                    0/1     Running     0          10s
projectcontour   contour-8696cbb9-zjz9k                    0/1     Running     0          10s
projectcontour   contour-certgen-v1.20.1-8nw4b             0/1     Completed   0          15s
projectcontour   envoy-bq6r4                               1/2     Running     0          8s
projectcontour   envoy-cfmmg                               1/2     Running     0          8s
projectcontour   envoy-kvmbf                               1/2     Running     0          8s
projectcontour   envoy-rbb9s                               1/2     Running     0          8s
projectcontour   envoy-xj7k9                               1/2     Running     0          8s
```

### Configuring [**ingress**](https://github.com/cloudfoundry/cf-k8s-controllers#configuring-ingress)

> To enable external access to workloads running on the cluster, you must configure ingress. Generally, a load balancer service will route traffic to the cluster with an external IP and an ingress controller will route the traffic based on domain name or the Host header.

> Provisioning a load balancer service is generally handled automatically by Contour, given the cluster infrastructure provider supports load balancer services. When a load balancer service is reconciled, it is assigned an external IP by the infrastructure provider.

In our case, EKS has created the load balancer and assigned an external url for it. We can grab it using the following:

```
kubectl get service envoy -n projectcontour -o wide
```

<details><summary>example output</summary><p>

```
NAME    TYPE           CLUSTER-IP       EXTERNAL-IP                                                               PORT(S)                      AGE     SELECTOR
envoy   LoadBalancer   10.100.100.236   a4f6620fe317b447fb69a7b3ee07cbd2-2121984464.us-east-1.elb.amazonaws.com   80:30048/TCP,443:31661/TCP   3h23m   app=envoy
```

```
gcloud dns record-sets create "api.install.korifi.cf-app.com." --type=A --rrdatas=34.65.196.103 --zone=korifi --project=cf-on-k8s-wg
gcloud dns record-sets create "*.apps.install.korifi.cf-app.com." --type=A --rrdatas=34.65.196.103 --zone=korifi --project=cf-on-k8s-wg
```

</p></details>

we may then create a cname record to our dns provider and point it to the cf domain we are going to be using.

### Domain [**Management**](https://github.com/cloudfoundry/cf-k8s-controllers#domain-management)

MOVE AFTER CONTROLLER INSTALLATION

> To be able to create workload routes via the CF API in the absence of the domain management endpoints, you must first create the appropriate CFDomain resource(s) for your cluster. Each desired domain name should be specified via the spec.name property of a distinct resource. The metadata.name for the resource can be set to any unique value (the API will use a GUID).

To configure it you will have to edit `controllers/config/samples/cfdomain.yaml` to add your `CFDomain` of choice. Here is the configuration for our example:

```sh
cat <<EOF | kubectl apply --namespace cf -f -
apiVersion: networking.cloudfoundry.org/v1alpha1
kind: CFDomain
metadata:
  name: default-domain
  namespace: cf
spec:
  name: apps.install.cf-app.com
EOF
```

### Default Domain

MOVE AFTER CONTROLLER INSTALLATION

> At the time of installation, platform operators can configure a default domain so that app developers can push an application without specifying domain information.
Operator can do so by setting the `defaultDomainName` at `api/config/base/apiconfig/cf_k8s_api_config.yaml`. The value should match `spec.name` on the `CFDomian` resource.

Here is our version:

```yml
externalFQDN: api.install.cf-app.com
internalPort: 9000

rootNamespace: cf
defaultLifecycleConfig:
  type: buildpack
  stack: cflinuxfs3
  stagingMemoryMB: 1024
  stagingDiskMB: 1024
packageRegistryBase: index.docker.io/1oannis/cf-k8s-controllers/kpack
packageRegistrySecretName: image-registry-credentials # Create this secret in the rootNamespace
clusterBuilderName: cf-kpack-cluster-builder
defaultDomainName: apps.install.cf-app.com
```

## Eirini-Controller

[Eirini Controller](https://github.com/cloudfoundry/eirini-controller#what-is-eirini-controller) _is a Kubernetes controller that aims to enable Cloud Foundry to deploy applications as Pods on a Kubernetes cluster. It brings the CF model to Kubernetes by definig well known Diego abstractions such as Long Running Processes (LRPs) and Tasks as custom Kubernetes resources._

### Install

- Secrets containing certificates for the webhooks need to be created. We have a script that does that for local dev and testing purposes

```sh
./scripts/generate-eirini-certs-secret.sh "*.eirini-controller.svc"
```

<details><summary>example output</summary><p>

```sh
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
100   890  100   890    0     0   1762      0 --:--:-- --:--:-- --:--:--  1762
Will now generate tls.ca tls.crt and tls.key files
~/aws/cf-k8s-controllers/keys ~/aws/cf-k8s-controllers
Error from server (AlreadyExists): namespaces "eirini-controller" already exists
Generating a RSA private key
..............................................................................................................................................................................++++
.................++++
writing new private key to 'tls.key'
-----
Creating the eirini-webhooks-certs secret in your kubernetes cluster
secret/eirini-webhooks-certs created
Done!
```

</p></details>

- In ordrer to install eirini-controller to your k8s cluster, run the command below, replacing x.y.z with a valid release version

```sh
VERSION=0.2.0
WEBHOOK_CA_BUNDLE="$(kubectl get secret -n eirini-controller eirini-webhooks-certs -o jsonpath="{.data['tls\.ca']}")"
helm install eirini-controller https://github.com/cloudfoundry-incubator/eirini-controller/releases/download/v$VERSION/eirini-controller-$VERSION.tgz \
  --set "webhooks.ca_bundle=$WEBHOOK_CA_BUNDLE" \
  --set "workloads.default_namespace=cf" \
  --set "controller.registry_secret_name=image-registry-credentials" \
  --namespace "eirini-controller"
```

<details><summary>example output</summary><p>

```yml
NAME: eirini-controller
LAST DEPLOYED: Fri Apr  1 08:07:12 2022
NAMESPACE: eirini-controller
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

</p></details>

## Hierarchical Namespaces Controller
---
[Hierarchical namespaces](https://github.com/kubernetes-sigs/hierarchical-namespaces#the-hierarchical-namespace-controller-hnc)
>Mmake it easier to share your cluster by making namespaces more powerful. For example, you can create additional namespaces under your team's namespace, even if you don't have cluster-level permission to create namespaces, and easily apply policies like RBAC and Network Policies across all namespaces in your team (e.g. a set of related microservices).
### Install
```sh
kubectl apply -f "https://github.com/kubernetes-sigs/hierarchical-namespaces/releases/download/v1.0.0/default.yaml"
```

<details><summary>example output</summary><p>

```
namespace/hnc-system created
customresourcedefinition.apiextensions.k8s.io/hierarchyconfigurations.hnc.x-k8s.io created
customresourcedefinition.apiextensions.k8s.io/hncconfigurations.hnc.x-k8s.io created
customresourcedefinition.apiextensions.k8s.io/subnamespaceanchors.hnc.x-k8s.io created
role.rbac.authorization.k8s.io/hnc-leader-election-role created
clusterrole.rbac.authorization.k8s.io/hnc-admin-role created
clusterrole.rbac.authorization.k8s.io/hnc-manager-role created
clusterrole.rbac.authorization.k8s.io/hnc-proxy-role created
rolebinding.rbac.authorization.k8s.io/hnc-leader-election-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/hnc-manager-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/hnc-proxy-rolebinding created
secret/hnc-webhook-server-cert created
service/hnc-controller-manager-metrics-service created
service/hnc-webhook-service created
deployment.apps/hnc-controller-manager created
mutatingwebhookconfiguration.admissionregistration.k8s.io/hnc-mutating-webhook-configuration created
validatingwebhookconfiguration.admissionregistration.k8s.io/hnc-validating-webhook-configuration created
```

</p></details>

Now configure hnc plugin to propagate secrets to subnamespaces.
```sh
kubectl patch hncconfigurations.hnc.x-k8s.io config --type=merge -p '{"spec":{"resources":[{"mode":"Propagate", "resource": "secrets"}]}}'
```

## Optional: Install Service Bindings Controller
---
Cloud Native Buildpacks and other app frameworks (such as [Spring Cloud Bindings](https://github.com/spring-cloud/spring-cloud-bindings)) are adopting the [K8s ServiceBinding spec](https://github.com/servicebinding/spec#workload-projection) model of volume mounted secrets.
We currently are providing apps access to these via the `VCAP_SERVICES` environment variable ([see this issue](https://github.com/cloudfoundry/cf-k8s-controllers/issues/462)) for backwards compatibility reasons.
We would also want to support the newer developments in the ServiceBinding ecosystem as well.
### Install
```sh
kubectl apply -f dependencies/service-bindings-0.7.1.yaml
```

<details><summary>example output</summary><p>

```
namespace/service-bindings created
clusterrole.rbac.authorization.k8s.io/service-binding-admin created
clusterrole.rbac.authorization.k8s.io/service-binding-core created
clusterrole.rbac.authorization.k8s.io/service-binding-crd created
clusterrole.rbac.authorization.k8s.io/service-binding-apps created
clusterrole.rbac.authorization.k8s.io/service-binding-knative-serving created
clusterrole.rbac.authorization.k8s.io/service-binding-app-viewer created
serviceaccount/controller created
clusterrolebinding.rbac.authorization.k8s.io/service-binding-controller-admin created
customresourcedefinition.apiextensions.k8s.io/provisionedservices.bindings.labs.vmware.com created
customresourcedefinition.apiextensions.k8s.io/servicebindings.servicebinding.io created
customresourcedefinition.apiextensions.k8s.io/servicebindingprojections.internal.bindings.labs.vmware.com created
mutatingwebhookconfiguration.admissionregistration.k8s.io/defaulting.webhook.bindings.labs.vmware.com created
validatingwebhookconfiguration.admissionregistration.k8s.io/validation.webhook.bindings.labs.vmware.com created
validatingwebhookconfiguration.admissionregistration.k8s.io/config.webhook.bindings.labs.vmware.com created
mutatingwebhookconfiguration.admissionregistration.k8s.io/servicebindingprojections.webhook.bindings.labs.vmware.com created
secret/webhook-certs created
service/webhook created
configmap/config-kapp created
configmap/config-logging created
configmap/config-observability created
deployment.apps/manager created
```

</p></details>

# Installation

Having met the prerequisites above and configured individual files accordingly, we are ready to proceed with the installation steps:

1. Edit the configuration file for cf-k8s-controllers  `controllers/config/base/controllersconfig/cf_k8s_controllers_config.yaml` to
* set the `kpackImageTag` to be the registry location you want for storing the images.

In our example the file looks like this:

```yml
kpackImageTag: index.docker.io/1oannis/cf-k8s-controllers/kpack
clusterBuilderName: cf-kpack-cluster-builder
cfProcessDefaults:
  memoryMB: 1024
  diskQuotaMB: 1024
cfRootNamespace: cf
cfk8s_controller_namespace: cf-k8s-controllers-system
workloads_tls_secret_name: cf-k8s-workloads-ingress-cert
```
2. Edit the configuration file for cf-k8s-api `api/config/base/apiconfig/cf_k8s_api_config.yaml` to
* set the packageRegistryBase field to be the registry location to which you want your source package image uploaded.

We have laready opted in to update the file earlier and the result is the following:

```yml
externalFQDN: "api.cfk8s.cloudruntime.eu"
internalPort: 9000

rootNamespace: cf
defaultLifecycleConfig:
  type: buildpack
  stack: cflinuxfs3
  stagingMemoryMB: 1024
  stagingDiskMB: 1024
packageRegistryBase: index.docker.io/1oannis/cf-k8s-controllers/kpack
packageRegistrySecretName: image-registry-credentials # Create this secret in the rootNamespace
clusterBuilderName: cf-kpack-cluster-builder
defaultDomainName: apps.cfk8s.cloudruntime.eu
```

3. Edit the file `api/config/base/api_url_patch.yaml` to specify the desired URL for the deployed API.

Again, in our example we have the following contents:

```yml
- op: replace
  path: /spec/virtualhost/fqdn
  value: "api.cfk8s.cloudruntime.eu"
```
<details><summary>example output</summary><p>

```sh
go: creating new go.mod: module tmp
Downloading sigs.k8s.io/controller-tools/cmd/controller-gen@v0.8.0
go: downloading sigs.k8s.io/controller-tools v0.8.0
go: downloading github.com/spf13/cobra v1.2.1
go: downloading golang.org/x/tools v0.1.6-0.20210820212750-d4cc65f0b2ff
go: downloading sigs.k8s.io/yaml v1.3.0
go: downloading github.com/fatih/color v1.12.0
go: downloading k8s.io/api v0.23.0
go: downloading k8s.io/apimachinery v0.23.0
go: downloading gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
go: downloading k8s.io/apiextensions-apiserver v0.23.0
go: downloading github.com/gobuffalo/flect v0.2.3
go: downloading github.com/mattn/go-colorable v0.1.8
go: downloading github.com/mattn/go-isatty v0.0.12
go: downloading gopkg.in/yaml.v2 v2.4.0
go: downloading github.com/spf13/pflag v1.0.5
go: downloading github.com/gogo/protobuf v1.3.2
go: downloading k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b
go: downloading github.com/google/gofuzz v1.1.0
go: downloading k8s.io/klog/v2 v2.30.0
go: downloading sigs.k8s.io/structured-merge-diff/v4 v4.1.2
go: downloading sigs.k8s.io/json v0.0.0-20211020170558-c049b76a60c6
go: downloading golang.org/x/sys v0.0.0-20210831042530-f4d43177bf5e
go: downloading gopkg.in/inf.v0 v0.9.1
go: downloading github.com/google/go-cmp v0.5.6
go: downloading golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
go: downloading github.com/go-logr/logr v1.2.0
go: downloading golang.org/x/net v0.0.0-20210825183410-e898025ed96a
go: downloading golang.org/x/mod v0.4.2
go: downloading github.com/json-iterator/go v1.1.12
go: downloading github.com/modern-go/reflect2 v1.0.2
go: downloading github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
go: downloading golang.org/x/text v0.3.7
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/controller-gen "crd" rbac:roleName=manager-role webhook paths="./controllers/..." output:crd:artifacts:config=controllers/config/crd/bases output:rbac:artifacts:config=controllers/config/rbac output:webhook:artifacts:config=controllers/config/webhook
go: creating new go.mod: module tmp
Downloading sigs.k8s.io/kustomize/kustomize/v4@v4.5.2
go: downloading sigs.k8s.io/kustomize/kustomize/v4 v4.5.2
go: downloading sigs.k8s.io/kustomize/api v0.11.2
go: downloading sigs.k8s.io/kustomize/cmd/config v0.10.4
go: downloading sigs.k8s.io/kustomize/kyaml v0.13.3
go: downloading sigs.k8s.io/yaml v1.2.0
go: downloading github.com/evanphx/json-patch v4.11.0+incompatible
go: downloading github.com/imdario/mergo v0.3.5
go: downloading github.com/go-errors/errors v1.0.1
go: downloading k8s.io/kube-openapi v0.0.0-20210421082810-95288971da7e
go: downloading github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00
go: downloading github.com/xlab/treeprint v0.0.0-20181112141820-a009c3971eca
go: downloading github.com/stretchr/testify v1.7.0
go: downloading github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
go: downloading github.com/olekukonko/tablewriter v0.0.4
go: downloading go.starlark.net v0.0.0-20200306205701-8dd3e2ee1dd5
go: downloading github.com/pmezard/go-difflib v1.0.0
go: downloading github.com/mattn/go-runewidth v0.0.7
go: downloading github.com/mitchellh/mapstructure v1.4.1
go: downloading github.com/go-openapi/swag v0.19.5
go: downloading github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a
go: downloading github.com/go-openapi/jsonreference v0.19.3
go: downloading github.com/mailru/easyjson v0.7.0
go: downloading github.com/go-openapi/jsonpointer v0.19.3
go: downloading github.com/PuerkitoBio/purell v1.1.1
go: downloading golang.org/x/text v0.3.5
go: downloading golang.org/x/net v0.0.0-20210405180319-a5a99cb37ef4
go: downloading github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize build controllers/config/crd | kubectl apply -f -
customresourcedefinition.apiextensions.k8s.io/cfapps.workloads.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfbuilds.workloads.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfdomains.networking.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfpackages.workloads.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfprocesses.workloads.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfroutes.networking.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfservicebindings.services.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfserviceinstances.services.cloudfoundry.org created
cd controllers/config/manager && /home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize edit set image cloudfoundry/cf-k8s-controllers=cloudfoundry/cf-k8s-controllers:latest
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize build controllers/config/default -o controllers/reference/cf-k8s-controllers.yaml
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize build controllers/config/default | kubectl apply -f -
namespace/cf-k8s-controllers-system created
customresourcedefinition.apiextensions.k8s.io/cfapps.workloads.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfbuilds.workloads.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfdomains.networking.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfpackages.workloads.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfprocesses.workloads.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfroutes.networking.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfservicebindings.services.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfserviceinstances.services.cloudfoundry.org configured
serviceaccount/cf-k8s-controllers-controller-manager created
role.rbac.authorization.k8s.io/cf-k8s-controllers-leader-election-role created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-admin created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-cfservicebinding-reconciler-role created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-manager-role created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-metrics-reader created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-organization-manager created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-organization-user created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-proxy-role created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-root-namespace-user created
clusterrolebinding.rbac.authorization.k8s.io/cf-k8s-controllers-manager-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/cf-k8s-controllers-proxy-rolebinding created
configmap/cf-k8s-controllers-config created
configmap/cf-k8s-controllers-manager-config created
service/cf-k8s-controllers-controller-manager-metrics-service created
service/cf-k8s-controllers-webhook-service created
deployment.apps/cf-k8s-controllers-controller-manager created
certificate.cert-manager.io/cf-k8s-controllers-serving-cert created
issuer.cert-manager.io/cf-k8s-controllers-selfsigned-issuer created
tlscertificatedelegation.projectcontour.io/cf-k8s-controllers-workloads-fallback-delegation created
mutatingwebhookconfiguration.admissionregistration.k8s.io/cf-k8s-controllers-mutating-webhook-configuration created
validatingwebhookconfiguration.admissionregistration.k8s.io/cf-k8s-controllers-validating-webhook-configuration created
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/controller-gen "crd" rbac:roleName=cf-admin-clusterrole paths=./api/... output:rbac:artifacts:config=api/config/base/rbac
cd api/config/base && /home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize edit set image cloudfoundry/cf-k8s-api=cloudfoundry/cf-k8s-api:latest
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize build api/config/base -o api/reference/cf-k8s-api.yaml
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize build api/config/base | kubectl apply -f -
namespace/cf-k8s-api-system created
serviceaccount/cf-k8s-api-cf-admin-serviceaccount created
clusterrole.rbac.authorization.k8s.io/cf-k8s-api-cf-admin-clusterrole created
clusterrolebinding.rbac.authorization.k8s.io/cf-k8s-api-cf-admin-clusterrolebinding created
configmap/cf-k8s-api-config-dht8hb62cg created
service/cf-k8s-api-svc created
deployment.apps/cf-k8s-api-deployment created
httpproxy.projectcontour.io/cf-k8s-api-proxy created
```

</p></details>

### Deploy

`make deploy`


<details><summary>example output</summary><p>

```
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/controller-gen "crd" rbac:roleName=manager-role webhook paths="./controllers/..." output:crd:artifacts:config=controllers/config/crd/bases output:rbac:artifacts:config=controllers/config/rbac output:webhook:artifacts:config=controllers/config/webhook
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize build controllers/config/crd | kubectl apply -f -
customresourcedefinition.apiextensions.k8s.io/cfapps.workloads.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfbuilds.workloads.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfdomains.networking.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfpackages.workloads.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfprocesses.workloads.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfroutes.networking.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfservicebindings.services.cloudfoundry.org created
customresourcedefinition.apiextensions.k8s.io/cfserviceinstances.services.cloudfoundry.org created
cd controllers/config/manager && /home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize edit set image cloudfoundry/cf-k8s-controllers=cloudfoundry/cf-k8s-controllers:latest
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize build controllers/config/default -o controllers/reference/cf-k8s-controllers.yaml
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize build controllers/config/default | kubectl apply -f -
namespace/cf-k8s-controllers-system created
customresourcedefinition.apiextensions.k8s.io/cfapps.workloads.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfbuilds.workloads.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfdomains.networking.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfpackages.workloads.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfprocesses.workloads.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfroutes.networking.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfservicebindings.services.cloudfoundry.org configured
customresourcedefinition.apiextensions.k8s.io/cfserviceinstances.services.cloudfoundry.org configured
serviceaccount/cf-k8s-controllers-controller-manager created
role.rbac.authorization.k8s.io/cf-k8s-controllers-leader-election-role created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-admin created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-cfservicebinding-reconciler-role created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-manager-role created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-metrics-reader created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-organization-manager created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-organization-user created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-proxy-role created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-root-namespace-user created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-space-auditor created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-space-developer created
clusterrole.rbac.authorization.k8s.io/cf-k8s-controllers-space-manager created
rolebinding.rbac.authorization.k8s.io/cf-k8s-controllers-leader-election-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/cf-k8s-controllers-manager-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/cf-k8s-controllers-proxy-rolebinding created
configmap/cf-k8s-controllers-config created
configmap/cf-k8s-controllers-manager-config created
service/cf-k8s-controllers-controller-manager-metrics-service created
service/cf-k8s-controllers-webhook-service created
deployment.apps/cf-k8s-controllers-controller-manager created
certificate.cert-manager.io/cf-k8s-controllers-serving-cert created
issuer.cert-manager.io/cf-k8s-controllers-selfsigned-issuer created
tlscertificatedelegation.projectcontour.io/cf-k8s-controllers-workloads-fallback-delegation created
mutatingwebhookconfiguration.admissionregistration.k8s.io/cf-k8s-controllers-mutating-webhook-configuration created
validatingwebhookconfiguration.admissionregistration.k8s.io/cf-k8s-controllers-validating-webhook-configuration created
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/controller-gen "crd" rbac:roleName=cf-admin-clusterrole paths=./api/... output:rbac:artifacts:config=api/config/base/rbac
cd api/config/base && /home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize edit set image cloudfoundry/cf-k8s-api=cloudfoundry/cf-k8s-api:latest
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize build api/config/base -o api/reference/cf-k8s-api.yaml
/home/ubuntu/aws/cf-k8s-controllers/controllers/bin/kustomize build api/config/base | kubectl apply -f -
namespace/cf-k8s-api-system created
serviceaccount/cf-k8s-api-cf-admin-serviceaccount created
clusterrole.rbac.authorization.k8s.io/cf-k8s-api-cf-admin-clusterrole created
clusterrolebinding.rbac.authorization.k8s.io/cf-k8s-api-cf-admin-clusterrolebinding created
configmap/cf-k8s-api-config-dht8hb62cg created
service/cf-k8s-api-svc created
deployment.apps/cf-k8s-api-deployment created
httpproxy.projectcontour.io/cf-k8s-api-proxy created
```

</p></details>

The following two pods are added to our cluster:

```sh
NAMESPACE                   NAME                                                     READY   STATUS      RESTARTS   AGE
cf-k8s-api-system           cf-k8s-api-deployment-5dfd484d57-jlvfx                   1/1     Running     0          55s
cf-k8s-controllers-system   cf-k8s-controllers-controller-manager-56b9b84884-fz86z   2/2     Running     0          77s
```


# Post Deployment

## Configure Image Registry Credentials Secret

```sh
kubectl create secret docker-registry image-registry-secret \
    --docker-username="1oannis" \
    --docker-password="s0m3p@$$w0rd" \
    --docker-server="https://index.docker.io/v1/" --namespace cf-k8s-api-system
```

<details><summary>example output</summary><p>

```
secret/image-registry-secret created
```

</p></details>








<details><summary>example output</summary><p>

</p></details>
