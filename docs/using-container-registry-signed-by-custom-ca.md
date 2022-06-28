# Use container registry signed by custom CAs
Korifi can be configured to use a custom registry that has a self-signed certicate.
Korifi accepts the `REGISTRY_CA_FILE` environment variable to point to a mounted secret containing the registry's CA cert.

## Configure korifi-api

- Create a secret containing the registry's CA cert in the `korifi-api-system` namespace. (see instructions)
    For example: `kubectl -n korifi-api-system create secret generic registry-ca-cert --from-file=cacerts.pem=./ca-cert.pem`
- Update [deployment definition](https://github.com/cloudfoundry/korifi/blob/main/api/config/base/deployment.yaml) to mount the secret (see [instructions](https://kubernetes.io/docs/concepts/configuration/secret/#using-secrets-as-files-from-a-pod))
- Set `REGISTRY_CA_FILE` environment variable on the deployment to point to the path of the mounted ca.crt file.

## Configure kpack-image-builder
Use the same instructions as above, but create secret in the `korifi-kpack-build-system` namespace and use this [deployment definition](https://github.com/cloudfoundry/korifi/blob/main/kpack-image-builder/config/manager/manager.yaml) 

## Kpack
Kpack does not support self-signed/internal CA configuration out of the box [see: pivotal/kpack#207](https://github.com/pivotal/kpack/issues/207) and providing support for that is not something in scope for us to take on. Operators can modify the Kpack deployment, using something like the [cert-injection-webhook](https://github.com/vmware-tanzu/cert-injection-webhook) on the kpack Pods, or bring their own build reconciler in these cases.