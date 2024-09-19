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
    CGO_ENABLED=0 GOOS=linux go build -ldflags "-X code.cloudfoundry.org/korifi/version.Version=${version}" -gcflags=all="-N -l" -o cfapi api/main.go

# Get Delve from a GOPATH not from a Go Modules project
WORKDIR /go/src/
RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM ubuntu

WORKDIR /
COPY --from=builder /workspace/cfapi .
COPY --from=builder /go/bin/dlv .
USER 1000:1000

CMD ["/dlv", "--listen=:40000", "--headless=true", "--api-version=2", "exec", "/cfapi", "--continue", "--accept-multiclient"]
