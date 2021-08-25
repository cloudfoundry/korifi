### Installation
Clone this repo:
```
cd ~/workspace
git clone git@github.com:cloudfoundry/cf-k8s-controllers.git
cd cf-k8s-controllers/
```

Deploy CRDs to K8s in current Kubernetes context
```
make install
```

Apply sample instances of the resources
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