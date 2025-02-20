# Service credentials projection

Date: 2025-02-20


## Status

Accepted

## Context
We have been using servicebinding.io [`ServiceBinding`](https://servicebinding.io/spec/core/1.1.0/#service-binding) to project services credentials into the app and build workloads. While this has the convenience of doing the projection for us, Korifi was not in control when to project the secret onto the workload statefulset. This lead to application restarts every time an app is bound to a service as the statefulset changes immediately causing app pod restart.

Furthermore, the `ServiceBinding` definition exlicitly used to reference the workload `StatefulSet` which would not work should the workload runner be replaced by one that is running e.g. `Deployment`s

## Decision
* Do not create servicebinding.io `ServiceBinding` anymore
* Introduce `CFApp.Status.ServiceBindings` to contain references to the service bindings secret (`CFServiceBinding.Status.MountSecretRef`)
* When the `CFProcess` controller reconciles to `AppWorkload`, it populates `AppWorkload.Spec.Services` with the references above. Note that this takes place only when the app workload is created. Binding the app to more services would not change `AppWorkload.Spec.Services`. This technique prevents from app pods being restarted on every `bind` operation.
* In order for new bindings to become effective into the app, the app has to be restaged/restarted (i.e. the `AppWorkload` is recreated with the new bindings)
* The app workload runner is responsible to project all the services credentials secrets onto the workload as specified by [servicebinding.io projection](https://servicebinding.io/spec/core/1.1.0/#workload-projection). In the case of the default statefulset runner:
  - it configures the `SERVICE_BINDING_ROOT` environment variable on the statefulset. It's value is set to `/bindings`
  - creates a `mount` for every credentials secret under `/bindings/<credentials-secret-name>` in the statefulset pod template

