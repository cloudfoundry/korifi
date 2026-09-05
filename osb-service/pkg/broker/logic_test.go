package broker

import (
	"net/http"
	"strings"
	"testing"
)

func TestAsyncProvision(t *testing.T) {
	b, err := NewBusinessLogic(Options{Async: true})
	if err != nil {
		t.Fatal(err)
	}
	out, status, err := b.Provision("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", ProvisionRequest{ServiceID: ServiceID, PlanID: PlanID, AcceptsIncomplete: true})
	if err != nil || status != http.StatusAccepted || out.Operation == "" {
		t.Fatalf("got %#v, %d, %v", out, status, err)
	}
}

func TestProvisionConflict(t *testing.T) {
	b, _ := NewBusinessLogic(Options{})
	id := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	_, _, _ = b.Provision(id, ProvisionRequest{ServiceID: ServiceID, PlanID: PlanID})
	_, _, err := b.Provision(id, ProvisionRequest{ServiceID: ServiceID, PlanID: "different"})
	if got := AsAPIError(err).Status; got != http.StatusConflict {
		t.Fatalf("got %d", got)
	}
}

func TestCatalogIsPostgres(t *testing.T) {
	b, err := NewBusinessLogic(Options{})
	if err != nil {
		t.Fatal(err)
	}
	c := b.Catalog()
	if len(c.Services) != 1 || c.Services[0].Name != "postgres" || c.Services[0].Plans[0].Name != "dedicated" {
		t.Fatalf("unexpected catalog: %#v", c)
	}
}

func TestBindCredentials(t *testing.T) {
	b, err := NewBusinessLogic(Options{})
	if err != nil {
		t.Fatal(err)
	}
	id := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	_, status, err := b.Provision(id, ProvisionRequest{ServiceID: ServiceID, PlanID: PlanID})
	if err != nil || status != http.StatusCreated {
		t.Fatalf("provision: %d %v", status, err)
	}
	resp, status, err := b.Bind(id, "bbbbbbbb-cccc-dddd-eeee-ffffffffffff", BindRequest{ServiceID: ServiceID, PlanID: PlanID})
	if err != nil || status != http.StatusCreated {
		t.Fatalf("bind: %d %v", status, err)
	}
	for _, key := range []string{"uri", "jdbcUrl", "username", "password", "hostname", "database", "sslmode"} {
		if resp.Credentials[key] == nil || resp.Credentials[key] == "" {
			t.Fatalf("missing credential %q: %#v", key, resp.Credentials)
		}
	}
	if resp.Credentials["sslmode"] != "require" {
		t.Fatalf("sslmode: got %#v", resp.Credentials["sslmode"])
	}
	uri, _ := resp.Credentials["uri"].(string)
	if !strings.Contains(uri, "sslmode=require") {
		t.Fatalf("uri missing sslmode=require: %s", uri)
	}
}

func TestResourceName(t *testing.T) {
	got, err := resourceName("d", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	if err != nil {
		t.Fatal(err)
	}
	if got != "daaaaaaaabbbbccccddddeeeeeeeeeeee" {
		t.Fatalf("got %q", got)
	}
	if _, err := resourceName("d", "not a uuid!"); err == nil {
		t.Fatal("expected error")
	}
}
