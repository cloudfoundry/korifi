FROM golang:1.17 as builder

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY main.go main.go
COPY apis/ apis/
COPY config/ config/
COPY routes/ routes/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o cfapi main.go

FROM scratch
WORKDIR /
COPY --from=builder /workspace/cfapi .
COPY config.json /config.json
USER 1000:1000

ENTRYPOINT [ "/cfapi" ]