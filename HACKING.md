# Development Workflows

## Prerequisites

In order to build and test Korifi yourself, consider installing:

-   [Go](https://go.dev/doc/install)
-   [`golangci-lint`](https://golangci-lint.run/usage/install/)
-   [Docker](https://docs.docker.com/get-docker/)
-   [`kind`](https://kind.sigs.k8s.io/docs/user/quick-start/)
-   [`kubectl`](https://kubernetes.io/docs/tasks/tools/#kubectl)
-   [`cf` v8+](https://docs.cloudfoundry.org/cf-cli/install-go-cli.html)
-   [Helm](https://helm.sh/docs/intro/install/)
-   [`kbld`](https://carvel.dev/kbld/docs/develop/install/)

## Running the tests

```sh
make test
```

> **Note**
> Some Controller and API tests run a real Kubernetes API/etcd via the [`envtest`](https://book.kubebuilder.io/reference/envtest.html) package. These tests rely on the CRDs from the `controllers` subdirectory.
> To update these CRDs use the `make generate` target described below.

> **Note**
> End-to-end tests deploy Korifi to a local `kind` cluster configured with port forwarding and a local Docker registry before running a set of tests to interact with the Cloud Foundry API. This test suite will fail if you already have Korifi deployed locally on a standard `kin`d cluster, or you have some other process using ports `80` or `443`.

## Deploying locally

This is the easiest method for deploying a kick-the-tires installation, or testing code changes end-to-end. It deploys Korifi on a local `kind` cluster with a local Docker registry.

```sh
./scripts/deploy-on-kind.sh <kind-cluster-name>
```

### User Permissions Disclaimer

When using `scripts/deploy-on-kind.sh`, you will get a separate `cf-admin` user by default with which to interact with the CF API.

So when prompted to select a user by the CLI you may see something like:

```sh
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
./scripts/deploy-on-kind.sh <kind-cluster-name> --debug
```

To remote debug, connect with `dlv` on `localhost:30051` (`controllers`) or `localhost:30052` (`api`).

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

> **Note**
> Images built for debugging are based on an Ubuntu container image, rather than distroless, so deploying with `--debug` is useful if you plan to `kubectl exec` into the running containers for any reason.

## Image tagging conventions

We store Korifi container images on [DockerHub](https://hub.docker.com/u/cloudfoundry).
These are:
-   [`korifi-api`](https://hub.docker.com/r/cloudfoundry/korifi-api);
-   [`korifi-controllers`](https://hub.docker.com/r/cloudfoundry/korifi-controllers).

Each time a commit is merged into the `main` branch, a image will be pushed and tagged with a `dev` tag.
The format is `dev-<next-release>-<commit-sha>`.

When a new Korifi version is released, the images will be tagged with the release version, e.g. `0.2.0`.
These will also be tagged as `latest`.
This way the `latest` tag always refers to the latest _release_ and not the head of the `main` branch.
