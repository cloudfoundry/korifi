# Development Workflows

## Prerequisites

In order to build & test korifi yourself, consider installing:

-   [Golang](https://go.dev/doc/install)
-   [Docker](https://docs.docker.com/get-docker/)
-   [kind](https://kubernetes.io/docs/tasks/tools/#kind)
-   [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
-   [cf cli v8+](https://docs.cloudfoundry.org/cf-cli/install-go-cli.html)
-   [helm](https://helm.sh/docs/intro/install/)
-   [kbld](https://carvel.dev/kbld/docs/develop/install/)

## Running the tests

```
make test
```

> Note: Some Controller and API tests run a real Kubernetes API/etcd via the [`envtest`](https://book.kubebuilder.io/reference/envtest.html) package. These tests rely on the CRDs from `controllers` subdirectory.
> To update these CRDs use the `make generate` target described below.

> Note: e2e tests deploy korifi to a local kind cluster configured with port forwarding and a local docker registry before running a set of tests to interact with the CloudFoundry API. This test suite will fail if you already have Korifi deployed locally on a standard kind cluster, or you have some other process using ports 80 or 443.

## Deploying locally

This is the easiest method for deploying a kick-the-tires installation, or testing code changes end-to-end. It deploys Korifi on a local kind cluster with a local docker registry.

```
./scripts/deploy-on-kind <kind-cluster-name>
```

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
