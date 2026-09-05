package broker

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	PostgresServiceID = "4f6e6cf6-ffdd-425f-a2c7-3c9258ad246a"
	PostgresPlanID    = "86064792-7ea2-467b-af93-ac9694d96d5b"
)

var identSafe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type postgresOffering struct {
	host    string
	port    int
	sslMode string
	pool    *pgxpool.Pool
}

func newDefaultOfferings(o Options) (store, []Offering, error) {
	st := store(newMemoryBackend())
	var pool *pgxpool.Pool
	if o.PostgresHost != "" {
		p, err := connectAdminPool(context.Background(), o)
		if err != nil {
			return nil, nil, err
		}
		sqlStore, err := newSQLStore(p)
		if err != nil {
			p.Close()
			return nil, nil, err
		}
		st = sqlStore
		pool = p
	}
	// Register further offerings here (nats, minio, …) the same way.
	return st, []Offering{newPostgresOffering(o, pool)}, nil
}

func newPostgresOffering(o Options, pool *pgxpool.Pool) *postgresOffering {
	host := o.PostgresHost
	if host == "" {
		host = "localhost"
	}
	port := o.PostgresPort
	if port == 0 {
		port = 5432
	}
	return &postgresOffering{host: host, port: port, sslMode: sslMode(o), pool: pool}
}

func (p *postgresOffering) Catalog() Service {
	return Service{
		Name:                 "postgres",
		ID:                   PostgresServiceID,
		Description:          "Shared PostgreSQL database",
		Bindable:             true,
		PlanUpdateable:       true,
		InstancesRetrievable: true,
		BindingsRetrievable:  true,
		Tags:                 []string{"postgres", "postgresql", "relational"},
		Metadata: map[string]any{
			"displayName":         "PostgreSQL",
			"providerDisplayName": "osb-service",
			"longDescription":     "Provisions a database and role on a shared Postgres server.",
		},
		Plans: []Plan{{
			Name:        "shared",
			ID:          PostgresPlanID,
			Description: "A database on the shared Postgres instance",
			Free:        boolPtr(true),
		}},
	}
}

func (p *postgresOffering) Healthy(ctx context.Context) error {
	if p.pool == nil {
		return nil
	}
	return p.pool.Ping(ctx)
}

func (p *postgresOffering) Provision(id string, req ProvisionRequest) (instance, error) {
	dbname, err := resourceName("d", id)
	if err != nil {
		return instance{}, err
	}
	username, err := resourceName("u", id)
	if err != nil {
		return instance{}, err
	}
	password, err := randomPassword(24)
	if err != nil {
		return instance{}, err
	}
	creds := postgresCredentials(p.host, p.port, p.sslMode, dbname, username, password)
	inst := instance{req: req, credentials: creds}
	if p.pool != nil {
		if err := p.createDatabaseAndRole(context.Background(), dbname, username, password); err != nil {
			return instance{}, err
		}
	}
	return inst, nil
}

func (p *postgresOffering) Deprovision(inst instance) error {
	if p.pool == nil {
		return nil
	}
	dbname, _ := inst.credentials["database"].(string)
	username, _ := inst.credentials["username"].(string)
	if dbname == "" || username == "" {
		return nil
	}
	return p.dropDatabaseAndRole(context.Background(), dbname, username)
}

func (p *postgresOffering) createDatabaseAndRole(ctx context.Context, dbname, username, password string) error {
	db := quoteIdent(dbname)
	role := quoteIdent(username)
	pass := quoteLiteral(password)
	if _, err := p.pool.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s`, db)); err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	if _, err := p.pool.Exec(ctx, fmt.Sprintf(`CREATE ROLE %s LOGIN PASSWORD %s`, role, pass)); err != nil {
		_ = p.dropDatabase(ctx, dbname)
		return fmt.Errorf("create role: %w", err)
	}
	if _, err := p.pool.Exec(ctx, fmt.Sprintf(`GRANT ALL PRIVILEGES ON DATABASE %s TO %s`, db, role)); err != nil {
		_ = p.dropDatabaseAndRole(ctx, dbname, username)
		return fmt.Errorf("grant database: %w", err)
	}
	if _, err := p.pool.Exec(ctx, fmt.Sprintf(`ALTER DATABASE %s OWNER TO %s`, db, role)); err != nil {
		_ = p.dropDatabaseAndRole(ctx, dbname, username)
		return fmt.Errorf("alter database owner: %w", err)
	}
	if err := p.withDatabase(ctx, dbname, func(conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, fmt.Sprintf(`GRANT ALL ON SCHEMA public TO %s`, role))
		return err
	}); err != nil {
		_ = p.dropDatabaseAndRole(ctx, dbname, username)
		return fmt.Errorf("grant schema: %w", err)
	}
	return nil
}

func (p *postgresOffering) dropDatabaseAndRole(ctx context.Context, dbname, username string) error {
	if err := p.dropDatabase(ctx, dbname); err != nil {
		return err
	}
	_, err := p.pool.Exec(ctx, fmt.Sprintf(`DROP ROLE IF EXISTS %s`, quoteIdent(username)))
	return err
}

func (p *postgresOffering) dropDatabase(ctx context.Context, dbname string) error {
	_, _ = p.pool.Exec(ctx, `
SELECT pg_terminate_backend(pid) FROM pg_stat_activity
WHERE datname = $1 AND pid <> pg_backend_pid()`, dbname)
	_, err := p.pool.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, quoteIdent(dbname)))
	return err
}

func (p *postgresOffering) withDatabase(ctx context.Context, dbname string, fn func(*pgx.Conn) error) error {
	cfg := p.pool.Config().ConnConfig.Copy()
	cfg.Database = dbname
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	return fn(conn)
}

func postgresCredentials(host string, port int, sslMode, dbname, username, password string) map[string]any {
	if sslMode == "" {
		sslMode = "require"
	}
	uri := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(username, password),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     "/" + dbname,
		RawQuery: "sslmode=" + url.QueryEscape(sslMode),
	}
	jdbc := fmt.Sprintf(
		"jdbc:postgresql://%s/%s?ssl=true&sslmode=%s&user=%s&password=%s",
		net.JoinHostPort(host, strconv.Itoa(port)),
		dbname,
		url.QueryEscape(sslMode),
		url.QueryEscape(username),
		url.QueryEscape(password),
	)
	return map[string]any{
		"username": username,
		"password": password,
		"hostname": host,
		"host":     host,
		"port":     port,
		"database": dbname,
		"name":     dbname,
		"sslmode":  sslMode,
		"uri":      uri.String(),
		"jdbcUrl":  jdbc,
	}
}

func quoteIdent(name string) string {
	if !identSafe.MatchString(name) {
		panic("unsafe postgres identifier: " + name)
	}
	return `"` + name + `"`
}

func quoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}
