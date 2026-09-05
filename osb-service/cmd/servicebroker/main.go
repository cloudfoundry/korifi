package main

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"code.cloudfoundry.org/korifi/osb-service/pkg/broker"
)

var version = "dev"

type options struct {
	broker.Options
	port                                        int
	insecure                                    bool
	tlsCertFile, tlsKeyFile, username, password string
}

func main() {
	if err := run(); err != nil {
		slog.Error("service broker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	var o options
	flag.IntVar(&o.port, "port", 8443, "port on which to listen")
	flag.BoolVar(&o.insecure, "insecure", false, "serve HTTP instead of HTTPS")
	flag.StringVar(&o.tlsCertFile, "tls-cert-file", "", "PEM certificate file for HTTPS")
	flag.StringVar(&o.tlsKeyFile, "tls-private-key-file", "", "PEM private key file for HTTPS")
	flag.StringVar(&o.username, "username", os.Getenv("BROKER_USERNAME"), "HTTP Basic authentication username")
	flag.StringVar(&o.password, "password", os.Getenv("BROKER_PASSWORD"), "HTTP Basic authentication password")
	broker.AddFlags(&o.Options)
	flag.Parse()
	if flag.Arg(0) == "version" {
		fmt.Printf("servicebroker/%s\n", version)
		return nil
	}
	logic, err := broker.NewBusinessLogic(o.Options)
	if err != nil {
		return err
	}
	handler := newHandler(logic)
	if o.username != "" || o.password != "" {
		if o.username == "" || o.password == "" {
			return errors.New("both username and password are required")
		}
		handler = basicAuth(o.username, o.password, handler)
	}
	srv := &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("starting service broker", "address", srv.Addr, "osb_api", broker.MinimumAPIVersion, "tls", !o.insecure)
	if o.insecure {
		err = srv.ListenAndServe()
	} else {
		if o.tlsCertFile == "" || o.tlsKeyFile == "" {
			return errors.New("TLS is enabled: set --tls-cert-file and --tls-private-key-file, or use --insecure")
		}
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		err = srv.ListenAndServeTLS(o.tlsCertFile, o.tlsKeyFile)
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func newHandler(logic *broker.BusinessLogic) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := logic.Healthy(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, broker.ErrorResponse{Error: "Unhealthy", Description: err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("# HELP osb_up Whether the broker is running.\n# TYPE osb_up gauge\nosb_up 1\n"))
	})
	mux.HandleFunc("GET /v2/catalog", api(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, logic.Catalog()) }))
	mux.HandleFunc("PUT /v2/service_instances/{instance_id}", api(func(w http.ResponseWriter, r *http.Request) {
		var q broker.ProvisionRequest
		decode(r, &q)
		q.AcceptsIncomplete = queryBool(r, "accepts_incomplete")
		out, status, err := logic.Provision(r.PathValue("instance_id"), q)
		respond(w, status, out, err)
	}))
	mux.HandleFunc("GET /v2/service_instances/{instance_id}", api(func(w http.ResponseWriter, r *http.Request) {
		out, err := logic.GetInstance(r.PathValue("instance_id"))
		respond(w, http.StatusOK, out, err)
	}))
	mux.HandleFunc("PATCH /v2/service_instances/{instance_id}", api(func(w http.ResponseWriter, r *http.Request) {
		var q broker.UpdateRequest
		decode(r, &q)
		q.AcceptsIncomplete = queryBool(r, "accepts_incomplete")
		out, status, err := logic.Update(r.PathValue("instance_id"), q)
		respond(w, status, out, err)
	}))
	mux.HandleFunc("DELETE /v2/service_instances/{instance_id}", api(func(w http.ResponseWriter, r *http.Request) {
		out, status, err := logic.Deprovision(r.PathValue("instance_id"), queryBool(r, "accepts_incomplete"))
		respond(w, status, out, err)
	}))
	mux.HandleFunc("GET /v2/service_instances/{instance_id}/last_operation", api(func(w http.ResponseWriter, r *http.Request) {
		out, err := logic.LastOperation(r.PathValue("instance_id"))
		respond(w, http.StatusOK, out, err)
	}))
	mux.HandleFunc("PUT /v2/service_instances/{instance_id}/service_bindings/{binding_id}", api(func(w http.ResponseWriter, r *http.Request) {
		var q broker.BindRequest
		decode(r, &q)
		out, status, err := logic.Bind(r.PathValue("instance_id"), r.PathValue("binding_id"), q)
		respond(w, status, out, err)
	}))
	mux.HandleFunc("GET /v2/service_instances/{instance_id}/service_bindings/{binding_id}", api(func(w http.ResponseWriter, r *http.Request) {
		out, err := logic.GetBinding(r.PathValue("instance_id"), r.PathValue("binding_id"))
		respond(w, http.StatusOK, out, err)
	}))
	mux.HandleFunc("DELETE /v2/service_instances/{instance_id}/service_bindings/{binding_id}", api(func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, struct{}{}, logic.Unbind(r.PathValue("instance_id"), r.PathValue("binding_id")))
	}))
	return mux
}

type handlerFunc func(http.ResponseWriter, *http.Request)

func api(next handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if id := r.Header.Get("X-Broker-API-Request-Identity"); id != "" {
			w.Header().Set("X-Broker-API-Request-Identity", id)
		}
		v := r.Header.Get("X-Broker-API-Version")
		if v == "" {
			writeJSON(w, http.StatusBadRequest, broker.ErrorResponse{Error: "BadRequest", Description: "X-Broker-API-Version header is required"})
			return
		}
		if !supportedVersion(v) {
			writeJSON(w, http.StatusPreconditionFailed, broker.ErrorResponse{Error: "APIVersionTooOld", Description: "Open Service Broker API version 2.17 or newer is required"})
			return
		}
		defer func() {
			if value := recover(); value != nil {
				if err, ok := value.(error); ok {
					respond(w, 0, nil, broker.BadRequest(err.Error()))
					return
				}
				panic(value)
			}
		}()
		next(w, r)
	}
}
func supportedVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return false
	}
	major, e1 := strconv.Atoi(parts[0])
	minor, e2 := strconv.Atoi(parts[1])
	return e1 == nil && e2 == nil && major == 2 && minor >= 17
}
func decode(r *http.Request, out any) {
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := d.Decode(out); err != nil {
		panic(fmt.Errorf("invalid JSON body: %w", err))
	}
}
func queryBool(r *http.Request, key string) bool { return r.URL.Query().Get(key) == "true" }
func respond(w http.ResponseWriter, status int, out any, err error) {
	if err != nil {
		e := broker.AsAPIError(err)
		writeJSON(w, e.Status, broker.ErrorResponse{Error: e.ErrorCode, Description: e.Description})
		return
	}
	writeJSON(w, status, out)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func basicAuth(username, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/metrics":
			next.ServeHTTP(w, r)
			return
		}
		gotUsername, gotPassword, ok := r.BasicAuth()
		usernameOK := subtle.ConstantTimeCompare([]byte(gotUsername), []byte(username)) == 1
		passwordOK := subtle.ConstantTimeCompare([]byte(gotPassword), []byte(password)) == 1
		if !ok || !usernameOK || !passwordOK {
			w.Header().Set("WWW-Authenticate", `Basic realm="service-broker"`)
			writeJSON(w, http.StatusUnauthorized, broker.ErrorResponse{Error: "Unauthorized", Description: "authentication required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
