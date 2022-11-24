# Korifi Helm chart

This documents the [Helm](https://helm.sh/) chart for [Korifi](https://github.com/cloudfoundry/korifi).

The chart is a composition of subcharts, one per component, with each individual component configuration nested under a top-level key named after the component itself.
Values under the top-level `global` key apply to all components.
Each component can be excluded from the deployment by the setting its `include` value to `false`.
See [_Customizing the Chart Before Installing_](https://helm.sh/docs/intro/using_helm/#customizing-the-chart-before-installing) for details on how to specify values when installing a Helm chart.

Here are all the values that can be set for the chart:

* `adminUserName` (_String_): Name of the admin user that will be bound to the Cloud Foundry Admin role.
* `global`
  - `rootNamespace` (_String_): Root of the Cloud Foundry namespace hierarchy.
  - `debug` (_Boolean_): Enables remote debugging with [Delve](https://github.com/go-delve/delve).
  - `defaultAppDomainName` (_String_): Base domain name for application URLs.
  - `generateIngressCertificates` (_Boolean_): Use `cert-manager` to generate self-signed certificates for the API and app endpoints.
  - `containerRegistrySecret` (_String_): Name of the `Secret` to use when pushing or pulling from package, droplet and kpack-build repositories
* `api`:
  - `include` (_Boolean_): Deploy the API component.
  - `replicas` (_Integer_): Number of replicas.
  - `resources` (_Object_): [`ResourceRequirements`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#resourcerequirements-v1-core) for the API.
  - `apiServer`:
    - `url` (_String_): API URL.
    - `port` (_Integer_): API external port. Defaults to `443`.
    - `internalPort` (_Integer_): Port used internally by the API container.
    - `timeouts`: HTTP timeouts.
      - `read` (_Integer_)
      - `write` (_Integer_)
      - `idle` (_Integer_)
      - `readHeader` (_Integer_)
  - `image` (_String_): Reference to the API container image.
  - `lifecycle`: Default lifecycle for apps.
    - `type` (_String_)
    - `stack` (_String_)
    - `stagingRequirements`:
      - `memoryMB` (_Integer_)
      - `diskMB` (_Integer_)
  - `builderName` (_String_): ID of the builder used to build apps. Defaults to `kpack-image-builder`.
  - `packageRepository` (_String_): The container image repository where app source packages will be stored. For DockerHub, this might be `index.docker.io/<username>/packages`.
  - `userCertificateExpirationWarningDuration` (_String_): Issue a warning if the user certificate provided for login has a long expiry. See [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) for details on the format.
  - `authProxy`: Needed if using a cluster authentication proxy, e.g. [Pinniped](https://pinniped.dev/).
    - `host` (_String_): Must be a host string, a host:port pair, or a URL to the base of the apiserver.
    - `caCert` (_String_): Proxy's PEM-encoded CA certificate (*not* as Base64).
* `controllers`:
  - `include` (_Boolean_): Deploy the controllers component.
  - `replicas` (_Integer_): Number of replicas.
  - `resources` (_Object_): The [`ResourceRequirements`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#resourcerequirements-v1-core) for the controllers.
  - `image` (_String_): Reference to the controllers container image.
  - `reconcilers`:
    - `build` (_String_): ID of the image builder to set on all `BuildWorkload` objects. Has to match `api.builderName`. Defaults to `kpack-image-builder`.
    - `app` (_String_): ID of the workload runner to set on all `AppWorkload` objects.
  - `processDefaults`:
    - `memoryMB` (_Integer_): Default memory limit for the `web` process.
    - `diskQuotaMB` (_Integer_): Default disk quota for the `web` process.
  - `taskTTL` (_String_): How long before the `CFTask` object is deleted after the task has completed. See [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) for details on the format, an additional `d` suffix for days is supported.
  - `workloadsTLSSecret` (_String_): TLS secret used when setting up an app routes.
* `job-task-runner`:
  - `include` (_Boolean_): Deploy the `job-task-runner` component.
  - `replicas` (_Integer_): Number of replicas.
  - `resources` (_Object_): The [`ResourceRequirements`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#resourcerequirements-v1-core) for the `job-task-runner`.
  - `image` (_String_): Reference to the `job-task-runner` container image.
  - `jobTTL` (_String_): How long before the `Job` backing up a task is deleted after completion. See [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) for details on the format, an additional `d` suffix for days is supported.
* `kpack-image-builder`:
  - `include` (_Boolean_): Deploy the `kpack-image-builder` component.
  - `replicas` (_Integer_): Number of replicas.
  - `resources` (_Object_): The [`ResourceRequirements`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#resourcerequirements-v1-core) for the `kpack-image-builder`.
  - `image` (_String_): Reference to the `kpack-image-builder` container image.
  - `dropletRepositoryPrefix` (_String_): Prefix of the container image repository where droplets will be stored. For DockerHub, this should be `index.docker.io/<username>`.
  - `clusterBuilderName` (_String_): The name of the `ClusterBuilder` Kpack has been configured with. Leave blank to let `kpack-image-builder` create an example `ClusterBuilder`.
  - `clusterStackBuildImage` (_String_): The image to use for building defined in the `ClusterStack`. Used when `kpack-image-builder` is blank.
  - `clusterStackRunImage` (_String_): The image to use for running defined in the `ClusterStack`. Used when `kpack-image-builder` is blank.
  - `builderRepository` (_String_): Container image repository to store the `ClusterBuilder` image. Required when `clusterBuilderName` is not provided.
* `statefulset-runner`:
  - `include` (_Boolean_): Deploy the `statefulset-runner` component.
  - `replicas` (_Integer_): Number of replicas.
  - `resources` (_Object_): The [`ResourceRequirements`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#resourcerequirements-v1-core) for the `statefulset-runner`.
  - `image` (_String_): Reference to the `statefulset-runner` container image.
