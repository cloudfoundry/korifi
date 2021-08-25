# 2. Set Convention For Controller Client Interfaces

Date: 2021-08-25

## Status

Accepted

## Context

While writing the inital implementation for Kubernetes CR Controllers, we were uncertain how to separate Reconcilers from their clients for the purpose of unit testing.

This led us to experiement with two approaches to mock a Kubernetes Client behind interfaces of varying complexity.

### Option 1: 
Use a wrapper interface to flatten interaction with the Kubernetes Client to a single interface

  
```Go
// For each custom resource, you have to define an interface that defines the precise interaction with the Kubernetes client
type CFAppClient interface {
	Get(ctx context.Context, name types.NamespacedName) (*workloadsv1alpha1.CFApp, error)
	UpdateStatus(ctx context.Context, cfApp *workloadsv1alpha1.CFApp) error
}

// CFAppK8sClient implements the CFAppClient interface with a Kubernetes Client
type CFAppK8sClient struct {
	Client client.Client
}

func (c *CFAppK8sClient) Get(ctx context.Context, name types.NamespacedName) (*workloadsv1alpha1.CFApp, error) {
	var cfApp workloadsv1alpha1.CFApp
	if err := c.Client.Get(ctx, name, &cfApp); err != nil {
		return nil, err
	}
	return &cfApp, nil
}

// UpdateStatus flattens the interaction with the Kubernetes Client, so we don't interact with the client.StatusWriter directly
func (c *CFAppK8sClient) UpdateStatus(ctx context.Context, cfApp *workloadsv1alpha1.CFApp) error {
	return c.Client.Status().Update(ctx, cfApp)
}
```
 
#### Pros:
* Only requires a single mock client, which reduces complexity of testing setup
  
</br>

### Option 2:
Use a transparent interface that matches Kubernetes Client exactly

```Go
type CFAppClient interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error
	Status() client.StatusWriter // Notice that this function returns an instance of the existing Kubernetes client.StatusWriter interface
    // This requires your unit testing to also Mock out client.StatusWriter in addition to CFAppClient
}

// We don't create an intermediate struct with Get() and Status() because the Kuberentes Client will conform to this interface directly
```

#### Pros:
* Kubernetes Client implements the client interface, which eliminates the need for an intermediate client wrapper
* Interaction with the Client is explicit and follows established conventions in the Kubernetes community
  
</br>

Comparing the two, we found them to be very similar. However, we decided to priorize reducing the barrier to entry for contributions. This comes at the cost of additional complexity in mocking out intermediate interfaces like `client.StatusWriter` when testing. However, the added testing overhead is comparable to expectations from other languages, and therefore not something that we felt the need to optimize.

</br>

---
</br>

## Decision

The cf-k8s-controllers Controllers will be implemented with a transparent interface that define the subset of Kubernetes Client functions the specific Controller Client will use.

## Consequences

* We will be able to use Kubernetes Clients in our Controller code transparently, which we hope will be easier for outside contributors to understand.
* We will have to mock out additional interfaces like the `client.StatusWriter` when writing unit tests. This can be partially mitigated for future Controllers by writing a consolidated mock-setup package.