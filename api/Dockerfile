# syntax = docker/dockerfile:experimental
FROM golang:1.19.1 as builder

WORKDIR /workspace

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY api/actions api/actions
COPY api/apierrors api/apierrors
COPY api/authorization api/authorization
COPY api/config/config.go api/config/config.go
COPY api/correlation api/correlation
COPY api/handlers api/handlers
COPY api/main.go api/main.go
COPY api/payloads api/payloads
COPY api/presenter api/presenter
COPY api/repositories api/repositories
COPY controllers/api controllers/api
COPY controllers/config controllers/config
COPY controllers/controllers/shared controllers/controllers/shared
COPY controllers/controllers/workloads controllers/controllers/workloads
COPY controllers/webhooks controllers/webhooks
COPY tools tools

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cfapi api/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/cfapi .
USER 1000:1000

ENTRYPOINT [ "/cfapi" ]
