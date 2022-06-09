# Integration Tests

This suite is testing Eirini with a real K8s cluster. For that, it expects the path to a valid kubeconfig file as `INTEGRATION_KUBECONFIG` environment variable. If `INTEGRATION_KUBECONFIG` is not provided, this will default to `~/.kube/config`.
