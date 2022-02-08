# Not found and forbidden errors returned from the repositories package

Date: 2022-02-07

## Status

Review

## Context

There has been a proliferation of error types that are returned from
repositories methods, namely `NotFoundError`, `ResourceNotFoundError`,
`ForbiddenError` and `PermissionDeniedOrNotFoundError`. The first two are
clearly duplicates, and the last is intended to return either a not found error
or a forbidden error.

The code contains a mixture of NotFound errors and PermissionDeniedOrNotFound
errors, and more Forbidden errors have been appearing recently. It is not
possible to know whether a repositories method would return a NotFound or a
PermissionDeniedOrNotFound error when a resource is not found. Or equally
whether it would return a Forbidden or a PermissionDeniedOrNotFound error when
a user's role does not grant access to the resource.

The consequence is that mistakes have been made in the handlers which call the
repo methods. For example, there are plenty of unit tests faking
PermissionDeniedOrNotFoundError responses, where the repo code will only ever
return a NotFoundError. The go language does not provide any type help with
returned errors, as they all implement the error interface, so we need to make
it as easy as possible to get error checking correct.

It should also be known that the CF API converts forbidden errors to not found
errors in certain circumstances to prevent information leakage to unauthorized
users. Generally, when perform a get of a single resource, a forbidden error
should be returned as a 404 not found error. When listing resources, forbidden
resources should be excluded from the list without error. And when modifying
resources, unauthorized gets while fetching related objects should return a 404
response, whereas an unauthorized update on a resource should return a 403
response.

## Decision

We should use only the NotFoundError and ForbiddenError types.

PermissionDeniedOrNotFoundError and ResourceNotFoundError are to be deleted.

Other errors encountered in the repositories package, not intended to be
specifically handled, can be returned as wrapped errors using
`fmt.Errorf("<detail>: %w", err)`. We would expect these to trigger a 500
error response.

Handlers are responsible for determining the end user error based on the errors
returned from the repositories package.

## Consequences

1. Errors returned from the repositories package when resources are not found
   or forbidden are predictable.
1. Unit tests are less likely to fake the wrong errors
1. Detailed information about the intent of the error is retained until the
   point at which it is converted to a user error.
1. k8s errors, although wrapped, are kept inside the repositories packages
1. No need to check wrapped errors inside the apis package
