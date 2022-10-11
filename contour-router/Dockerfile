# Build the manager binary
FROM golang:1.19.2 as builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.sum ./

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY controllers/api controllers/api
COPY contour-router/api contour-router/api
COPY contour-router/config/config.go contour-router/config/config.go
COPY contour-router/controllers contour-router/controllers
COPY contour-router/main.go contour-router/main.go
COPY tools tools

# Build
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod \
     CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager contour-router/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
