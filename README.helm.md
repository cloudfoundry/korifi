# Korifi Helm chart

This documents the [Helm](https://helm.sh/) chart for [Korifi](https://github.com/cloudfoundry/korifi).

The chart is a composition of subcharts, one per component, with each individual component configuration nested under a top-level key named after the component itself.
Values under the top-level `global` key apply to all components.
Each component can be excluded from the deployment by the setting its `include` value to `false`.
See [_Customizing the Chart Before Installing_](https://helm.sh/docs/intro/using_helm/#customizing-the-chart-before-installing) for details on how to specify values when installing a Helm chart.

Here are all the values that can be set for the chart:

- `adminUserName` (_String_): Name of the admin user that will be bound to the Cloud Foundry Admin role.
- `global`: Global values that are shared between Korifi and its subcharts.
  - `containerRegistrySecret` (_String_): Name of the `Secret` to use when pushing or pulling from package, droplet and kpack-build repositories. Required if eksContainerRegistryRoleARN not set. Ignored if eksContainerRegistryRoleARN is set.
  - `containerRepositoryPrefix` (_String_): The prefix of the container repository where package and droplet images will be pushed. This is suffixed with the app GUID and `-packages` or `-droplets`. For example, a value of `index.docker.io/korifi/` will result in `index.docker.io/korifi/<appGUID>-packages` and `index.docker.io/korifi/<appGUID>-droplets` being pushed.
  - `debug` (_Boolean_): Enables remote debugging with [Delve](https://github.com/go-delve/delve).
  - `defaultAppDomainName` (_String_): Base domain name for application URLs.
  - `eksContainerRegistryRoleARN` (_String_): Amazon Resource Name (ARN) of the IAM role to use to access the ECR registry from an EKS deployed Korifi. Required if containerRegistrySecret not set.
  - `generateIngressCertificates` (_Boolean_): Use `cert-manager` to generate self-signed certificates for the API and app endpoints.
  - `rootNamespace` (_String_): Root of the Cloud Foundry namespace hierarchy.
- `api`:
  - `apiServer`:
    - `internalPort` (_Integer_): Port used internally by the API container.
    - `port` (_Integer_): API external port. Defaults to `443`.
    - `timeouts`: HTTP timeouts.
      - `idle` (_Integer_): Idle timeout.
      - `read` (_Integer_): Read timeout.
      - `readHeader` (_Integer_): Read header timeout.
      - `write` (_Integer_): Write timeout.
    - `url` (_String_): API URL.
  - `authProxy`: Needed if using a cluster authentication proxy, e.g. [Pinniped](https://pinniped.dev/).
    - `caCert` (_String_): Proxy's PEM-encoded CA certificate (*not* as Base64).
    - `host` (_String_): Must be a host string, a host:port pair, or a URL to the base of the apiserver.
  - `builderName` (_String_): ID of the builder used to build apps. Defaults to `kpack-image-builder`.
  - `image` (_String_): Reference to the API container image.
  - `include` (_Boolean_): Deploy the API component.
  - `lifecycle`: Default lifecycle for apps.
    - `stack` (_String_): Stack.
    - `stagingRequirements`:
      - `diskMB` (_Integer_): Disk in MB for staging.
      - `memoryMB` (_Integer_): Memory in MB for staging.
    - `type` (_String_): Lifecycle type (only `buildpack` accepted currently).
  - `replicas` (_Integer_): Number of replicas.
  - `resources`: [`ResourceRequirements`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#resourcerequirements-v1-core) for the API.
    - `limits`: Resource limits.
      - `cpu` (_String_): CPU limit.
      - `memory` (_String_): Memory limit.
    - `requests`: Resource requests.
      - `cpu` (_String_): CPU request.
      - `memory` (_String_): Memory request.
  - `userCertificateExpirationWarningDuration` (_String_): Issue a warning if the user certificate provided for login has a long expiry. See [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) for details on the format.
- `controllers`:
  - `image` (_String_): Reference to the controllers container image.
  - `include` (_Boolean_): Deploy the controllers component.
  - `processDefaults`:
    - `diskQuotaMB` (_Integer_): Default disk quota for the `web` process.
    - `memoryMB` (_Integer_): Default memory limit for the `web` process.
  - `reconcilers`:
    - `app` (_String_): ID of the workload runner to set on all `AppWorkload` objects. Defaults to `statefulset-runner`.
    - `build` (_String_): ID of the image builder to set on all `BuildWorkload` objects. Has to match `api.builderName`. Defaults to `kpack-image-builder`.
  - `replicas` (_Integer_): Number of replicas.
  - `resources`: [`ResourceRequirements`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#resourcerequirements-v1-core) for the API.
    - `limits`: Resource limits.
      - `cpu` (_String_): CPU limit.
      - `memory` (_String_): Memory limit.
    - `requests`: Resource requests.
      - `cpu` (_String_): CPU request.
      - `memory` (_String_): Memory request.
  - `taskTTL` (_String_): How long before the `CFTask` object is deleted after the task has completed. See [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) for details on the format, an additional `d` suffix for days is supported.
  - `workloadsTLSSecret` (_String_): TLS secret used when setting up an app routes.
- `job-task-runner`:
  - `image` (_String_): Reference to the `job-task-runner` container image.
  - `include` (_Boolean_): Deploy the `job-task-runner` component.
  - `jobTTL` (_String_): How long before the `Job` backing up a task is deleted after completion. See [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) for details on the format, an additional `d` suffix for days is supported.
  - `replicas` (_Integer_): Number of replicas.
  - `resources`: [`ResourceRequirements`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#resourcerequirements-v1-core) for the API.
    - `limits`: Resource limits.
      - `cpu` (_String_): CPU limit.
      - `memory` (_String_): Memory limit.
    - `requests`: Resource requests.
      - `cpu` (_String_): CPU request.
      - `memory` (_String_): Memory request.
- `kpack-image-builder`:
  - `builderRepository` (_String_): Container image repository to store the `ClusterBuilder` image. Required when `clusterBuilderName` is not provided.
  - `clusterBuilderName` (_String_): The name of the `ClusterBuilder` Kpack has been configured with. Leave blank to let `kpack-image-builder` create an example `ClusterBuilder`.
  - `clusterStackBuildImage` (_String_): The image to use for building defined in the `ClusterStack`. Used when `kpack-image-builder` is blank.
  - `clusterStackRunImage` (_String_): The image to use for running defined in the `ClusterStack`. Used when `kpack-image-builder` is blank.
  - `image` (_String_): Reference to the `kpack-image-builder` container image.
  - `include` (_Boolean_): Deploy the `kpack-image-builder` component.
  - `replicas` (_Integer_): Number of replicas.
  - `resources`: [`ResourceRequirements`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#resourcerequirements-v1-core) for the API.
    - `limits`: Resource limits.
      - `cpu` (_String_): CPU limit.
      - `memory` (_String_): Memory limit.
    - `requests`: Resource requests.
      - `cpu` (_String_): CPU request.
      - `memory` (_String_): Memory request.
- `statefulset-runner`:
  - `image` (_String_): Reference to the `statefulset-runner` container image.
  - `include` (_Boolean_): Deploy the `statefulset-runner` component.
  - `replicas` (_Integer_): Number of replicas.
  - `resources`: [`ResourceRequirements`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#resourcerequirements-v1-core) for the API.
    - `limits`: Resource limits.
      - `cpu` (_String_): CPU limit.
      - `memory` (_String_): Memory limit.
    - `requests`: Resource requests.
      - `cpu` (_String_): CPU request.
      - `memory` (_String_): Memory request.
