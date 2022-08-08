# Build the manager binary
FROM golang:1.18.4 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY controllers/api controllers/api/
COPY tools tools
COPY job-task-runner/main.go job-task-runner/main.go
COPY job-task-runner/api/ job-task-runner/api/
COPY job-task-runner/controllers/ job-task-runner/controllers/

# Build
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod \
     CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager job-task-runner/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
