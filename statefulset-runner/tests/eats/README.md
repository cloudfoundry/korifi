# Eirini Acceptance Tests (EATs)

## How to run tests against various environments

### Local Kind

Firstly, target kind: `kind export kubeconfig --name ENV-NAME`

#### Using the Eirini deployment files (aka Helmless)

```
$EIRINI_RELEASE_DIR/deploy/scripts/cleanup.sh
$EIRINI_RELEASE_DIR/deploy/scripts/deploy.sh

EIRINI_ADDRESS=https://$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[0].address}') \
EIRINI_SYSTEM_NS=eirini-controller \
$EIRINI_DIR/scripts/run_eats_tests.sh
```

### cf-for-k8s

1. Target an environment
1. Patch-me-if-you-can for the modified components
1. Remove the eirini network policy
1. Make the local EATs Fixture test code ignore certificate validation by setting InsecureSkipVerify: true in the TLSConfig
1. Add an Istio virtual service (substituting ENV-NAME as appropriate):
   ```
   apiVersion: networking.istio.io/v1beta1
   kind: VirtualService
   metadata:
     name: eirini-external
     namespace: cf-system
   spec:
     gateways:
     - cf-system/istio-ingressgateway
     hosts:
     - eirini.ENV-NAME.ci-envs.eirini.cf-app.com
     http:
     - route:
       - destination:
           host: eirini.cf-system.svc.cluster.local
           port:
             number: 8080
   ```
1. ```
   EIRINI_ADDRESS=https://eirini.ENV-NAME.ci-envs.eirini.cf-app.com \
   EIRINI_SYSTEM_NS=cf-system \
   ./scripts/run_eats_tests.sh
   ```

Expect a failure due to name of the configmap in cf-for-k8s

## Getting Eirini K8S logs on failure

### Why do you need them

- Standard EATs failure logs only contain standard Ginkgo output, i.e. no K8S logs
- K8S logs might turn to be quite useful when troubleshooting a failure as they might tell you what has been going on on the cluster at the time the test failed

### How to get them

Make sure to list the names of needed Eirini components logs in a `needs-logs-for` "section" in the top-level `Describe` name (for example `Describe("Tasks Reporter [needs-logs-for: eirini-api, eirini-task-reporter]", func(){...})`). Upon test failure EATs infrastructure would collect the logs for the requested Eriini components and print them to the `GinkgoWriter`
