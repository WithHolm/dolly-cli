package api_test

import (
	"testing"

	"github.com/withholm/dolly-cli/graph/api"
)

// The shape of an answer, as the real Graph gives it (checked through the Graph Explorer proxy on 2026-10-29).
func TestAnswers_HaveTheShapeOfGraph(t *testing.T) {
	f := &fakeUsers{users: []api.Object{{"id": "2", "displayName": "Alan Turing", "userPrincipalName": "alan@x"}}}
	ts, _ := mount(testEnv{}, api.Users(f))
	defer ts.Close()
	first := func(out map[string]any) map[string]any { return out["value"].([]any)[0].(map[string]any) }

	// one user: @odata.context, and every default property, null when the user has no value (a list is empty)
	code, one := do(t, ts, "GET", "/users/alan@x", "")
	if code != 200 || one["@odata.context"] != "https://graph.microsoft.com/v1.0/$metadata#users/$entity" {
		t.Fatalf("one user: %d %v", code, one)
	}
	for _, name := range []string{"givenName", "jobTitle", "mail", "mobilePhone", "officeLocation", "preferredLanguage", "surname"} {
		if v, has := one[name]; !has || v != nil {
			t.Errorf("%s must be there, and null: %v %v", name, v, has)
		}
	}
	if bp, ok := one["businessPhones"].([]any); !ok || len(bp) != 0 {
		t.Errorf("businessPhones is an empty list: %#v", one["businessPhones"])
	}
	if one["id"] != "2" || one["displayName"] != "Alan Turing" {
		t.Errorf("the values that the user has: %v", one)
	}

	// the list says its set; a $select says its properties in the context
	_, list := do(t, ts, "GET", "/users", "")
	if list["@odata.context"] != "https://graph.microsoft.com/v1.0/$metadata#users" {
		t.Errorf("list context: %v", list["@odata.context"])
	}
	_, sel := do(t, ts, "GET", "/users?$select=displayName", "")
	if sel["@odata.context"] != "https://graph.microsoft.com/v1.0/$metadata#users(displayName)" {
		t.Errorf("select context: %v", sel["@odata.context"])
	}
	// $select adds no id: it gives what it names
	if got := first(sel); len(got) != 1 || got["displayName"] != "Alan Turing" {
		t.Errorf("$select=displayName gives only displayName: %v", got)
	}
	// a property that the schema declares and the user has no value for is null
	if got := first(func() map[string]any { _, o := do(t, ts, "GET", "/users?$select=mobilePhone,id", ""); return o }()); len(got) != 2 || got["id"] != "2" || got["mobilePhone"] != nil {
		t.Errorf("$select=mobilePhone,id: %v", got)
	} else if _, has := got["mobilePhone"]; !has {
		t.Errorf("mobilePhone must be there, with null")
	}
	// a name that is not a property is dropped, with no error
	if code, o := do(t, ts, "GET", "/users?$select=nothing", ""); code != 200 || len(first(o)) != 0 {
		t.Errorf("$select=nothing: %d %v", code, o)
	}
	// the same on one user
	if code, o := do(t, ts, "GET", "/users/alan@x?$select=displayName", ""); code != 200 || len(o) != 2 || o["@odata.context"] != "https://graph.microsoft.com/v1.0/$metadata#users(displayName)/$entity" {
		t.Errorf("one user with $select: %d %v", code, o)
	}
}

func TestTop_ByTheRulesOfGraph(t *testing.T) {
	ts, _ := mount(testEnv{}, api.Users(&fakeUsers{users: []api.Object{{"id": "1", "displayName": "A"}}}))
	defer ts.Close()
	for _, c := range []struct{ top, code, message string }{
		{"0", "Request_UnsupportedQuery", "Invalid page size specified: '0'. Must be between 1 and 999 inclusive."},
		{"1000", "Request_UnsupportedQuery", "Invalid page size specified: '1000'. Must be between 1 and 999 inclusive."},
		{"-1", "Request_UnsupportedQuery", "Invalid page size specified: '-1'. Must be between 1 and 999 inclusive."},
		{"abc", "BadRequest", "Invalid value 'abc' for $top query option found. The $top query option requires a non-negative integer value."},
	} {
		code, out := do(t, ts, "GET", "/users?$top="+c.top, "")
		e, _ := out["error"].(map[string]any)
		if code != 400 || e["code"] != c.code || e["message"] != c.message {
			t.Errorf("$top=%s: %d %v", c.top, code, out)
		}
	}
	for _, top := range []string{"1", "999"} {
		if code, _ := do(t, ts, "GET", "/users?$top="+top, ""); code != 200 {
			t.Errorf("$top=%s: %d", top, code)
		}
	}
}

func TestExpand_ANameThatIsNotAPropertyIsNotFound(t *testing.T) {
	ts, _ := mount(testEnv{}, api.Users(&fakeUsers{users: []api.Object{{"id": "1", "displayName": "A", "userPrincipalName": "a@x"}}}))
	defer ts.Close()
	code, out := do(t, ts, "GET", "/users/a@x?$expand=nothing", "")
	e, _ := out["error"].(map[string]any)
	if code != 400 || e["code"] != "BadRequest" || e["message"] != "Parsing OData Select and Expand failed: Could not find a property named 'nothing' on type 'microsoft.graph.user'." {
		t.Errorf("$expand=nothing: %d %v", code, out)
	}
	// a relation that the user has, and that Dolly does not expand yet, is a different error
	code, out = do(t, ts, "GET", "/users/a@x?$expand=memberOf", "")
	if e, _ = out["error"].(map[string]any); code != 400 || e["code"] != "Request_UnsupportedQuery" {
		t.Errorf("$expand=memberOf: %d %v", code, out)
	}
}
