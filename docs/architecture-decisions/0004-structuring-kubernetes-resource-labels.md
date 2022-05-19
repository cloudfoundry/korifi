# Structuring Kubernetes Resource Labels

Date: 2021-09-13

## Status

Accepted

## Context

Apart from serving as identifying attributes for objects, Labels enable us to map relationships onto system objects in a loosely coupled fashion.
i.e., it doesn't imply semantics to the underlying core system. As the CR controllers add and modify the labels,  we want to ensure the correct domain and label structure.
We would like to see the labels conform to [kubernetes label recommendations](https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/).

The kubernetes recommended labels are tailored more towards describing (name, version, instance, created-by...) the current object.
`app.kubernetes.io/part-of` is available to describe relationship with other objects, but it cannot be used in its raw form.
We have CRs that have more than one label describing its relationship with objects within the `*.cloudfoundry.org`.
Since a set of labels needs to be unique for an object, we will try to come up with our own set of well-defined labels.

Referring to the [syntax documentation about labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/), some options to consider:

- AppGUID:
    - korifi.cloudfoundry.org/CFAppGUID
    - korifi.cloudfoundry.org/cfappguid
    - korifi.cloudfoundry.org/cf-app-guid

## Decision

We have decided to use lower-case-kebabs for label-names and API Group as the prefix.

- AppGUID     : korifi.cloudfoundry.org/app-guid
- Build GUID  : korifi.cloudfoundry.org/build-guid
- Package GUID: korifi.cloudfoundry.org/package-guid
- Process GUID: korifi.cloudfoundry.org/process-guid
- Process Type: korifi.cloudfoundry.org/process-type

