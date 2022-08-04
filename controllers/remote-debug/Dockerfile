# syntax = docker/dockerfile:experimental
FROM golang:1.19.0 as builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY api/repositories api/repositories/
COPY api/apierrors api/apierrors/
COPY api/authorization api/authorization/
COPY api/config api/config/
COPY api/correlation api/correlation/
COPY controllers/api controllers/api/
COPY controllers/controllers controllers/controllers/
COPY controllers/coordination controllers/coordination/
COPY controllers/webhooks controllers/webhooks/
COPY controllers/config/config.go controllers/config/
COPY controllers/main.go controllers/
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -gcflags=all="-N -l" -o manager controllers/main.go

# Get Delve from a GOPATH not from a Go Modules project
WORKDIR /go/src/
RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM ubuntu
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /go/bin/dlv .

EXPOSE 8080 8081 9443 40000

CMD ["/dlv", "--listen=:40000", "--headless=true", "--api-version=2", "exec", "/manager", '--continue', "--accept-multiclient"]
