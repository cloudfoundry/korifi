# Kubernetes CRD tests

These tests run a "smoke test" of the Korifi system, driven exclusively by the kubernetes API (via our Custom Resources).

## Prerequisites

Before running these tests, you must be targeting a kubernetes cluster that has korifi installed. This can be done via
the default kubeconfig or by setting the `KUBECONFIG` environment variable. Your currently selected context must be for
a user that has the ability to get/list namespaces and to get/list/create/update/patch/delete all korifi
Custom Resources (e.g. CFOrg, CFSpace).

For local testing against a kind cluster, you can simply run the `scripts/deploy-on-kind.sh` script to bring up an
environment and then follow the instructions below to run the tests.

## Running the tests

Simply run `ginkgo` from this directory or run `ginkgo ./tests/crds` from the project root. 

## Configuration

If your deployment uses a non-standard root namespace (default is `cf`), then you must set the `ROOT_NAMESPACE`
environment variable when running the tests.
