# List results filtering, sorting and pagination

Date: 2025-07-10

## Status

Accepted

## Context

The CF API provides list endpoints that support filtering, sorting, and pagination. Since these result sets can be large, Korifi should handle these operations efficiently by:
* Utilizing Kubernetes’ native features where possible
* Keeping list responses reasonably sized to minimize network usage
* Applying sensible defaults when the `per_page` query parameter is not set

## Decision

Korifi uses [label selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/) for filtering, following Kubernetes best practices.

Sorting and ordering are not natively supported by the Kubernetes API, so Korifi implements these features in its API layer, specifically in the [resource repositories](https://github.com/cloudfoundry/korifi/tree/522149887bbc3be17db437b4142d6d9ff4da71b1/api/repositories).

Filtering, sorting, and pagination are applied sequentially: first filtering, then sorting, and finally pagination.

### Filtering

To enable efficient filtering, Korifi resources are labeled with values that the API allows filtering by. These labels are added automatically and synchronously via webhooks.

There are two types of labels:

#### Common Labels

These are applied to all Korifi resources by the [common labels webhook](https://github.com/cloudfoundry/korifi/blob/522149887bbc3be17db437b4142d6d9ff4da71b1/controllers/webhooks/common_labels/webhook.go):
* `korifi.cloudfoundry.org/guid` – resource GUID
* `korifi.cloudfoundry.org/created_at` – creation timestamp ([RFC3339](https://datatracker.ietf.org/doc/html/rfc3339) format)
* `korifi.cloudfoundry.org/updated_at` – last update timestamp ([RFC3339](https://datatracker.ietf.org/doc/html/rfc3339) format)

#### Resource-Specific Labels

Set by the [`label indexer`](https://github.com/cloudfoundry/korifi/blob/522149887bbc3be17db437b4142d6d9ff4da71b1/controllers/webhooks/label_indexer/webhook.go) webhook, these labels are unique to each resource type.

For example, the `CFProcess` resource specific labels are:
* `korifi.cloudfoundry.org/space-guid` – the space GUID
* `korifi.cloudfoundry.org/app-guid` – the application GUID
* `korifi.cloudfoundry.org/process-type` – the process type (e.g., `web`, `worker`)

### Sorting

Korifi retrieves resources as [tables](https://kubernetes.io/docs/reference/using-api/api-concepts/#receiving-resources-as-tables), which provide a minimal representation. The `Name` column always contains the custom resource name (the object GUID).

Korifi resources custom [printer columns](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#additional-printer-columns) expose fields that can be used for sorting.

The API repository sorts the table by the desired column and compiles an ordered list of object GUIDs.

### Paging

After sorting, Korifi paginates the ordered list of GUIDs based on the `page` and `per_page` parameters. If `per_page` is not specified, it defaults to the [`api.list.defaultPageSize`](https://github.com/cloudfoundry/korifi/blob/1cc08d3fac3fc80c15263b29e584239fd6a59eb3/helm/korifi/values.yaml#L74) Helm value.

### Resolving GUIDs to Objects

Korifi then resolves the paginated ordered GUIDs to actual resources by making a second Kubernetes list request using the label selector `korifi.cloudfoundry.org/guid in (<paginated-order-guids>)`. Since Kubernetes does not guarantee order, Korifi reorders the results to match the sorted GUIDs order.

### The Repositories `Klient`

Korifi repositories use a custom [Kubernetes client](https://github.com/cloudfoundry/korifi/blob/main/api/repositories/k8sklient/klient.go) that mimics the controller-runtime [`Client` interface](https://github.com/kubernetes-sigs/controller-runtime/blob/252af6420feba970d0020ba4c029a0a1ce3f9688/pkg/client/interfaces.go#L164). Its [`List` method](https://github.com/cloudfoundry/korifi/blob/1cc08d3fac3fc80c15263b29e584239fd6a59eb3/api/repositories/k8sklient/klient.go#L109) accepts convenient [list options](https://github.com/cloudfoundry/korifi/blob/1cc08d3fac3fc80c15263b29e584239fd6a59eb3/api/repositories/list_options.go#L55) and returns both results and pagination metadata. The results are then [presented](https://github.com/cloudfoundry/korifi/blob/1cc08d3fac3fc80c15263b29e584239fd6a59eb3/api/presenter/shared.go#L87) to users in the [CF API specification format](https://v3-apidocs.cloudfoundry.org/version/3.197.0/index.html#pagination).

