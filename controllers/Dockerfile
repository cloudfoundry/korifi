# syntax = docker/dockerfile:experimental
FROM golang:1.19.2 as builder

WORKDIR /workspace

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY controllers/api controllers/api
COPY controllers/config/config.go controllers/config/config.go
COPY controllers/controllers controllers/controllers
COPY controllers/coordination controllers/coordination
COPY controllers/main.go controllers/main.go
COPY controllers/webhooks controllers/webhooks
COPY tools tools

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager controllers/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
