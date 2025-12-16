# Buildpacks
Korifi utilizes [kpack](https://github.com/buildpacks-community/kpack) to build Container Images combining [Cloud Native Buildpacks (CNB)](https://buildpacks.io) with application source code.

## Default Buildpacks

If you choose Helm to deploy Korifi some default buildpacks are pre-configured for you.
You can see the available buildpacks via the CF CLI using the command 

```bash
cf buildpacks
Getting buildpacks as cf-admin...

position   name                            stack                        enabled   locked   filename
1          paketo-buildpacks/java          io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/java@17.0.0
2          paketo-buildpacks/go            io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/go@4.12.2
3          paketo-buildpacks/nodejs        io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/nodejs@4.2.1
4          paketo-buildpacks/ruby          io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/ruby@0.47.6
5          paketo-buildpacks/procfile      io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/procfile@5.10.0
```

Those default buildpacks are configured within the `ClusterStore` and `ClusterBuilder` of the Helm Chart, which can be found [here](../helm/korifi/kpack-image-builder/cluster-builder.yaml)
 
## Custom Buildpacks

If you want to make more buildpacks than the default ones available, you have two possibilities. You can either adjust the `ClusterStore` and `ClusterBuilder` within your cluster to include further buildpacks or you provide your own `ClusterBuilder` and use it instead.

### Adjust the existing ClusterBuilder/ ClusterStore 

As an example we will add the [paketo/web-servers](https://github.com/paketo-buildpacks/web-servers) buildpack to Korifi and deploy an app with it.

#### Adjusting ClusterStore

First we get the current `ClusterStore` applied on the cluster. The default name is `cf-default-buildpacks`. 

```yaml
kubectl get ClusterStore cf-default-buildpacks -o yaml

apiVersion: kpack.io/v1alpha2
kind: ClusterStore
metadata:
  annotations:
    meta.helm.sh/release-name: korifi
    meta.helm.sh/release-namespace: korifi
  creationTimestamp: "2024-11-04T12:58:59Z"
  generation: 2
  labels:
    app.kubernetes.io/managed-by: Helm
  name: cf-default-buildpacks
  resourceVersion: "4105"
  uid: 374265ba-9dd8-4769-8363-76ab2b94d56e
spec:
  sources:
  - image: paketobuildpacks/java
  - image: paketobuildpacks/nodejs
  - image: paketobuildpacks/ruby
  - image: paketobuildpacks/procfile
  - image: paketobuildpacks/go
...
``` 
We now need to add the image for the new buildpack `web-servers` we want to use under `.spec.sources`. For that we use the patch command below:

```yaml
kubectl patch ClusterStore/cf-default-buildpacks --type json -p '[{"op": "add", "path": "/spec/sources/-","value": {"image": "paketobuildpacks/web-servers"}}]'
```

Afterwards we can see that sources now includes web-servers as well

```yaml
kubectl get ClusterStore cf-default-buildpacks -o jsonpath={".spec.sources"} | jq
[
  {
    "image": "paketobuildpacks/java"
  },
  {
    "image": "paketobuildpacks/nodejs"
  },
  {
    "image": "paketobuildpacks/ruby"
  },
  {
    "image": "paketobuildpacks/procfile"
  },
  {
    "image": "paketobuildpacks/go"
  },
  {
    "image": "paketobuildpacks/web-servers"
  }
]
```

#### Adjusting ClusterBuilder

The next step to make the buildpacks available to CF is to adjust the `ClusterBuilder`.
An example of the `ClusterBuilder` looks like this. The default name is `cf-kpack-cluster-builder`.

```yaml
kubectl get ClusterBuilder cf-kpack-cluster-builder -o yaml
apiVersion: kpack.io/v1alpha2
kind: ClusterBuilder
metadata:
  annotations:
    meta.helm.sh/release-name: korifi
    meta.helm.sh/release-namespace: korifi
  creationTimestamp: "2024-11-04T14:37:37Z"
  generation: 1
  labels:
    app.kubernetes.io/managed-by: Helm
  name: cf-kpack-cluster-builder
  resourceVersion: "5138"
  uid: ff1ab7ea-4482-4e25-a959-4587d33314d2
spec:
  order:
  - group:
    - id: paketo-buildpacks/java
  - group:
    - id: paketo-buildpacks/go
  - group:
    - id: paketo-buildpacks/nodejs
  - group:
    - id: paketo-buildpacks/ruby
  - group:
    - id: paketo-buildpacks/procfile
  serviceAccountRef:
    name: kpack-service-account
    namespace: cf
  stack:
    kind: ClusterStack
    name: cf-default-stack
  store:
    kind: ClusterStore
    name: cf-default-buildpacks
  tag: localregistry-docker-registry.default.svc.cluster.local:30050/kpack-builder
...
```
We will now add the `web-server` buildpack as a new element to `.spec.order` which makes it appear for the CF CLI.

```yaml
kubectl patch ClusterBuilder/cf-kpack-cluster-builder --type json -p '[{"op":"add", "path":"/spec/order/-","value": {"group": [{"id": "paketo-buildpacks/web-servers"}]}}]'
```

After that you can see the buildpack using the CF CLI.

```yaml
cf buildpacks
Getting buildpacks as cf-admin...

position   name                            stack                        enabled   locked   filename
1          paketo-buildpacks/java          io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/java@17.0.0
2          paketo-buildpacks/go            io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/go@4.12.2
3          paketo-buildpacks/nodejs        io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/nodejs@4.2.1
4          paketo-buildpacks/ruby          io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/ruby@0.47.6
5          paketo-buildpacks/procfile      io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/procfile@5.10.0
6          paketo-buildpacks/web-servers   io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/web-servers@0.27.1
```

### Provide your own ClusterBuilder/ ClusterStore 

Instead on relying on the kpack `ClusterBuilder`/ `ClusterStore` provided by the Helm Chart you can deploy your own set of `ClusterBuilder`, `ClusterStore` and `ClusterStack` and make the Helm Chart use this one.

The following steps will only provide very basic manifests to add a new buildpack, for more details on the matter we recommend the [kpack documentation](https://github.com/buildpacks-community/kpack/blob/main/docs/build.md) or for this specific case the [kpack Tutorial](https://github.com/buildpacks-community/kpack/blob/main/docs/tutorial.md) to ensure all pre-requisits are met to perform the steps below.

Lets start with the `ClusterStore`.
For that copy the manifest down below into a file called `store.yaml` and run `kubectl apply -f store.yaml`.

```yaml
apiVersion: kpack.io/v1alpha2
kind: ClusterStore
metadata:
  name: custom-cluster-store
spec:
  sources:
  - image: paketobuildpacks/web-servers
```

Please check that the `ClusterStore` was succesfully created

```yaml
kubectl get ClusterStore
NAME                   READY
custom-cluster-store   True
```

Next we need a `ClusterStack` where we specify which stack should be used building the application image. For that please store the manifest below into a file called `stack.yaml` and run `kubectl apply -f stack.yaml`.

```yaml
apiVersion: kpack.io/v1alpha2
kind: ClusterStack
metadata:
  name: custom-cluster-stack
spec:
  id: "io.buildpacks.stacks.jammy"
  buildImage:
    image: "paketobuildpacks/build-jammy-base"
  runImage:
    image: "paketobuildpacks/run-jammy-base"
```

Verify that the `ClusterStack` has been created successfully.

```yaml
kubectl get ClusterStack
NAME                   READY
custom-cluster-stack   True
```

Now we only need a `ClusterBuilder` to have everything in place.
For that please save the manifest below into a file called `builder.yaml` and apply it `kubectl apply -f builder.yaml`.

```yaml
apiVersion: kpack.io/v1alpha2
kind: ClusterBuilder
metadata:
  name: custom-cluster-builder
  namespace: default
spec:
  serviceAccountRef:
    name: <ServiceAccount> # e.g. kpack-service-account
    namespace: <ServiceAccountNamespace> # e.g. cf
  tag: <CLUSTER-BUILDER-IMAGE-TAG> # e.g. localregistry-docker-registry.default.svc.cluster.local:30050/kpack-builder
  stack:
    name: custom-cluster-stack
    kind: ClusterStack
  store:
    name: custom-cluster-store
    kind: ClusterStore
  order:
  - group:
    - id: paketo-buildpacks/web-servers
```

Verify that the `ClusterBuilder` was created successfully.

```yaml
kubectl get ClusterBuilder
NAME                       LATESTIMAGE                                                                                                                                                                                   READY   UPTODATE
custom-cluster-builder     localregistry-docker-registry.default.svc.cluster.local:30050/kpack-builder:clusterbuilder-custom-cluster-builder@sha256:4e273a309e6679e071ab93c754617869b486fcd0bf420ddde2babf32bd1dc491     True    True
```

Now we only need to tell Helm that it should use our own custom `ClusterBuilder` to build images by setting the value `--set=kpackImageBuilder.clusterBuilderName="custom-cluster-builder"` during the Helm install command.

Afterwards we can see all buildpacks we have configured in CF.

```yaml
cf buildpacks
Getting buildpacks as cf-admin...

position   name                            stack                        enabled   locked   filename
1          paketo-buildpacks/web-servers   io.buildpacks.stacks.jammy   true      false    paketo-buildpacks/web-servers@0.27.1
```

### Verify your buildpack is working

To verify that your newly added buildpack is working you can deploy one of the [sample apps for cloud native buildpacks](https://github.com/paketo-buildpacks/samples).

In our case we use the [web-servers/nginx-sample](https://github.com/paketo-buildpacks/samples/tree/main/web-servers/nginx-sample)

```yaml
cd web-servers/nginx-sample
cf push nginx-sample -b paketo-buildpacks/web-servers
```

and after a short waiting time we can see our app is successfully deployed

```yaml
cf apps
Getting apps in org test-org / space test-space as cf-admin...

name           requested state   processes   routes
nginx-sample   started           web:1/1     nginx-sample.apps-127-0-0-1.nip.io
```

and we can as well access it via curl (or browser if you want to).

```yaml
curl -k https://nginx-sample.apps-127-0-0-1.nip.io
<!DOCTYPE html>
<html>
  <head>
    <title>Powered By Paketo Buildpacks</title>
  </head>
  <body>
    <img style="display: block; margin-left: auto; margin-right: auto; width: 50%;" src="https://paketo.io/images/paketo-logo-full-color.png"></img>
  </body>
</html>`
```
