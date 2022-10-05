# syntax = docker/dockerfile:experimental
FROM golang:1.19.2 as builder

WORKDIR /workspace

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY controllers/api controllers/api
COPY statefulset-runner/api/ statefulset-runner/api
COPY statefulset-runner/controllers/ statefulset-runner/controllers
COPY statefulset-runner/main.go statefulset-runner/main.go
COPY tools tools

RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod \
     CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager statefulset-runner/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
