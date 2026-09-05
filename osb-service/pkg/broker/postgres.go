package broker

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
)

const (
	PostgresServiceID = "4f6e6cf6-ffdd-425f-a2c7-3c9258ad246a"
	PostgresPlanID    = "86064792-7ea2-467b-af93-ac9694d96d5b"
)

type postgresOffering struct {
	sslMode string
	everest *everestClient
}

func newDefaultOfferings(o Options) (store, []Offering, error) {
	st := store(newMemoryBackend())
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
	}
	everest, err := newEverestClient(o)
	if err != nil {
		return nil, nil, err
	}
	return st, []Offering{newPostgresOffering(o, everest)}, nil
}

func newPostgresOffering(o Options, everest *everestClient) *postgresOffering {
	return &postgresOffering{sslMode: sslMode(o), everest: everest}
}

func (p *postgresOffering) Catalog() Service {
	return Service{
		Name:                 "postgres",
		ID:                   PostgresServiceID,
		Description:          "Dedicated PostgreSQL cluster",
		Bindable:             true,
		PlanUpdateable:       true,
		InstancesRetrievable: true,
		BindingsRetrievable:  true,
		Tags:                 []string{"postgres", "postgresql", "relational"},
		Metadata: map[string]any{
			"displayName":         "PostgreSQL",
			"providerDisplayName": "osb-service",
			"longDescription":     "Provisions a dedicated PostgreSQL cluster via OpenEverest.",
		},
		Plans: []Plan{{
			Name:        "dedicated",
			ID:          PostgresPlanID,
			Description: "A dedicated PostgreSQL cluster",
			Free:        boolPtr(true),
		}},
	}
}

func (p *postgresOffering) Healthy(ctx context.Context) error {
	if p.everest == nil {
		return nil
	}
	return p.everest.healthy(ctx)
}

func (p *postgresOffering) Provision(id string, req ProvisionRequest) (instance, error) {
	if p.everest != nil {
		creds, err := p.everest.provision(context.Background(), id)
		if err != nil {
			return instance{}, err
		}
		return instance{req: req, credentials: creds}, nil
	}
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
	creds := postgresCredentials("localhost", 5432, p.sslMode, dbname, username, password)
	return instance{req: req, credentials: creds}, nil
}

func (p *postgresOffering) Deprovision(inst instance) error {
	if p.everest == nil {
		return nil
	}
	name, _ := inst.credentials["cluster"].(string)
	if name == "" {
		return nil
	}
	return p.everest.deprovision(context.Background(), name)
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
