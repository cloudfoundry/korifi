# syntax = docker/dockerfile:experimental
FROM golang:1.23 as builder

ARG version=dev

WORKDIR /workspace

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY api api
COPY model model
COPY controllers/api controllers/api
COPY controllers/controllers/shared controllers/controllers/shared
COPY controllers/controllers/workloads controllers/controllers/workloads
COPY controllers/controllers/services/credentials controllers/controllers/services/credentials
COPY controllers/webhooks controllers/webhooks
COPY tools tools
COPY version version

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -ldflags "-X code.cloudfoundry.org/korifi/version.Version=${version}" -o cfapi api/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/cfapi .
USER 1000:1000

ENTRYPOINT [ "/cfapi" ]
