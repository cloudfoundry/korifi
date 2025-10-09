# Korifi Helm Chart
Korifi is a kubernetes native implementation of the Cloud Foundry V3 API. For more information check out the [project on github](https://github.com/cloudfoundry/korifi)

# Quick Start

If you want to try out Korifi on your local machine we recommend [installing korifi on kind](https://github.com/cloudfoundry/korifi/blob/main/INSTALL.kind.md)

# Installation

1. Make sure [dependencies](https://github.com/cloudfoundry/korifi/blob/main/INSTALL.md#dependencies) are installed


2. Add the korifi repository:

```
helm repo add korifi https://cloudfoundry.github.io/korifi/
```

3. Install the korifi helm chart

```
helm install my-korifi korifi/korifi
    --create-namespace \
    --namespace=korifi \
    --set=generateIngressCertificates=true \
    --set=adminUserName=cf-admin \
    --set=api.apiServer.url="api.korifi.example.org" \
    --set=defaultAppDomainName="apps.korifi.example.org" \
    --set=containerRepositoryPrefix=index.docker.io/my-organization/ \
    --set=kpackImageBuilder.builderRepository=index.docker.io/my-organization/kpack-builder \
    --set=networking.gatewayClass=contour \
    --wait
```

Refer to the [installation guide](https://github.com/cloudfoundry/korifi/blob/main/INSTALL.md) for more information on how to configure the helm values
