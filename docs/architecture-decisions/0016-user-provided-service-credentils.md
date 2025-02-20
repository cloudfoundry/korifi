# User Provided Service Credentials

Date: 2024-02-23

## Status

Accepted

## Context
According to the CF API, when creating or patching user provided services one can optionally specify a [credentials](https://v3-apidocs.cloudfoundry.org/version/3.159.0/index.html#optional-parameters-for-user-provided-service-instance) json object parameter. Up to Korifi `v0.11.0`, the credentials parameter was unintentionally limited to flat collection of key value pairs. In this ADR we discribe the mechanism of supporting json object credentials in a backwards compatible manner.

What is meant by backwards compatible manner:
- Existing apps that are already bound to existing service instances continue to see the same service credentials
- Existing apps that are already bound to existing service instances do not get restarted during Korifi upgrade
- All apps continue to see servicebinding.io credentials as flat collection of key value pairs

## Decision
* Store CF user provided service instance credentials in a dedicated secret named after the service instance with a single `credentials` key.
* Derive the servicebinding.io secret from the service instance secret, converting the credentials to key value pairs.
* Keep the [duck type](https://servicebinding.io/spec/core/1.0.0/#terminology-definition) of pre-existing service bindings unchanged to avoid undesired app restarts when Korifi is upgraded

### Service Instance
We introduce a new `Credentials` field to `CFServiceInstance` status, where the service instance controller published the service instance credentials secret. Usually this is the secret from the `CFServiceInstance` spec.

The service instance credentials secret is an `Opaque` secret. Its data has a single `credentials` key with the marshalled credentials object as its value. For example:

```
apiVersion: v1
kind: Secret
type: Opaque
data:
  credentials: '{"type":"database","user":{"name":"alice", "password": "bob"}}'
```

### Service Binding
The service binding credentials need to be stored in two different formats at the same time:
- As bytes on the filesystem (i.e. in marshalled form) as specified by the servicebinding.io [spec](https://servicebinding.io/spec/core/1.0.0/#workload-projection)
- As a json object  (i.e. in unmarhalled form) as specified by the cf [docs](https://docs.cloudfoundry.org/devguide/deploy-apps/environment-variable.html#VCAP-SERVICES)

In order to support both specs the service binding controller holds references to two secrets, one for each format representation:
  - The `.status.mountSecretRef` secret: The service binding controller derives the secret to project into the workload from the `.status.credentials` field of the `CFServiceInstance`. The binding secret is in the [format](https://servicebinding.io/spec/core/1.0.0/#example-secret) specified by servicebinding.io and is named after the service binding. According to that spec the secret has type `servicebinding.io/<type>` and its data is a flat collection of key value pairs. For example:

  ```
  apiVersion: v1
  kind: Secret
  type: servicebinding.io/database
  data:
    type: database
    user: '{"name":"alice", "password": "bob"}'
  ```
  This secret is later on projected in the app workload by the app workload runner


  - The `.status.envSecretRef` secret: The service binding controller sets's the `.status.envSecretRef` to refer to the secret referred to by the `.status.credentials` of the `CFServiceInstance`. This secret is later on used to calculate the value of the `VCAP_SERVICES` env variable.
