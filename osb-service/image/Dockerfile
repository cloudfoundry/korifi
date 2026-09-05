FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY pkg ./pkg
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /servicebroker ./cmd/servicebroker

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /servicebroker /opt/servicebroker/servicebroker
USER 65532:65532
EXPOSE 8443
ENTRYPOINT ["/opt/servicebroker/servicebroker"]
