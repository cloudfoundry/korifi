# syntax = docker/dockerfile:experimental
FROM golang:1.19.1 as builder

WORKDIR /workspace

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY controllers/api controllers/api/
COPY statefulset-runner/api/ statefulset-runner/api/
COPY statefulset-runner/controllers/ statefulset-runner/controllers/
COPY statefulset-runner/main.go statefulset-runner/main.go
COPY tools tools

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -gcflags=all="-N -l" -o manager statefulset-runner/main.go

# Get Delve from a GOPATH not from a Go Modules project
WORKDIR /go/src/
RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM ubuntu

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /go/bin/dlv .
EXPOSE 8080 8081 9443 40000

CMD ["/dlv", "--listen=:40000", "--headless=true", "--api-version=2", "exec", "/manager", "--continue", "--accept-multiclient"]
