![Build Status](https://github.com/cloudfoundry/korifi/actions/workflows/test-build-push-main.yml/badge.svg)
[![Maintainability](https://api.codeclimate.com/v1/badges/1112ab5cfa6a0654cfd2/maintainability)](https://codeclimate.com/github/cloudfoundry/korifi/maintainability)
[![Test Coverage](https://api.codeclimate.com/v1/badges/1112ab5cfa6a0654cfd2/test_coverage)](https://codeclimate.com/github/cloudfoundry/korifi/test_coverage)

# Introduction
This repository contains an experimental implementation of the [V3 Cloud Foundry API](http://v3-apidocs.cloudfoundry.org) that is backed entirely by Kubernetes [custom resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/).

We maintain our own set of [API Endpoint docs](docs/api.md) for tracking what is currently supported by the API Shim.

For more information about what we're building, check out the [Vision for CF on Kubernetes](https://docs.google.com/document/d/1rG814raI5UfGUsF_Ycrr8hKQMo1RH9TRMxuvkgHSdLg/edit) document.

If you are interested in learning about [Differences between Korifi and CF-for-Vms](https://github.com/cloudfoundry/korifi/blob/main/docs/known-differences-with-cf-for-vms.md)

# Prerequisites
Before installing, ensure that you have the following:
- [the kubernetes cli](https://kubernetes.io/docs/tasks/tools/#kubectl)
- a compatible kubernetes cluster (e.g. kind, GKE)
- a container registry that you have write access for (e.g. GCR, Harbor)
- [the CloudFoundry cli](https://docs.cloudfoundry.org/cf-cli/install-go-cli.html) version 8.1 or later
- [Helm](https://helm.sh/docs/helm/helm_install/)
- A cloned copy of this repository:
  ```sh
  cd ~/workspace
  git clone git@github.com:cloudfoundry/korifi.git
  cd korifi
  ```

# Dependencies
## Install Cert-Manager
To deploy Korifi and run it in a cluster, you must first [install cert-manager](https://cert-manager.io/docs/installation/).
```sh
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.5.3/cert-manager.yaml
```
Or
```sh
kubectl apply -f dependencies/cert-manager.yaml
```
---
## Install Kpack
To deploy Korifi and run it in a cluster, you must first [install kpack](https://github.com/pivotal/kpack/blob/main/docs/install.md).
```sh
kubectl apply -f https://github.com/pivotal/kpack/releases/download/v0.5.2/release-0.5.2.yaml
```
Or
```sh
kubectl apply -f dependencies/kpack-release-0.5.2.yaml
```

### Configure an Image Registry Credentials Secret
Edit the file: `config/kpack/cluster_builder.yaml` and set the `tag` field to be the registry location you want your ClusterBuilder image to be uploaded to.

Run the command below, substituting the values for the Docker credentials to the registry where images will be uploaded to.
```sh
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="<DOCKER_USERNAME>" \
    --docker-password="<DOCKER_PASSWORD>" \
    --docker-server="<DOCKER_SERVER>" --namespace default
```

### Configure a Default Builder
```sh
kubectl apply -f dependencies/kpack/service_account.yaml \
    -f dependencies/kpack/cluster_stack.yaml \
    -f dependencies/kpack/cluster_store.yaml \
    -f dependencies/kpack/cluster_builder.yaml
```
> note: Edit `cluster_builder.yaml` to specify an image tag that you have write access to using the credentials above.
---
## Install Contour and Envoy
To deploy Korifi and run it in a cluster, you must first [install contour](https://projectcontour.io/getting-started/).
```sh
kubectl apply -f https://projectcontour.io/quickstart/contour.yaml
```
Or
```sh
kubectl apply -f dependencies/contour-1.19.1.yaml
```

### Configuring Ingress
To enable external access to workloads running on the cluster, you must configure ingress. Generally, a load balancer service will route traffic to the cluster with an external IP and an ingress controller will route the traffic based on domain name or the Host header.

Provisioning a load balancer service is generally handled automatically by Contour, given the cluster infrastructure provider supports load balancer services. When a load balancer service is reconciled, it is assigned an external IP by the infrastructure provider. The external IP is visible on the Service resource itself via `kubectl get service envoy -n projectcontour -o wide`. You can use that external IP to define a DNS A record to route traffic based on your desired domain name.

With the load balancer provisioned, you must configure the ingress controller to route traffic based on your desired domain/host name. For Contour, this configuration goes on an HTTPProxy, for which we have a default resource defined to route traffic to the CF API.

### Domain Management
To be able to create workload routes via the CF API in the absence of the domain management endpoints, you must first create the appropriate `CFDomain` resource(s) for your cluster. Each desired domain name should be specified via the `spec.name` property of a distinct resource. The `metadata.name` for the resource can be set to any unique value (the API will use a GUID). See `controllers/config/samples/cfdomain.yaml` for an example.

### Configuring Default Domain

At the time of installation, platform operators can configure a default domain so that app developers can push an application without specifying domain information. 
Operator can do so by setting the `defaultDomainName` at `api/config/base/apiconfig/korifi_api_config.yaml`. The value should match `spec.name` on the `CFDomain` resource.

Note: Platform operators are responsible for creating the required `CFDomain` resource. See `controllers/config/samples/cfdomain.yaml` for an example.

---
## Install Eirini-Controller

### From release url
Follow the installation instructions for [eirini-controllers](https://github.com/cloudfoundry-incubator/eirini-controller#installation)

### From local repository
Clone the [eirini-controller](https://github.com/cloudfoundry-incubator/eirini-controller) repo and go to its root directory, render the Helm chart, and apply it. In this case we use the image built based on the latest commit of main at the time of authoring.
```sh
# Set the certificate authority value for the eirini installation
export webhooks_ca_bundle="$(kubectl get secret -n eirini-controller eirini-webhooks-certs -o jsonpath="{.data['tls\.ca']}")"
# Run from the eirini-controller repository root
helm template eirini-controller "deployment/helm" \
  --values "deployment/helm/values-template.yaml" \
  --set "webhooks.ca_bundle=${webhooks_ca_bundle}" \
  --set "workloads.create_namespaces=true" \
  --set "workloads.default_namespace=cf" \
  --set "controller.registry_secret_name=image-registry-credentials" \
  --set "images.eirini_controller=eirini/eirini-controller@sha256:42e22b3222e9b3788782f5c141d260a5e163da4f4032e2926752ef2e5bae0685" \
  --namespace "eirini-controller" | kubectl apply -f -
```

### Configure a Controller Certificate
Eirini-controller requires a certificate for the controller and webhook. If you are using openssl, or libressl v3.1.0 or later:
```sh
openssl req -x509 -newkey rsa:4096 \
  -keyout tls.key -out tls.crt \
  -nodes -subj '/CN=localhost' \
  -addext "subjectAltName = DNS:*.eirini-controller.svc, DNS:*.eirini-controller.svc.cluster.local" \
  -days 365
```

If you are using an older version of libressl (the default on OSX):
```sh
openssl req -x509 -newkey rsa:4096 \
  -keyout tls.key -out tls.crt \
  -nodes -subj '/CN=localhost' \
  -extensions SAN -config <(cat /etc/ssl/openssl.cnf <(printf "[ SAN ]\nsubjectAltName='DNS:*.eirini-controller.svc, DNS:*.eirini-controller.svc.cluster.local'")) \
  -days 365
```

Once you have created a certificate, use it to create the secret required by eirini-controller:
```
kubectl create secret -n eirini-controller generic eirini-webhooks-certs --from-file=tls.crt=./tls.crt --from-file=tls.ca=./tls.crt --from-file=tls.key=./tls.key
```

---
## Install Hierarchical Namespaces Controller
```sh
kubectl apply -f "https://github.com/kubernetes-sigs/hierarchical-namespaces/releases/download/v0.9.0/hnc-manager.yaml"
```
After running the above command, it will take up to 30s for HNC to refresh the
certificates on its webhooks. In the meantime, install the kubectl HNC plugin to some location in your `PATH`:

```sh
HNC_VERSION=v0.9.0
HNC_PLATFORM=darwin_amd64 # also supported: linux_amd64, windows_amd64
curl -L https://github.com/kubernetes-sigs/hierarchical-namespaces/releases/download/${HNC_VERSION}/kubectl-hns_${HNC_PLATFORM} -o /usr/local/bin/kubectl-hns
chmod +x /usr/local/bin/kubectl-hns
```
Ensure the plugin is working:

```sh
kubectl hns
```
The help text should be displayed.

Now use the hnc plugin to propagate the kpack image registry secret to
subnamespaces.

```sh
kubectl hns config set-resource secrets --mode Propagate
```

---
## Optional: Install Service Bindings Controller

Cloud Native Buildpacks and other app frameworks (such as [Spring Cloud Bindings](https://github.com/spring-cloud/spring-cloud-bindings)) are adopting the [K8s ServiceBinding spec](https://github.com/servicebinding/spec#workload-projection) model of volume mounted secrets. 
We currently are providing apps access to these via the `VCAP_SERVICES` environment variable ([see this issue](https://github.com/cloudfoundry/korifi/issues/462)) for backwards compatibility reasons.
We would also want to support the newer developments in the ServiceBinding ecosystem as well.

We are not implementing this ourselves but allowing controller that adopts the [ServiceBinding Spec](https://github.com/servicebinding/spec) that volume mounts secrets to workload containers to be used. 
We have tested with [this Service Binding reconciler](https://github.com/vmware-labs/service-bindings). Install it using 

```sh
kubectl apply -f https://github.com/vmware-tanzu/servicebinding/releases/download/v0.7.1/service-bindings-0.7.1.yaml
```

---
# Installation

## Create the root namespace
Create your root namespace. The default name is `cf`, but you can override this in the configuration files for korifi-controllers and korifi-api (see below).

Example: `kubectl create namespace cf`

## Configure korifi-api
The configuration file for korifi-api is located at `api/config/base/apiconfig/korifi_api_config.yaml`.

### Configure the API
- Set the `packageRegistryBase` field to be the registry location to which you want your source package image uploaded. (required)
- Edit the file `api/config/base/api_url_patch.yaml` to specify the desired URL for the deployed API. (required)

### Configure Kpack
- Set the `kpackImageTag` to be the registry location you want for storing the images. (required)
- Set the `clusterBuilderName`, if you want Kpack to use a different cluster builder. (optional)

### Configure an Auth Proxy (optional) 
This section is only required if you are using an authentication proxy with your cluster to enable SSO.
- Set the `authProxyHost` field to the IP address of your cluster's auth proxy.
- Set the `authProxyCACert` field to the CA certificate of your cluster's auth proxy.

## Configure korifi-controllers
The configuration file for korifi-controllers is located at `controllers/config/base/controllersconfig/korifi_controllers_config.yaml`.

## Install korifi-api and korifi-controllers
From the `korifi` directory use the Makefile to deploy the controllers and API shim:
```
make deploy
```

## Post Deployment

### Create a role binding for your cluster admin user
To grant your kubernetes user admin-level access to the Cloud Foundry API,
they need the `korifi-controllers-admin` role binding in your root namespace (i.e `cf`).

Example: `kubectl create rolebinding default-admin-binding -n cf --role korifi-controllers-admin --user <YOUR USER>`

### Configure Image Registry Credentials Secret
Run the command below, substituting the values for the Docker credentials to the registry where source package images will be uploaded to.

```
kubectl create secret docker-registry image-registry-secret \
  --docker-username="<DOCKER_USERNAME>" \
  --docker-password="<DOCKER_PASSWORD>" \
  --docker-server="<DOCKER_SERVER>" --namespace korifi-api-system
```

### Configure API Ingress TLS Certificate Secret
Generate a self-signed certificate. If you are using openssl, or libressl v3.1.0 or later:
```
openssl req -x509 -newkey rsa:4096 \
  -keyout tls.key -out tls.crt \
  -nodes -subj '/CN=localhost' \
  -addext "subjectAltName = DNS:localhost" \
  -days 365
```

If you are using an older version of libressl (the default on OSX):
```
openssl req -x509 -newkey rsa:4096 \
  -keyout tls.key -out tls.crt \
  -nodes -subj '/CN=localhost' \
  -extensions SAN -config <(cat /etc/ssl/openssl.cnf <(printf "[ SAN ]\nsubjectAltName='DNS:localhost'")) \
  -days 365
```

Create a TLS secret called `korifi-api-ingress-cert` using the self-signed
certificate generated above, or from your own existing certificate:
```
kubectl create secret tls \
  korifi-api-ingress-cert \
  --cert=./tls.crt --key=./tls.key \
  -n korifi-api-system
```

**NOTE**: If you choose to generate a self-signed certificate, you will need to
skip TLS validation when connecting to the API.

### Configure Workload Ingress TLS Certificate Secret
Generate a self-signed certificate. If you are using openssl, or libressl v3.1.0 or later:
```
openssl req -x509 -newkey rsa:4096 \
  -keyout tls.key -out tls.crt \
  -nodes -subj '/CN=localhost' \
  -addext "subjectAltName = DNS:localhost" \
  -days 365
```

If you are using an older version of libressl (the default on OSX):
```
openssl req -x509 -newkey rsa:4096 \
  -keyout tls.key -out tls.crt \
  -nodes -subj '/CN=localhost' \
  -extensions SAN -config <(cat /etc/ssl/openssl.cnf <(printf "[ SAN ]\nsubjectAltName='DNS:localhost'")) \
  -days 365
```

Create a TLS secret called `korifi-workloads-ingress-cert` using the self-signed
certificate generated above, or from your own existing certificate:
```
kubectl create secret tls \
  korifi-workloads-ingress-cert \
  --cert=./tls.crt --key=./tls.key \
  -n korifi-controllers-system
```

**NOTE**: If you choose to generate a self-signed certificate, you will need to
skip TLS validation when connecting to the workload.

# Development Workflows

## Prerequisites

In order to build & install images yourself, consider installing:
- [Docker](https://docs.docker.com/get-docker/)
- [kind](https://kubernetes.io/docs/tasks/tools/#kind)
- [kubebuilder](https://book-v1.book.kubebuilder.io/getting_started/installation_and_setup.html)
- [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/)

---
## Running the tests
```
make test
```
or
```
make test-controllers
```
or
```
make test-api
```
> Note: Some API tests run a real Kubernetes API/etcd via the [`envtest`](https://book.kubebuilder.io/reference/envtest.html) package. These tests rely on the CRDs from `controllers` subdirectory.
> To update these CRDs use the `make generate-controllers` target described below.

---
## Deploying to `kind` with a locally deployed container registry
This is the easiest method for deploying a kick-the-tires installation, or testing code changes end-to-end.

```
./scripts/deploy-on-kind <kind-cluster-name> --local
```
or
```
./scripts/deploy-on-kind --help
```
for usage notes.

### User Permissions Disclaimer
When using the deploy-on-kind script, you will get a separate `cf-admin` user by default with which to interact with the cf api.

So when prompted to select a user by the cli you may see something like:
```
$ cf login
API endpoint: http://localhost
Warning: Insecure http API endpoint detected: secure https API endpoints are recommended

1. cf-admin
2. kind-test
```

Of these options, `cf-admin` is the user with permissions set up by default. Selecting the other use may allow you to login and
successfully create resources, but you may notice that the user lacks the permissions to list those resources once created.

---
## Deploying to `kind` for remote debugging with a locally deployed container registry
This is the above method, but run with `dlv` for remote debugging.

```
./scripts/deploy-on-kind <kind-cluster-name> --local --debug
```
To remote debug, connect with `dlv` on `localhost:30051` (controller) or `localhost:30052` (api)

A sample VSCode `launch.json` configuration is provided below:
```
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Attach to Debug Controller on Kind",
            "type": "go",
            "debugAdapter": "dlv-dap",
            "request": "attach",
            "mode": "remote",
            "substitutePath": [
                {
                    "from": "${workspaceFolder}",
                    "to": "/workspace"
                }
            ],
            "host": "localhost",
            "port": 30051
        },
        {
            "name": "Attach to Debug API on Kind",
            "type": "go",
            "debugAdapter": "dlv-dap",
            "request": "attach",
            "mode": "remote",
            "substitutePath": [
                {
                    "from": "${workspaceFolder}",
                    "to": "/workspace"
                }
            ],
            "host": "localhost",
            "port": 30052
        }
    ]
}

```


---
## Install all dependencies with script
```sh
# modify kpack dependency files to point towards your registry
scripts/install-dependencies.sh -g "<PATH_TO_GCR_CREDENTIALS>"
```

---
## Build, Install and Deploy to a K8s cluster
Set the $IMG_CONTROLLERS and $IMG_API environment variables to locations where you have push/pull access. For example:
```sh
export IMG_CONTROLLERS=foo/korifi-controllers:bar #Replace this with your image ref
export IMG_API=foo/korifi-api:bar #Replace this with your image ref
make generate-controllers docker-build docker-push deploy
```
*This will generate the CRD bases, build and push images with the repository and tags specified by the environment variables, install CRDs and deploy the controller manager and API Shim.*

---
## Sample Resources
The `samples` directory contains examples of the custom resources that are created by the API shim when a user interacts with the API. They are useful for testing controllers in isolation.
### Apply sample instances of the resources
```
kubectl apply -f controllers/config/samples/. --recursive
```

---
## Using the Makefile

### Running controllers locally
```sh
make run-controllers
```

### Running the API Shim Locally

```make
make run-api
```

To specify a custom configuration file, set the `APICONFIG` environment variable to its path when running the web server. Refer to the [default config](api/config/base/apiconfig/korifi_api_config.yaml) for the config file structure and options.

### Set image respository and tag for controller manager
```sh
export IMG_CONTROLLERS=foo/korifi-controllers:bar #Replace this with your image ref
```
### Set image respository and tag for API
```sh
export IMG_API=foo/korifi-api:bar #Replace this with your image ref
```
### Generate CRD bases
```sh
make generate-controllers
```
### Build Docker image
```
make docker-build
```
or
```
make docker-build-controllers
```
or
```
make docker-build-api
```

### Push Docker image
```
make docker-push
```
or
```
make docker-push-controllers
```
or
```
make docker-push-api
```

### Install CRDs to K8s in current Kubernetes context
```
make install-crds
```
### Deploy controller manager/API to K8s in current Kubernetes context
```
make deploy
```
or
```
make deploy-controllers
```
or
```
make deploy-api
```


### Generate reference yaml
Build reference yaml (with defaults) to be applied with kubectl
```
make build-reference
```
or
```
make build-reference-controllers
```
or
```
make build-reference-api
```

### Regenerate kubernetes resources after making changes
To regenerate the generated kubernetes resources under `./api/config` and `./controllers/config`:

```
make manifests
```
or
```
make manifests-controllers
```
or
```
make manifests-api
```
# Contributing
This project follows [Cloud Foundry Code of Conduct](https://www.cloudfoundry.org/code-of-conduct/)
