# Use Contour for Ingress

Date: 2021-10-11

## Status

Accepted

## Context

Ultimately, we want to use the new Gateway API for both API and workload ingress. When we started our implementation based on the Routing/Domain Proposal document, we found a blocker on the latest tag of the Gateway API:

> API version: v1alpha2
>
> The working group expects that this release candidate is quite close to the final v1alpha2 API. However, breaking API changes are still possible.
>
> This release candidate is suitable for implementors, but the working group does not recommend shipping products based on a release candidate API due to the possibility of incompatible changes prior to the final release.

In terms of existing compatibility, Contour is the furthest along towards supporting the latest version of the Gateway API. Similarly, the overlap between Contour and Gateway API maintainers suggests that this trend will continue.

While Istio and Knative Ingress have plans to integrate against the Gateway API, their current interfaces are more widely diverse, which means that it would require more effort to migrate away from them in future.

## Decision

Given the disclaimer, we decided not to directly integrate against the Gateway API until the interface stabilizes. Instead, we identified Contour as what we think is the closest available alternative. We will reconcile CFRoutes to native Contour HTTPProxy resources and defer the use of any Gateway resources.

## Consequences

* We will have to circle back once the Gateway API stabilizes to migrate from Contour. We expect compatibility for the most part and some amount of migration work with help from documentation from Contour.
* Until the Gateway API stabilizes and we migrate the controller implementation, we will not have support for other ingress providers (Istio, Knative).

## Links
* [Routing/Domain Proposal](https://docs.google.com/document/d/1WUiFwMgKkZrHHbqL1sMuHdqempzfIiYXpnq6qoazVKc/edit#heading=h.x4x8m3y0db1o)
* [Latest Gateway API Tag](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.4.0-rc1)
