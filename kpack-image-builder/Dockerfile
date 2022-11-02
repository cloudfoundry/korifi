# syntax = docker/dockerfile:experimental
FROM golang:1.19.3 as builder

WORKDIR /workspace

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY controllers/api controllers/api
COPY kpack-image-builder/api/ kpack-image-builder/api
COPY kpack-image-builder/config/config.go kpack-image-builder/config/config.go
COPY kpack-image-builder/controllers/ kpack-image-builder/controllers
COPY kpack-image-builder/main.go kpack-image-builder/main.go
COPY tools tools

# Build
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager kpack-image-builder/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
