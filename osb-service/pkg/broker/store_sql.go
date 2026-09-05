package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sqlStore persists OSB instance/binding metadata in Postgres. It is not
// postgres-the-offering; any offering can use it when POSTGRES_HOST is set.
type sqlStore struct {
	pool *pgxpool.Pool
}

func connectAdminPool(ctx context.Context, o Options) (*pgxpool.Pool, error) {
	if o.PostgresPassword == "" {
		return nil, fmt.Errorf("postgres password is required when --postgres-host is set")
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(o.PostgresUser, o.PostgresPassword),
		Host:     net.JoinHostPort(o.PostgresHost, strconv.Itoa(o.PostgresPort)),
		Path:     "/" + o.PostgresDB,
		RawQuery: "sslmode=" + sslMode(o),
	}
	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 5 * time.Minute
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(connectCtx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return pool, nil
}

func sslMode(o Options) string {
	if o.PostgresSSLMode != "" {
		return o.PostgresSSLMode
	}
	return "require"
}

func newSQLStore(pool *pgxpool.Pool) (*sqlStore, error) {
	s := &sqlStore{pool: pool}
	if err := s.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *sqlStore) ensureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS osb_instances (
  id TEXT PRIMARY KEY,
  service_id TEXT NOT NULL,
  plan_id TEXT NOT NULL,
  credentials JSONB NOT NULL,
  parameters JSONB,
  context JSONB
);
CREATE TABLE IF NOT EXISTS osb_bindings (
  instance_id TEXT NOT NULL REFERENCES osb_instances(id) ON DELETE CASCADE,
  binding_id TEXT NOT NULL,
  credentials JSONB NOT NULL,
  PRIMARY KEY (instance_id, binding_id)
);
`)
	return err
}

func (s *sqlStore) healthy(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *sqlStore) getInstance(id string) (instance, bool, error) {
	var inst instance
	var creds, params, ctxJSON []byte
	err := s.pool.QueryRow(context.Background(), `
SELECT service_id, plan_id, credentials, parameters, context
FROM osb_instances WHERE id = $1`, id).Scan(
		&inst.req.ServiceID, &inst.req.PlanID, &creds, &params, &ctxJSON,
	)
	if err == pgx.ErrNoRows {
		return instance{}, false, nil
	}
	if err != nil {
		return instance{}, false, err
	}
	if len(creds) > 0 {
		_ = json.Unmarshal(creds, &inst.credentials)
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &inst.req.Parameters)
	}
	if len(ctxJSON) > 0 {
		_ = json.Unmarshal(ctxJSON, &inst.req.Context)
	}
	return inst, true, nil
}

func (s *sqlStore) putInstance(id string, inst instance) error {
	ctx := context.Background()
	_, ok, err := s.getInstance(id)
	if err != nil {
		return err
	}
	creds, _ := json.Marshal(inst.credentials)
	params, _ := json.Marshal(inst.req.Parameters)
	ctxJSON, _ := json.Marshal(inst.req.Context)
	if ok {
		_, err = s.pool.Exec(ctx, `
UPDATE osb_instances SET service_id = $2, plan_id = $3, credentials = $4, parameters = $5, context = $6 WHERE id = $1`,
			id, inst.req.ServiceID, inst.req.PlanID, jsonBytes(creds), jsonBytes(params), jsonBytes(ctxJSON),
		)
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO osb_instances (id, service_id, plan_id, credentials, parameters, context)
VALUES ($1, $2, $3, $4, $5, $6)`,
		id, inst.req.ServiceID, inst.req.PlanID, jsonBytes(creds), jsonBytes(params), jsonBytes(ctxJSON),
	)
	return err
}

func (s *sqlStore) deleteInstance(id string) (bool, error) {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM osb_instances WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *sqlStore) getBinding(instanceID, bindingID string) (BindResponse, bool, error) {
	var raw []byte
	err := s.pool.QueryRow(context.Background(), `
SELECT credentials FROM osb_bindings WHERE instance_id = $1 AND binding_id = $2`,
		instanceID, bindingID).Scan(&raw)
	if err == pgx.ErrNoRows {
		return BindResponse{}, false, nil
	}
	if err != nil {
		return BindResponse{}, false, err
	}
	var resp BindResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return BindResponse{}, false, err
	}
	return resp, true, nil
}

func (s *sqlStore) putBinding(instanceID, bindingID string, resp BindResponse) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(), `
INSERT INTO osb_bindings (instance_id, binding_id, credentials) VALUES ($1, $2, $3)`,
		instanceID, bindingID, jsonBytes(raw))
	return err
}

func (s *sqlStore) deleteBinding(instanceID, bindingID string) (bool, error) {
	tag, err := s.pool.Exec(context.Background(), `
DELETE FROM osb_bindings WHERE instance_id = $1 AND binding_id = $2`, instanceID, bindingID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func jsonBytes(b []byte) string {
	if len(b) == 0 || string(b) == "null" {
		return "null"
	}
	return string(b)
}
