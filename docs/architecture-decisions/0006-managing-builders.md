# Managing Builders

Date: 2021-09-15

## Status

Accepted

## Context

### CF App Staging Buildpack and Stack selection
The CF API allows users to specify a list of `buildpacks` and a `stack` when `lifecycletype` is set to `buildpacks` while creating apps
or while initiating staging by creating builds.

As the lifecycle field is optional on both create app and create build requests, we want a default available.

If lifecycle data is specified, the precedence we follow is `build lifecycle > app lifecycle > default lifecycle`.

On Kubernetes we use `Kpack` for staging. In order to do that we need a `kpack builder` referencing a set of `buildpacks` and `stacks`. To provide default `buildpacks` and `stacks` for app staging, we define a default kpack builder.

Below are the possible permutation of lifecycle scenarios and kpack builders to use

- App created with empty lifecycle & Build created with empty lifecycle
    - Default builder is used
- App created with lifecycle data & Build created with empty lifecycle
  - Check if a builder exists within the same namespace referencing only the app-guid.
    - if it exists, the builder referencing only the app-guid is reused.
    - if its doesn't exist, a new builder with specified `buildpacks` and `stacks` is created.
- Build created with lifecycle data
    - A new builder with specified `buildpacks` and `stacks` is created

### CF Stack and Buildpack Updates
On CF for VMs, when we update a stack, running apps are updated to use the updated stack. Note that for now, we are excluding droplet rollback concerns from our decision.

On CF for VMs, when we update a buildpack, running apps are not automatically rebuilt to use the new buildpack.

After a stack is updated, subsequent builds will use the new stack, so we focus on the cases of running apps and assigning droplets for which there is not necessarily a required restage.

## Decision
### Default Cluster Builder
Operator is responsible for creating a default cluster builder, and it has be named as `cf-kpack-cluster-builder`.
Operator is also responsible for creating service-account & secrets for access to a single registry on each namespace.
We expect the operators to follow `kpack-service-account` naming convention for service accounts.

### Handling Stack and Buildpack Updates
At kpack Builder update time (for both Stack and Buildpack), the kpack controller will see the changes, find affected kpack Images, rebuild them, and add a new status on the Image with the reason for rebuild. A CF controller will see the changes on the kpack Image status, find affected CFApps by builder guid label, and update the droplet image ref of the associated CFBuild with the newly built image ref.

The CF controller must intentionally ignore changes triggered by buildpack updates. When a buildpack is updated, the operator is responsible for explicitly rebuilding apps that rely on that buildpack as desired.

### App-Specific Builders
In the context above we have captured details about when builders other than the default cluster builder are created. That behavior may be implemented in a subsequent iteration alongside the lifecycle data on CFApp and CFBuild resources.
At creation time for such a builder we will label it with `app-guid` and/or `build-guid`.
Additionally, we will track the readiness of those builders using `status` conditions on the builder.
We will not initially clean up builders or associated images.
We will although be reusing existing builder based on labels as detailed above.

## Consequences

### Operator Responsibility for Buildpack Updates
The platform operator must manually manage buildpack updates by restaging apps and rolling back as necessary on errors. As the api does not necessarily provide kpack error details, the operator will need access to kpack resources and troubleshooting documentation.

### CF API Owners
The CF API owners must implement a controller reconciliation loop to update CFApp and related resources based on kpack Image updates triggered by Stack updates.
