# Valid UUIDs for CF Resource GUIDs

Date: 2024-11-21

## Status

Accepted

## Context

We have received feedback from CF API users (such as the [MultiApps Controller](https://github.com/cloudfoundry/multiapps-controller)) that [using prefixes in resource GUIDS](./0012-resource-name-prefixes.md) violates the [CF API specification](https://v3-apidocs.cloudfoundry.org/version/3.181.0/index.html#api-resource) and such GUIDs cannot be stored in database columns that are configured with the `UUID` type. We have introduced those prefixes in the past as a request to improve operators' ergonomics. However, ergonomics cannot justify violating the CF API specification.

## Decision
We are going to use valid UUIDs for Korifi k8s resource names when new resources are being created. Existing resources should not be affected by this change.

* [V4 UUIDs](https://www.rfc-editor.org/rfc/rfc9562.html#name-uuid-version-4) are going to be used by default
* Whereever we want to leverage optimistic name locking (such as ensure one process resource per app per process type) we use [v5 UUIDs](https://www.rfc-editor.org/rfc/rfc9562.html#name-uuid-version-5), where the "parent resource" guid is used as namespace.

## Consequences

### Pros
* CF API specification compliance

### Cons
* Reduced operators' ergonomics. One could implement a custom tool to map CFSpace to k8s namespaces if such a mapping is required.

