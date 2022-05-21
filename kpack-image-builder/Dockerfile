# Build the manager binary
FROM golang:1.18.1 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY controllers/apis controllers/apis/

# Copy the go source
COPY kpack-image-builder/main.go kpack-image-builder/main.go
COPY kpack-image-builder/api/ kpack-image-builder/api/
COPY kpack-image-builder/controllers/ kpack-image-builder/controllers/
COPY kpack-image-builder/config/config.go kpack-image-builder/config/

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
