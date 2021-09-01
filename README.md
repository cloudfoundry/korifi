# cf-k8s-api

## API Endpoint Docs
- [API Endpoints](docs/api.md)

## Dependencies
There is a dependency on [cf-k8s-controllers CRDs](https://github.com/cloudfoundry/cf-k8s-controllers), which can be sources with [vendir.](https://carvel.dev/vendir/) Run `vendir sync` once you have sourced the vendir binary.

## How to run locally
make
```make
make run
```
shell
```shell
go run main.go
```

## How to run tests
make
```make
make test
```
shell (testbin must be sourced first if using this method)
```shell
KUBEBUILDER_ASSETS=$PWD/testbin/bin go test ./... -coverprofile cover.out
```

## Edit local configuration
To specify a custom configuration file, set the `CONFIG` environment variable to its path when running the web server.
Refer to the [default config](config.json) for the config file structure and options.