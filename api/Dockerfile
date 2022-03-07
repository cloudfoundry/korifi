# syntax = docker/dockerfile:experimental
FROM golang:1.17 as builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY controllers/apis controllers/apis/
COPY controllers/webhooks controllers/webhooks/
COPY api/actions api/actions/
COPY api/apis api/apis/
COPY api/apierrors api/apierrors
COPY api/authorization api/authorization/
COPY api/payloads api/payloads/
COPY api/presenter api/presenter/
COPY api/repositories api/repositories/
COPY api/config/config.go api/config/
COPY api/main.go api/
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cfapi api/main.go

FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/cfapi .
USER 1000:1000

ENTRYPOINT [ "/cfapi" ]
