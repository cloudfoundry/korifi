# Korifi resource ownership

Date: 2022-11-10

## Status

Accepted

## Context

Korifi defines a network of kubernetes resources in order to model the CF api
on top of kubernetes. Sometimes one resource that we are going to call the
parent is the logical owner of another resource which we are going to call the
child. The reconciler of the parent object is responsible of cleaning up all
child resources associated with the parent resource that is being deleted. This
cleanup is usually impemented by setting an [owner
ref](https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/)
on the child.

There are generally two options for who to set these owner refs:

### The child creator

This approach follows the good practice of the creator being responsible for
the deletion of the objects it creates. In this case deletion is determined by
the owner reference and the implied cascading delete. However there are some
drawbacks:

-   In order to set owner reference in kubernetes you have to first lookup the
    parent object. This violates the declarative nature of the kubernetes
    client as it cannot just apply a collection of resources at once. This does
    not apply when the collection of objects being created is self contained,
    i.e there are no references to previously created objects.
-   There is a potential race when the creator client cache is stale and does not
    know about the parent yet. This race can be mitigated by a retry loop, but
    this introduces unnecessary complexity in the client.

### The child reconciler

This approach mitigates the drawbacks of the child creator approach as
reconciliation runs in a loop anyway.

This approach however is only applicable when the child object already holds
some reference to its parent, e.g. `LocalObjectReference`. For example the
`CFApp` owns a secret that describes its environment. The buitin kubernetes
secret kind has no way of referring to and app, so no reconciler would be able
to set this owner reference.

A drawback of this approach is tha owner references are being set implicitly
without the knowledge of potential kubectl users and may come as a surprise to
them. We believe that korifi resources are not general purpose ones, so this
automation is unlikely to get in the way. Also korifi child resources such as
`CFServiceBinding` do not make much sense without being owned by their parent.

## Decision

We favour setting owner references in the child's reconciliation loop unless
that is impossible. In this case the owner ref should be set by the creator. An
example of this is the service instance creation, where a secret containing the
service instance sensitive data is created along with the service instance
itself and an owner ref is set by the creator.
