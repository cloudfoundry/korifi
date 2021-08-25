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
$ kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.5.3/cert-manager.yaml
```

### Build, Install and Deploy to K8s cluster
Set the $IMG environment variable to a location you have push/pull access. For example 
```
export IMG=relintdockerhubpushbot/cf-k8s-controllers:add-webhooks #Replace this with your image ref
make generate docker-build docker-push install deploy
```

### Deploy CRDs to K8s in current Kubernetes context
```
make install
```

### Apply sample instances of the resources
```
kubectl apply -f config/samples/. --recursive
```



