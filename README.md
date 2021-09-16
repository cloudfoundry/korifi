## Installation
### Clone this repo

```
cd ~/workspace
git clone git@github.com:cloudfoundry/cf-k8s-controllers.git
cd cf-k8s-controllers/
```

### Prerequisites
To deploy cf-k8s-controller and run it in a cluster, you must first [install cert-manager](https://cert-manager.io/docs/installation/) 
```
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.5.3/cert-manager.yaml
```
Or
```
kubectl apply -f dependencies/cert-manager.yaml
```
---
## Development workflow
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

