# 9. Implement CF Restart in CF Workloads Controllers

Date: 2021-12-10

## Status

Accepted

## Context

As part of the initial `v0.1` release, we implemented `cf restart` via the `v3/apps/<app-guid>/actions/restart` endpoint in the api shim: [implementation reference](https://github.com/cloudfoundry/cf-k8s-controllers/blob/8154b97397a8f46bd4c6150b22ea8cf34654a426/api/apis/app_handler.go#L613-L694). 

However, we discovered that the `cf cli` actually uses a combination of `actions/stop` and `actions/start` to execute a restart. Due to how the CF Process Controller created Eirini Long Running Processes(LRPs), this quick shut off and power on did not properly roll the underlying pods due to Erini setting its pod selectors and names deterministically.

As part of issue [#260](https://github.com/cloudfoundry/cf-k8s-controllers/issues/260), we searched for a solution to support the cf cli’s `cf restart` but still followed the established conventions of Kubernetes.

## Decision

We will take inspiration from the Kubernetes Deployment Update implementation to introduce a “revision” to the CFApp Custom Resource in the form of an annotation. From there, the CF Process Controller will create LRPs based on the combination of it’s Process GUID and the App’s current revision. This will ensure that the LRPs for each start and stop cycle are unique so long as the CF App revision is incremented for each “stop” operation.

### Pros:
- Maintains the conventional boundaries of the `metadata`, `spec`, and `status` fields. Users only interact with the `metadata` and `spec`, while the controllers exclusively manage the `status` fields
  - The `desiredState` always accurately represents the desired state of the app, even during a restart. We don’t introduce an ephemeral `restarting` state, and instead use the metadata “revision” annotation to trigger restarts
- Centralizes the control logic underneath the CF Process Controller
  - Multiple restarts of the same App do not require special logic or spin up independent threads
  - Eliminates the need to poll or watch pods, since CF Process Controller is directly responsible for creating and deleting LRPs
- Works with existing implementations of API Shim `actions/start` and `actions/stop` without any modifications

### Cons:
- `cf restart` is dependent on the CF App Mutating Webhook in order to automatically bump the “revision” annotation of the CF App. Without it, the CF App pods will always have deterministic names and selectors and not restart
- The CF Process Controller becomes more complex. Now it needs to keep track of multiple LRPs, from current and previous revisions, as well as include logic to filter and clean up orphaned LRPs
- The API Shim `process/stats` endpoint needs to be updated to account for multiple potential revisions

## Diagram
We have summarized the design in the following [diagram](https://miro.com/app/board/o9J_lFiI8CU=/?moveToWidget=3458764514898457927&cot=14)


## Consequences
This design requires multiple changes to CF Controller components, detailed below:

#### CF App CRD
- Introduces a new `ObservedDesiredState` field under CFApp status. This is used by the Mutating Webhook to notice when a CFApp goes from “STARTED” to “STOPPED” state.

#### CF App Controller
- On CF App reconciliation, set the Status.ObservedDesiredState to Spec.DesiredState.

#### CF App Mutating Webhook
- Sets the `app-rev` annotation if it’s missing to a default value of “0”
- Increments the `app-rev` annotation value if the Spec.DesiredState is `STOPPED` and the status.ObservedDesiredState is `STARTED`

#### CF Process Controller
- Sets `LRP.Name` to be a combination of ProcessGUID and `app-rev`(hashed)
- Sets `LRP.Version` to equal `app-rev`
- Creates or Patches LRPs based on the LRPs current state
- Cleans up any LRPs
  - That don't match the current `app-rev`
  - If the DesiredState of the CFApp is stopped.

#### CF API Shim
- The API Shim `process/stats` endpoint needs to be updated to account for multiple potential revisions





