#  Docker applications support

## Overview

Users can use the `cf` cli to push apps with docker images. Korifi then uses
the docker image to schedule workloads that are bcked up by the docker image.

In contrast to CF for VMs, Korifi supports pushing docker images by default and
the feature cannot be disabled.

To deploy a docker image from a container registry repository (such as
Dockerhub), run:

```
cf push APP-NAME --docker-image REPO/IMAGE:TAG
```

For more information and examples, please refer to the [CF
documentation](https://docs.cloudfoundry.org/devguide/deploy-apps/push-docker.html)

## Limitations

Korifi would reject docker images that are built to run their processes as the
privileged (`root`) user. Such workloads on Kubernetes would run as the
Kubernetes node `root` user and that is insecure.

Kubernetes [user namespace
support](https://kubernetes.io/docs/concepts/workloads/pods/user-namespaces/)
addresses that. However, this feature is only available in very recent
Kubernetes versions that are currently not provided by major Kubernetes
providers.

## Running docker images with non-standard ports

Cloud Foundry expects that all apps listen on port `8080` to handle HTTP
requests. Docker images may configure alternative ports via the `EXPOSE`
directive in the Dockerfile. Korifi respects this metadata and would configure
the default route acoordingly.

If the `EXPOSE` directive is not specified in the Dockerfile, Korifi would
default to port `8080` when creating the default route. When the `EXPOSE`
directive is missing and the app is listening on port other than `8080`, the
following procedure can be used to properly configure the app route:

```
APP_NAME=<your app name>
DOMAIN=<the domain name>
IMAGE=<repo/image:tag>
DESIRED_PORT=<app port>

cf push $APP_NAME --no-route --docker-image $IMAGE
cf create-route $DOMAIN --hostname $APP_NAME
route_guid=$(cf curl /v3/routes | jq -r ".resources[] | select(.host==\"$APP_NAME\").guid")
read -r -d '' destination <<EOF
{
   "destinations": [
   {
     "app": {
       "guid": "$(cf app $APP_NAME --guid)",
       "process": {
         "type": "web"
       }
     },
     "port": $DESIRED_PORT,
     "protocol": "http1"
   }
 ]
}
EOF
cf curl -XPOST "/v3/routes/$route_guid/destinations" -d "$destination"
```
