# syntax = docker/dockerfile:experimental
FROM golang:1.18.1 as builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY controllers/api controllers/api/
COPY controllers/webhooks controllers/webhooks/
COPY api/actions api/actions/
COPY api/handlers api/handlers/
COPY api/apierrors api/apierrors
COPY api/authorization api/authorization/
COPY api/payloads api/payloads/
COPY api/presenter api/presenter/
COPY api/repositories api/repositories/
COPY api/config/config.go api/config/
COPY api/main.go api/
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -gcflags=all="-N -l" -o cfapi api/main.go

# Get Delve from a GOPATH not from a Go Modules project
WORKDIR /go/src/
RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM ubuntu

WORKDIR /
COPY --from=builder /workspace/cfapi .
COPY --from=builder /go/bin/dlv .
USER 1000:1000

ENTRYPOINT [ "/cfapi" ]
