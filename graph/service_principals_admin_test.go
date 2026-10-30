package graph_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

func TestAdmin_TenantAndServicePrincipals(t *testing.T) {
	srv := graph.New(graph.Generate(graph.Options{Users: 10, Seed: 1}), graph.ServerOptions{})
	h := srv.Handler()
	const app = "11111111-2222-3333-4444-555555555555"
	if _, err := srv.World().EnsureServicePrincipal(app, graph.Actor{Source: "login"}); err != nil {
		t.Fatal(err)
	}

	// the tenant ID
	rec := send(h, "GET", "/_dolly/tenant", "")
	if !strings.Contains(rec.Body.String(), graph.FakeTenantID) {
		t.Errorf("tenant: %s", rec.Body)
	}
	if rec = send(h, "PUT", "/_dolly/tenant", `{"id":"not a guid"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a tenant ID that is not a GUID: %d", rec.Code)
	}
	if rec = send(h, "PUT", "/_dolly/tenant", `{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`); rec.Code != 200 {
		t.Fatalf("set tenant: %d %s", rec.Code, rec.Body)
	}
	if got := send(h, "GET", "/v1.0/organization", "").Body.String(); !strings.Contains(got, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee") {
		t.Errorf("/organization after the change: %s", got)
	}

	// the service principals, the mode, and the credential
	var list struct {
		ServicePrincipals []struct{ AppID, Mode string }
	}
	rec = send(h, "GET", "/_dolly/service-principals", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.ServicePrincipals) < 5 {
		t.Fatalf("apps: %v %s", err, rec.Body)
	}
	if rec = send(h, "PUT", "/_dolly/service-principals/"+app+"/mode", `{"mode":"enforce"}`); rec.Code != 204 {
		t.Fatalf("mode: %d %s", rec.Code, rec.Body)
	}
	if rec = send(h, "PUT", "/_dolly/service-principals/"+app+"/mode", `{"mode":"loud"}`); rec.Code != 400 {
		t.Errorf("a mode that is not known: %d", rec.Code)
	}
	if rec = send(h, "PUT", "/_dolly/service-principals/99999999-2222-3333-4444-555555555555/mode", `{"mode":"open"}`); rec.Code != 404 {
		t.Errorf("an app that is not known: %d", rec.Code)
	}
	if rec = send(h, "POST", "/_dolly/service-principals/"+app+"/credential/arm", ""); rec.Code != 204 {
		t.Fatalf("arm: %d %s", rec.Code, rec.Body)
	}
	a, _ := srv.World().FindServicePrincipal(app)
	if a.Mode != "enforce" || a.Credential.State != "armed" {
		t.Errorf("app: %+v", a)
	}
	if rec = send(h, "DELETE", "/_dolly/service-principals/"+app+"/credential", ""); rec.Code != 204 {
		t.Fatalf("clear: %d", rec.Code)
	}
	if a, _ = srv.World().FindServicePrincipal(app); a.Credential.State != "any" {
		t.Errorf("after clear: %+v", a.Credential)
	}
}
