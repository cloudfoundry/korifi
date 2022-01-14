# Merge CF on K8s Repositories

Date: 2021-10-26

## Status

Accepted

## Context

The `cf-k8s-api` and `cf-k8s-controllers` repositories were originally created separately to create a deliberate interface between "client" and "server" code. The intention was to motivate developers and users to reason about, change, and install the pieces separately. Instead, it has become difficult to decide which repository should house helper code and dependencies. Recently, we started working on a new `cf-on-k8s-integration` repository to pull the two components together and provide a home for both installation instructions and CI configuration for testing. As we worked on initialising this new repository, it became clear that different types of users wanted different things from it:

* developers want a single place to work regardless of which component they are modifying
* users want a single place to install both components and any required dependencies

## Decision

* The contents of the existing `cf-k8s-api` and `cf-k8s-controllers` repositories are going to be merged into a single repository.
* The `cf-k8s-controllers` repository will be reused for this purpose.
* The current separation between the API and controllers code bases will be maintained using sub-directories in the new repository.
* There will be a shared `go.mod` that defines the Go dependencies for both components.
* The interface between the API and the controllers will continue to be the CF custom resource definitions.
* The API and controllers can still be installed independently.
* All CI artifacts related to the building and testing of this single repository will live in either the `ci` or `.github` directories.

## Consequences

* Go packaging may change as both code bases become included in a single Go module with package-level separation.
* GitHub actions need to trigger on and focus specific directories.
* Any OCI annotations on the images that may depend on repository need to be updated.
