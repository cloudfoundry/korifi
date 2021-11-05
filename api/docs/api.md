# CF K8s API Endpoint Documentation

**This document captures API endpoints currently supported by the shim**

## Resources

### Root

Docs: https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#root

| Resource | Endpoint |
|--|--|
| Global API Root | GET / |
| V3 API Root | GET /v3 |

### Resource Matches

Docs: https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#resource-matches

| Resource | Endpoint |
|--|--|
| Create a Resource Match | POST /v3/resource_matches |

#### [Create a Resource Match](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#create-a-resource-match)
```bash
curl "http://localhost:9000/v3/resource_matches" \
  -X POST \
  -d '{}'
```

### Apps

Docs: https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#apps

| Resource | Endpoint |
|--|--|
| List Apps | GET /v3/apps |
| Get App | GET /v3/apps/\<guid> |
| Create App | POST /v3/apps |
| Set App's Current Droplet | PATCH /v3/apps/\<guid>/relationships/current_droplet |
| Start App | POST /v3/apps/\<guid>/actions/start |
| Stop App | POST /v3/apps/\<guid>/actions/stop |
| List App Processes | GET /v3/apps/\<guid>/processes |
| Scale App Process | POST /v3/apps/<guid>/processes/<type>/actions/scale |
| List App Routes | GET /v3/apps/\<guid>/routes |

#### [Creating Apps](https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#the-app-object)
Note : `namespace` needs to exist before creating the app.
```bash
curl "http://localhost:9000/v3/apps" \
  -X POST \
  -d '{"name":"my-app","relationships":{"space":{"data":{"guid":"<namespace-name>"}}}}'
```

#### [Setting App's Current Droplet](https://v3-apidocs.cloudfoundry.org/version/3.108.0/index.html#update-a-droplet)
```bash
curl "http://localhost:9000/v3/apps/<app-guid>/relationships/current_droplet" \
  -X PATCH \
  -d '{"data":{"guid":"<droplet-guid>"}}'
```

#### [Scaling App's Process](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#scale-a-process)
```bash
curl "http://localhost:9000/v3/apps/<guid>/processes/<type>/actions/scale" \
  -X POST \
  -d '{ "instances": 5, "memory_in_mb": 256, "disk_in_mb": 1024 }'
```


#### [Start an app](https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#start-an-app)
```bash
curl "http://localhost:9000/v3/apps/<app-guid>/actions/start" \
  -X POST
```

#### [Stop an app](https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#stop-an-app)
```bash
curl "http://localhost:9000/v3/apps/<app-guid>/actions/stop" \
  -X POST
```

### Packages

| Resource | Endpoint |
|--|--|
| Create Package | POST /v3/packages |
| Upload Package Bits | POST /v3/packages/<guid>/upload |

#### [Creating Packages](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#create-a-package)
```bash
curl "http://localhost:9000/v3/packages" \
  -X POST \
  -d '{"type":"bits","relationships":{"app":{"data":{"guid":"<app-guid-goes-here>"}}}}'
```

#### [Uploading Package Bits](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#upload-package-bits)
```bash
curl "http://localhost:9000/v3/packages/<guid>/upload" \
  -X POST \
  -F bits=@"<path-to-app-source.zip>"
```
 
### Builds

Docs: https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#builds

| Resource | Endpoint |
|--|--|
| Get Build | GET /v3/builds/\<guid> |
| Create Build | POST /v3/builds|

#### [Creating Builds](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#create-a-build)
```bash
curl "http://localhost:9000/v3/builds" \
  -X POST \
  -d '{"package":{"guid":"<package-guid-goes-here>"}}'
```

### Droplet

Docs: https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#droplets

| Resource | Endpoint |
|--|--|
| Get Droplet | GET /v3/droplets/\<guid> |

### Process

Docs: https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#processes

| Resource | Endpoint |
|--|--|
| Get Process | GET /v3/processes/\<guid>/sidecars |
| Get Process Sidecars | GET /v3/processes/\<guid>/sidecars |
| Scale Process | POST /v3/processes/\<guid>/actions/scale |

#### [Scaling Processes](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#scale-a-process)
```bash
curl "http://localhost:9000/v3/processes/<guid>/actions/scale" \
  -X POST \
  -d '{ "instances": 5, "memory_in_mb": 256, "disk_in_mb": 1024 }'
```


### Routes

| Resource | Endpoint |
|--|--|
| Get Route | GET /v3/routes/\<guid> |
| Get Route List | GET /v3/routes |
| Get Route Destinations | GET /v3/routes/\<guid\>/destinations |
| Create Route | POST /v3/routes |
| Add Destinations to Route | POST /v3/routes/\<guid\>/destinations |

#### [Creating Routes](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#create-a-route)
```bash
curl "http://localhost:9000/v3/routes/<guid>/destinations" \
  -X POST \
  -d '{"host": "hostname","path": "/path","relationships": {"domain": {"data": { "guid": "<domain-guid-goes-here>" }},"space": {"data": { "guid": "<namespace-name>" }}}}'
```

#### [Add Destinations to Route](https://v3-apidocs.cloudfoundry.org/version/3.108.0/index.html#insert-destinations-for-a-route)
```bash
curl "http://localhost:9000/v3/routes" \
  -X POST \
  -d '{
        "destinations": [
          {
            "app": {
              "guid": "<app-guid-1>"
            }
          },
          {
            "app": {
              "guid": "<app-guid-2>",
              "process": {
                "type": "api"
              }
            },
            "port": 9000,
            "protocol": "http1"
          }
        ]
      }'
```

### Manifest

| Resource | Endpoint |
|--|--|
| Apply a manifest | POST /v3/spaces/\<space-guid>/actions/apply_manifest |

#### [Applying a manifest](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#create-a-route)
```bash
curl "http://localhost:9000/v3/spaces/<space-guid>/actions/apply_manifest" \
  -X POST \
  -H "Content-type: application/x-yaml" \
  --data-binary @<path-to-manifest.yml>
```

| Resource | Endpoint |
|--|--|
| Create a manifest diff for a space | POST /v3/spaces/\<space-guid>/manifest_diff |

#### [Create a manifest diff for a space](https://v3-apidocs.cloudfoundry.org/version/3.109.0/index.html#create-a-manifest-diff-for-a-space-experimental)
**WARNING:** Currently this endpoint always returns an empty diff.

##### Example Request
```bash
curl "http://localhost:9000/v3/spaces/<space-guid>/manifest_diff" \
  -X POST \
  -H "Content-type: application/x-yaml" \
  --data-binary @<path-to-manifest.yml>
```

##### Example Response
```json
HTTP/1.1 202 OK
Content-Type: application/json

{
  "diff": []
}
```
