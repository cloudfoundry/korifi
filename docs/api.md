# CF K8s API Endpoint Documentation

** This document captures api endpoints currently supported by the shim**

## Resources

### Root

Docs: https://v3-apidocs.cloudfoundry.org/version/3.102.0/index.html#root

| Resource | Endpoint |
|--|--|
| Global API Root | GET / |
| V3 API Root | GET /v3 |


### Apps

Docs: https://v3-apidocs.cloudfoundry.org/version/3.102.0/index.html#apps

| Resource | Endpoint |
|--|--|
| Get App | GET /v3/apps/<guid> |
| Create App | POST /v3/apps 

#### [Creating Apps](https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#the-app-object)
Note : `namespace` needs to exist before creating the app.  
```bash
curl "http://localhost:9000/v3/apps" \
  -X POST \
  -d '{"name":"my-app","relationships":{"space":{"data":{"guid":"<namespace-name>"}}}}'
```

### Packages

| Resource | Endpoint |
|--|--|
| Create Package | POST /v3/packages |

#### [Creating Packages](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#create-a-package)
```bash
curl "http://localhost:9000/v3/packages" \
  -X POST \
  -d '{"type":"bits","relationships":{"app":{"data":{"guid":"<app-guid-goes-here>"}}}}'
```