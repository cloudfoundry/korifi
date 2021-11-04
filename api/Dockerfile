FROM golang:1.17 as builder

WORKDIR /workspace
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o cfapi api/main.go

FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/cfapi .
USER 1000:1000

ENTRYPOINT [ "/cfapi" ]
