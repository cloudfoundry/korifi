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
  - The `.status.binding` secret: The service binding controller derives the secret to put in the `.status.binding` duck type of the `CFServiceBinding` from the `.status.credentials` field of the `CFServiceInstance`. The binding secret is in the [format](https://servicebinding.io/spec/core/1.0.0/#example-secret) specified by servicebinding.io and is named after the service binding. According to that spec the secret has type `servicebinding.io/<type>` and its data is a flat collection of key value pairs. For example:

  ```
  apiVersion: v1
  kind: Secret
  type: servicebinding.io/database
  data:
    type: database
    user: '{"name":"alice", "password": "bob"}'
  ```
  This secret is later on projected in the app workload by servicebinding.io


  - The `.status.credentials` secret: The service binding controller sets's the `.status.credentials` to refer to the secred referred to by the `.status.credentials` of the `CFServiceInstance`. This secret is later on used to calculate the value of the `VCAP_SERVICES` env variable.

### Backwards Compatibility
The servicebinding.io contoller monitors the value of the `.status.binding` duck type of the `CFServiceBinding` and updates the spec of the workload statefulset to reflect any changes. The statefulset controller reacts to that change and rolls the statefulset out, causing an app restart. In order to avoid undesired restarting of apps during Korifi upgrade we make sure not to modify the value of the duck type of any pre-existing `CFServiceBinding`s.
- After a Korifi upgrade the servicebindings still reference the legacy credentials secret from the `CFServiceInstance` spec.
- The service instance controller creates a migrated secret in the new format and sets it in the `.status.credentials`.
- As long as service instance credentials do not change, the value of the `CFServiceBinding` duck type does not change as well.
- When the credentials of the legacy service instance are updated:
  - The instance repo sets the `CFServiceInstance.Spec.SecretName` to the value from `CFServiceInstance.Status.Credentials`
  - The instance repo applies the updated credentials to the credentials secret
  - The service binding controller gets triggered by the `CFServiceInstance` update, derives the new binding secret and sets it in the `CFServiceBinding` duck type. This causes an app restart.
  - From this moment on the original credentials secret (the one bearing the service instance name) becomes unused. It will eventually get deleted by owner reference once the service instance gets deleted.

