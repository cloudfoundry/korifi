FROM golang:1.18.3 as builder
WORKDIR /eirini/
COPY . .
RUN  CGO_ENABLED=0 GOOS=linux go build -mod vendor -trimpath -a -installsuffix cgo -o notdora ./tests/integration/assets/notdora

FROM scratch
COPY --from=builder /eirini/notdora /notdora
USER 1001

ENV PORT=8888
EXPOSE 8888

ENTRYPOINT ["/notdora"]
