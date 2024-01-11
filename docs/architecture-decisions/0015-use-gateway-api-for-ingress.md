# Use Gateway API for Ingress

Date: 2024-01-10

## Status

Accepted

## Context

For Korifi routing we would like to provide users with options when deciding what networker implementation to use. Previously this could be achieved by turning off the default Korifi contour router so that users can implement their own controllers that reconciles `CFRoute`s to whatever netwoker they use. This is however quite an involving process that Kubernetes addresses by introducing the [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/). The API has now reached 1.0 and is adequately supported by both Contour and Istio.

## Decision

As the Gateway API is mature enough, we now leverage it for Korifi routing.

## Links
* [Replaceable Networker Proposal](https://docs.google.com/document/d/117buRig9YUm4z2njK5gdnqxBldn5fTZNCM_QTUy28lw/edit?usp=sharing)
* [Latest Gateway API Tag](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v1.0.0)
