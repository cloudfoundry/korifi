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

#### [Start an app](https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#start-an-app)
```bash
curl "http://localhost:9000/v3/apps/<app-guid>/actions/start" \
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
| Get Process Sidecars | GET /v3/processes/\<guid>/sidecars |




### Routes

| Resource | Endpoint |
|--|--|
| Get Route | GET /v3/routes/\<guid> |
| Create Route | POST /v3/routes |

#### [Creating Routes](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#create-a-route)
```bash
curl "http://localhost:9000/v3/routes" \
  -X POST \
  -d '{"host": "hostname","path": "/path","relationships": {"domain": {"data": { "guid": "<domain-guid-goes-here>" }},"space": {"data": { "guid": "<namespace-name>" }}}}'
```
