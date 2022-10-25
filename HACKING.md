# Development Workflows

## Prerequisites

In order to build & test korifi yourself, consider installing:

- [Golang](https://go.dev/doc/install)
- [Docker](https://docs.docker.com/get-docker/)
- [kind](https://kubernetes.io/docs/tasks/tools/#kind)
- [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
- [cf cli v8+](https://docs.cloudfoundry.org/cf-cli/install-go-cli.html)
- [helm](https://helm.sh/docs/intro/install/)
- [kbld](https://carvel.dev/kbld/docs/develop/install/)


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

> Note: Some Controller and API tests run a real Kubernetes API/etcd via the [`envtest`](https://book.kubebuilder.io/reference/envtest.html) package. These tests rely on the CRDs from `controllers` subdirectory.
> To update these CRDs use the `make generate-controllers` target described below.

or

```
make test-e2e
```

> Note: e2e tests deploy korifi to a local kind cluster before running a set of tests to interact with the CloudFoundry API. This test suite will fail if you already have Korifi deployed locally, or you have some other process using ports 80 or 443.


## Deploying to `kind` with a locally deployed container registry
This is the easiest method for deploying a kick-the-tires installation, or testing code changes end-to-end.

```
./scripts/deploy-on-kind <kind-cluster-name>
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
API endpoint: https://localhost

1. cf-admin
2. kind-test
```

Of these options, `cf-admin` is the user with permissions set up by default. Selecting the other user may allow you to login and
successfully create resources, but you may notice that the user lacks the permissions to list those resources once created.


## Deploying to `kind` for remote debugging with a locally deployed container registry
This is the above method, but run with `dlv` for remote debugging.

```
./scripts/deploy-on-kind <kind-cluster-name> --debug
```
To remote debug, connect with `dlv` on `localhost:30051` (controller), `localhost:30052` (api), `localhost:30053` (kpack-image-builder), `localhost:30054` (statefulset-runner), or `localhost:30055` (job-task-runner).

A sample VSCode `launch.json` configuration is provided below:
```
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Attach to Debug Controllers on Kind",
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

> Note also that images build for debugging are based on an Ubuntu container
> image, rather than distroless, so deploying with --debug is useful if you plan
> to `kubectl exec` into the running containers for any reason.


## Install all dependencies with script
```sh
scripts/install-dependencies.sh
```


## Image tagging conventions

We store korifi docker images on docker hub.
These are:

-   korifi-api
-   korifi-controllers
-   korifi-kpack-image-builder
-   korifi-statefulset-runner
-   korifi-job-task-runner

Each time a commit is merged into the main branch, a image will be stored tagged with a `dev` tag.
The format is `dev-<next-release>-<commit sha>`.

When a new korifi version is released, the images will be tagged with the release version, e.g. `0.2.0`.
These will also be tagged as `latest`.
This way the `latest` tag always refers to the latest _release_ and not the head of the main branch.

## Build, Install and Deploy to a K8s cluster

Set the $IMG_XXX environment variables to locations where you have push/pull access. For example:

```sh
export IMG_CONTROLLERS=foo/korifi-controllers:bar #Replace this with your image ref
export IMG_API=foo/korifi-api:bar #Replace this with your image ref
export IMG_KIB=foo/korifi-kpack-image-builder:bar #Replace this with your image ref
export IMG_SSR=foo/korifi-statefulset-runner:bar #Replace this with your image ref
export IMG_JTR=foo/korifi-job-task-runner:bar #Replace this with your image ref
make generate docker-build docker-push deploy
```

This will generate the CRD bases, build and push images with the repository and tags specified by the environment variables, install CRDs and deploy the controller manager and API Shim.

## Sample Resources
The `samples` directory contains examples of the custom resources that are created by the API shim when a user interacts with the API. They are useful for testing controllers in isolation.
### Apply sample instances of the resources
```
kubectl apply -f controllers/config/samples/. --recursive
```

## Using the Makefile

### Running controllers locally

```
make run-controllers
```

### Running the API Shim Locally

```
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

```
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
