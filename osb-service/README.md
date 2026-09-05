# osb-service

Open Service Broker with a catalog of **offerings**. Postgres is the first
offering; add another file next to `pkg/broker/postgres.go` that implements
`Offering` and register it in `newDefaultOfferings`.

The process serves HTTPS on port 8443. TLS cert/key files are required
(`--tls-cert-file`, `--tls-private-key-file`). `--insecure` is only for
local `go run` / tests — deploy never passes it. The postgres admin
connection and bind credentials use `sslmode=require` unless
`--postgres-sslmode` / `POSTGRES_SSLMODE` is set.

```sh
go test ./...
go run ./cmd/servicebroker --insecure --port 8080 \
  --username broker --password change-me
```

Deploy stacks pass backing-store env from `ServiceBrokerServices` and mount
a TLS secret (cert-manager self-signed unless `tlsSecretName` is set).
