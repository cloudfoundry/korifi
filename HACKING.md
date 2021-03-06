# Development Workflows

## Prerequisites

In order to build & install images yourself, consider installing:
- [Docker](https://docs.docker.com/get-docker/)
- [kind](https://kubernetes.io/docs/tasks/tools/#kind)
- [kubebuilder](https://book-v1.book.kubebuilder.io/getting_started/installation_and_setup.html)
- [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/)

---
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
> Note: Some API tests run a real Kubernetes API/etcd via the [`envtest`](https://book.kubebuilder.io/reference/envtest.html) package. These tests rely on the CRDs from `controllers` subdirectory.
> To update these CRDs use the `make generate-controllers` target described below.

---
## Deploying to `kind` with a locally deployed container registry
This is the easiest method for deploying a kick-the-tires installation, or testing code changes end-to-end.

```
./scripts/deploy-on-kind <kind-cluster-name> --local
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
API endpoint: http://localhost
Warning: Insecure http API endpoint detected: secure https API endpoints are recommended

1. cf-admin
2. kind-test
```

Of these options, `cf-admin` is the user with permissions set up by default. Selecting the other use may allow you to login and
successfully create resources, but you may notice that the user lacks the permissions to list those resources once created.

---
## Deploying to `kind` for remote debugging with a locally deployed container registry
This is the above method, but run with `dlv` for remote debugging.

```
./scripts/deploy-on-kind <kind-cluster-name> --local --debug
```
To remote debug, connect with `dlv` on `localhost:30051` (controller) or `localhost:30052` (api)

A sample VSCode `launch.json` configuration is provided below:
```
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Attach to Debug Controller on Kind",
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


---
## Install all dependencies with script
```sh
# modify kpack dependency files to point towards your registry
scripts/install-dependencies.sh -g "<PATH_TO_GCR_CREDENTIALS>"
```

---
## Build, Install and Deploy to a K8s cluster
Set the $IMG_CONTROLLERS and $IMG_API environment variables to locations where you have push/pull access. For example:
```sh
export IMG_CONTROLLERS=foo/korifi-controllers:bar #Replace this with your image ref
export IMG_API=foo/korifi-api:bar #Replace this with your image ref
make generate-controllers docker-build docker-push deploy
```
*This will generate the CRD bases, build and push images with the repository and tags specified by the environment variables, install CRDs and deploy the controller manager and API Shim.*

---
## Sample Resources
The `samples` directory contains examples of the custom resources that are created by the API shim when a user interacts with the API. They are useful for testing controllers in isolation.
### Apply sample instances of the resources
```
kubectl apply -f controllers/config/samples/. --recursive
```

---
## Using the Makefile

### Running controllers locally
```sh
make run-controllers
```

### Running the API Shim Locally

```make
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
```sh
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


### Generate reference yaml
Build reference yaml (with defaults) to be applied with kubectl
```
make build-reference
```
or
```
make build-reference-controllers
```
or
```
make build-reference-api
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
