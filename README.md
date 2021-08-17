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


