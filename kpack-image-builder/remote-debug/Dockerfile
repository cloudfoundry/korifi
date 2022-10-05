# syntax = docker/dockerfile:experimental
FROM golang:1.19.2 as builder

WORKDIR /workspace

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY controllers/api controllers/api/
COPY kpack-image-builder/api/ kpack-image-builder/api/
COPY kpack-image-builder/config/config.go kpack-image-builder/config/config.go
COPY kpack-image-builder/controllers/ kpack-image-builder/controllers/
COPY kpack-image-builder/main.go kpack-image-builder/main.go
COPY tools tools

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -gcflags=all="-N -l" -o manager kpack-image-builder/main.go

# Get Delve from a GOPATH not from a Go Modules project
WORKDIR /go/src/
RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM ubuntu

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /go/bin/dlv .
EXPOSE 8080 8081 9443 40000

CMD ["/dlv", "--listen=:40000", "--headless=true", "--api-version=2", "exec", "/manager", "--continue", "--accept-multiclient"]
