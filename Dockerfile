FROM golang:1.17 as builder

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY main.go main.go
COPY apis/ apis/
COPY config/*.go config/
COPY payloads/ payloads/
COPY presenter/ presenter/
COPY repositories/ repositories/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o cfapi main.go

FROM scratch
WORKDIR /
COPY --from=builder /workspace/cfapi .
USER 1000:1000

ENTRYPOINT [ "/cfapi" ]
