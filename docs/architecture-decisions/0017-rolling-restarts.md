# App Restart with rolling strategy

Date: 2024-06-28

## Status

Accepted

## Context

The purpose of this ADR is to capture important architectural decisions made when introducing no-donwtime restart and restage by implementing some of the [`/v3/deployments`](https://v3-apidocs.cloudfoundry.org/version/3.159.0/index.html#deployments) endpoints so that `cf restart/restage --strategy=rolling` commands are supported. This ADR comes after the fact in order to eliminate future confusion when reasoning about app revision annotations. It is closely related to ADR [#0011](0011-document-cf-restart-implementation.md) that documents the implementation of plain `cf restart` with downtime and partly overlaps with it to reflect changes in the underlying Korifi CRDs that have been made since the publising of that ADR.

For reference, rolling restart support has been introduced by the following issues:
- [#2525](https://github.com/cloudfoundry/korifi/pull/2525)
- [#2559](https://github.com/cloudfoundry/korifi/pull/2559)

It has been available since Korifi [v0.8.0](https://github.com/cloudfoundry/korifi/releases/tag/v0.8.0)

## Decision

### In a nutshell

- A new `last-stop-app-rev` annotation is introduced on the `CFApp` object
- The `CFApp` webhook has the responsibility of bumping both the old `app-rev` and the new `last-stop-app-rev` annotations when the `CFApp` is being stopped, i.e. when `.Spec.DesiredState` transitions from `STARTED` to `STOPPED`
- The name of the `AppWorkload` object is being calculated using the new `last-stop-app-rev` annotation. This way the `AppWorkload` is recreated if and only if the app was explicily stopped by the user. In all other cases the `AppWorkload` lives on, but the `app-rev` annotation is changed, leading to a change in the spec of the underlying `StatfulSet`. This change causes the `StatfulSet`'s `Pod`s to be rolled out, which means the app will experience no downtime provided that it is running more that one instance.

### The slightly longer version

#### How `cf restart` works

This is where this ADR overlaps with #0011, but this has been done for completeness in order to reflect some changes that were introduced in the meantime.

- The cli performs a `POST /v3/apps/{guid}/actions/restart` request
- The API shim sets the `CFApp`'s `.Spec.DesiredState` to `STOPPED`, then immediately to `STARTED`
- The `CFApp`'s `AppRevWebhook` bumps the `app-rev` annotation and sets the new `last-stop-app-rev` annotation to the bumped value. At this point the two annotations have the same value.
- The `CFProcess` controller calculates the name of the `AppWorkload` object to be `${process-guid}-sha1(${last-stop-app-rev})`. Thanks to the fact that the webhook bumped both `app-rev` and `last-stop-app-rev` annotations the `AppWorkload` name is guaranteed to have changed, so a new `AppWorkload` object with a new name is created for the given `CFProcess` next to the old one. The controller then lists all `AppWorkload` for the `CFProcess`, deleting the ones that have desired state `STOPPED`, zero instances, or stale `last-stop-app-rev` annotation.
- The `AppWorkload` controller creates a new `StatfulSet` with the bumped `app-rev` annotation. Re-creating the statefulset like this causes app downtime, which is the desired behaviour of a `cf restart`

#### How `cf restart --strategy=rolling` works

- The cli performs a `POST /v3/deployments` request
- The API shim looks up the related `CFApp` and bumps its `app-rev` annotation, leaving the `last-stop-app-rev` annotation unchanged. This is the only case when those two annotations become out of sync.
- The `CFApp`'s `AppRevWebhook` is a noop, because the app state is not changed.
- The `CFProcess` controller calculates the name of the `AppWorkload` object to be `${process-guid}-sha1(${last-stop-app-rev})`. Thanks to `last-stop-app-rev` staying unchanged the `AppWorkload` name does not change.
- As a consequence the `AppWorkload` controller updates the existing `StatfulSet` with the new `app-rev` value. This causes the underlying `Pod`s to be rolled out, menaning that the app won't experience downtime if it has been previously scalse to more than one instance.
- The cli performs a `GET /v3/deployments/{guid}` call
- The API shim looks up the related `CFApp` and sets the dployment status accordingly:
  - If the `CFApp` has a `Ready=true` status condition, the deployment is in `DEPLOYED` state
  - In all other cases the deployment is in `DEPLOYING` state
- Once the app is rolled out and the deployment becomes `DEPLOYED` the cf cli operation completes

This solution is backwards compatible, since the `AppWorkload` names of legacy apps (ones deployed before this feature is introduced) don't change unless someone tries to restart them with strategy rolling.

#### Issues and mitigations

Legacy apps, or apps that were deployed before this feature was introduced will not be able to be restarted using the new rolling strategy out of the box. The rolling strategy support relies on the pod selector of the underlying stateful set being [changed](https://github.com/cloudfoundry/korifi/commit/00593c9297eb58c26bc2ca3411d0ebe03b6441f3#diff-fc3e21b7327c763ed9d6434df5b86ce07f0e3aa8b41e885fb3a0da637acb3f3bR240) to remove app version. Since the pod selector is immutable, this requires the statefulset to be recreated, which can be achieved by a `cf push`, `restage` or `restart` as long as the `--strategy=rolling` option is not passed.

In simple words this means that no legacy app should be restared with strategy rolling before it is recreated by the means of old fashioned `cf restart`. This defect is mitigated by [#2559](https://github.com/cloudfoundry/korifi/pull/2559) that introduces a menaningful error explaining this to the users, preventing them from breaking their apps.

#### Implications for other API endpoints

The introcuction of the `app-rev` and the `last-stop-app-rev` annotations and the fact that there can be pods from different app revisions running at the same time means that certain endpoints that are circumventing Korifi abstractions and are looking directly into the app `Pod`s have to be careful to only lookup `Pod`s from the current app revision. At the moment of writing these are:

- `GET /v3/apps/{guid}/processes/{type}/stats` fetching pod [stats](https://github.com/cloudfoundry/korifi/blob/main/api/actions/process_stats.go#L92)
- `GET /api/v1/read/{guid}` fetching pod [logs](https://github.com/cloudfoundry/korifi/blob/main/api/repositories/pod_repository.go#L60)

Ideally we should introduce abstractions so that we don't rely on concrete runner implementations, but that is a topic for another ADR.

