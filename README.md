## Installation
### Clone this repo

```
cd ~/workspace
git clone git@github.com:cloudfoundry/cf-k8s-controllers.git
cd cf-k8s-controllers/
```

### Prerequisites

### Install Cert-Manager
To deploy cf-k8s-controller and run it in a cluster, you must first [install cert-manager](https://cert-manager.io/docs/installation/) 
```
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.5.3/cert-manager.yaml
```
Or
```
kubectl apply -f dependencies/cert-manager.yaml
```

### Install kpack

Edit the file: `config/kpack/cluster_builder.yaml` and set the `tag` field to be the registry location you want your ClusterBuilder image to be uploaded to.

Run the commands below substituting the values for the Docker credentials to the registry where images will be uploaded to.
```
kubectl create secret docker-registry image-registry-credentials \
    --docker-username="<DOCKER_USERNAME>" \
    --docker-password="<DOCKER_PASSWORD>" \
     --docker-server="<DOCKER_SERVER>" --namespace default

kubectl apply -f dependencies/kpack/release-0.3.1.yaml
kubectl apply -f config/kpack/service_account.yaml \
    -f config/kpack/cluster_stack.yaml \
    -f config/kpack/cluster_store.yaml \
    -f config/kpack/cluster_builder.yaml
```

### Install Contour and Envoy

To deploy cf-k8s-controller and run it in a cluster, you must first [install contour](https://projectcontour.io/getting-started/) 
```
kubectl apply -f https://projectcontour.io/quickstart/contour.yaml
```
Or
```
kubectl apply -f dependencies/contour-1.18.1.yaml

```

---
## Development workflow

### Install dependencies with hack script
```
# modify kpack dependency files to point towards your registry
hack/install-dependencies.sh -g "<PATH_TO_GCR_CREDENTIALS>"
```

### Configure cf-k8s-controllers
Configuration file for cf-k8s-controllers is at `config/cf/cf_k8s_controllers_k8s.yaml`
Note: Edit this file and set the `kpackImageTag` field to be the registry location you want for storing the images. 

### Build, Install and Deploy to K8s cluster
Set the $IMG environment variable to a location you have push/pull access. For example 
```
export IMG=foo/cf-k8s-controllers:bar #Replace this with your image ref
make generate docker-build docker-push install deploy
```
*This will generate the CRD bases, build and push an image with the repository and tag specified by the environment variable, install CRDs and deploy the controller manager. This should cover an entire development loop. Further fine grain instructions are below.*

---
## Using the Makefile
### Set image respository and tag for controller manager
```
export IMG=foo/cf-k8s-controllers:bar #Replace this with your image ref
```
### Generate CRD bases
```
make generate
```
### Build Docker image
```
make docker-build
```
### Push Docker image
```
make docker-push
```
### Install CRDs to K8s in current Kubernetes context
```
make install
```
### Deploy controller manager to K8s in current Kubernetes context
```
make deploy
```

### Generate reference yaml
Build reference yaml (with defaults) to be applied with kubectl
```
make build-reference
```
---
## Using kubectl
Apply CRDs and controller-manager with reference defaults
```
kubectl apply -f reference/cf-k8s-controllers.yaml
```
---
## Sample Resources
### Apply sample instances of the resources
```
kubectl apply -f config/samples/. --recursive
```

Running the unit tests
```
go test ./...
```

Running integration tests
```
# First time (pulls kube-apiserver and etcd for envtest):
make test

# Afterwards
export KUBEBUILDER_ASSETS=~/workspace/cf-k8s-controllers/testbin/bin/
go test ./... -tags=integration
```

