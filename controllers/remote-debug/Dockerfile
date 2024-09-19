# syntax = docker/dockerfile:experimental
FROM golang:1.23 as builder

ARG version=dev

WORKDIR /workspace

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY model model
COPY controllers controllers
COPY kpack-image-builder kpack-image-builder
COPY job-task-runner job-task-runner
COPY statefulset-runner statefulset-runner
COPY tools tools
COPY version version

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -ldflags "-X code.cloudfoundry.org/korifi/version.Version=${version}" -gcflags=all="-N -l" -o manager controllers/main.go

# Get Delve from a GOPATH not from a Go Modules project
WORKDIR /go/src/
RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM ubuntu

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /go/bin/dlv .
EXPOSE 8080 8081 9443 40000

CMD ["/dlv", "--listen=:40000", "--headless=true", "--api-version=2", "exec", "/manager", "--continue", "--accept-multiclient"]
