# syntax = docker/dockerfile:experimental
FROM golang:1.17.6 as builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY controllers/apis controllers/apis/
COPY controllers/controllers controllers/controllers/
COPY controllers/coordination controllers/coordination/
COPY controllers/webhooks controllers/webhooks/
COPY controllers/config/config.go controllers/config/
COPY controllers/main.go controllers/
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager controllers/main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
