> **Warning**
> Make sure you are using the correct version of these instructions by using the link in the release notes for the version you're trying to install. If you're not sure, check our [latest release](https://github.com/cloudfoundry/korifi/releases/latest).

# Install Korifi on kind

In order to install korifi on kind effortlessly we have prepared an installation job definition that you simply apply to your kind cluster. It will install korifi with reasonable defautls using a local docker registry (also running on your kind cluster).

> **Warning**
> The installer will deploy korifi with experimental features. To find out more please check out the `experimental` section of korifi's helm [values](./helm/korifi/values.yaml) file.

## Cluster creation

In order to access the Korifi API, we'll need to [expose the cluster ingress locally](https://kind.sigs.k8s.io/docs/user/ingress/). To do it, create your kind cluster using a command like this:

```sh
cat <<EOF | kind create cluster --name korifi --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localregistry-docker-registry.default.svc.cluster.local:30050"]
        endpoint = ["http://127.0.0.1:30050"]
    [plugins."io.containerd.grpc.v1.cri".registry.configs]
      [plugins."io.containerd.grpc.v1.cri".registry.configs."127.0.0.1:30050".tls]
        insecure_skip_verify = true
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 32080
    hostPort: 80
    protocol: TCP
  - containerPort: 32443
    hostPort: 443
    protocol: TCP
  - containerPort: 30050
    hostPort: 30050
    protocol: TCP
EOF
```

## Install Korifi

- Run the installer job:

```sh
kubectl apply -f https://github.com/cloudfoundry/korifi/releases/latest/download/install-korifi-kind.yaml
```

- If you want track the job progress, run:

```sh
kubectl -n korifi-installer logs --follow job/install-korifi
```

- **Optional** After the job is complete you can delete the `korifi-installer` namespace

```sh
kubectl delete namespace korifi-installer
```

## Test Korifi

- Target the api:

```sh
cf api https://localhost --skip-ssl-validation
```

- Authenticate as the cf admin user:

```sh
cf auth kind-korifi
```

- Create and target an org and a space

```sh
cf create-org org && cf create-space -o org space && cf target -o org
```

- Push a buildpack app and access it:

```sh
make build-dorifi
cf push dorifi -p tests/assets/dorifi
curl -k https://dorifi.apps-127-0-0-1.nip.io
```

- Push a docker app and access it:

```sh
cf push nginx --docker-image nginxinc/nginx-unprivileged:1.23.2
curl -k https://nginx.apps-127-0-0-1.nip.io
```

## Cleanup

When you no longer need korifi you can delete the whole kind cluster via:

```sh
kind delete cluster --name korifi
```
