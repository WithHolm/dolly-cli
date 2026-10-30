package api

import (
	"cmp"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph/query"
)

func userRules(advanced bool) rules {
	return rules{m: query.Load(), resource: "user", eventual: advanced, advanced: advanced}
}

func run(t *testing.T, filter string, rl rules, o Object) (bool, *Error) {
	t.Helper()
	f, e := compileFilter(filter, rl)
	if e != nil {
		return false, e
	}
	return f(o), nil
}

var ada = Object{
	"id": "1", "displayName": "Ada Lovelace", "department": "Legal", "jobTitle": "Counsel", "accountEnabled": true,
	"mail": "ada@acme.example", "companyName": nil, "createdDateTime": "2024-03-01T10:00:00Z",
	"proxyAddresses":                []string{"SMTP:ada@acme.example", "smtp:ada@acme.onmicrosoft.com"},
	"assignedLicenses":              []any{map[string]any{"skuId": "84a661c4-e949-4bd2-a560-ed7766fcaf2b"}},
	"otherMails":                    []string{},
	"onPremisesExtensionAttributes": map[string]any{"extensionAttribute1": "blue"},
}

// What the filter says about an item, with the rules of Graph for users, in an advanced query so that nothing is refused.
func TestFilter_Evaluates(t *testing.T) {
	for filter, want := range map[string]bool{
		`department eq 'Legal'`:                           true,
		`department eq 'legal'`:                           true, // capitals do not matter
		`department eq 'Sales'`:                           false,
		`department ne 'Sales'`:                           true,
		`accountEnabled eq true`:                          true,
		`accountEnabled eq false`:                         false,
		`startsWith(displayName,'ada')`:                   true,
		`startswith(displayName, 'Lovelace')`:             false,
		`endsWith(mail,'@acme.example')`:                  true,
		`department in ('Sales','Legal')`:                 true,
		`department in ('Sales','HR')`:                    false,
		`department eq 'Legal' and jobTitle eq 'Counsel'`: true,
		`department eq 'Sales' or jobTitle eq 'Counsel'`:  true,
		`department eq 'Sales' or jobTitle eq 'Sales'`:    false,
		`not(department eq 'Sales')`:                      true,
		`NOT startsWith(displayName,'Ada')`:               false,
		`(department eq 'Sales' or department eq 'Legal') and accountEnabled eq true`: true,
		`companyName eq null`:                                                     true, // a property with no value is null
		`mail ne null`:                                                            true,
		`jobTitle eq null`:                                                        false,
		`createdDateTime ge 2024-01-01T00:00:00Z`:                                 true,
		`createdDateTime le 2024-01-01T00:00:00Z`:                                 false,
		`createdDateTime ge 2024-03-01`:                                           true,
		`proxyAddresses/any(p:startsWith(p,'SMTP:'))`:                             true,
		`proxyAddresses/any(p:p eq 'nobody')`:                                     false,
		`proxyAddresses/any(p:endsWith(p,'onmicrosoft.com'))`:                     true,
		`assignedLicenses/any(a:a/skuId eq 84a661c4-e949-4bd2-a560-ed7766fcaf2b)`: true,
		`assignedLicenses/any(a:a/skuId eq 00000000-0000-0000-0000-000000000000)`: false,
		`assignedLicenses/$count ne 0`:                                            true,
		`assignedLicenses/$count eq 0`:                                            false,
		`otherMails/$count eq 0`:                                                  true,
		`proxyAddresses/any()`:                                                    true,
		`otherMails/any()`:                                                        false,
		`proxyAddresses/all(p:startsWith(p,'smtp:'))`:                             true, // capitals do not matter
	} {
		got, e := run(t, filter, userRules(true), ada)
		if e != nil {
			t.Errorf("%s: %v", filter, e)
		} else if got != want {
			t.Errorf("%s = %v, want %v", filter, got, want)
		}
	}
	// the property path into an object, and the tags of a service principal, where capitals matter
	if got, e := run(t, `onPremisesExtensionAttributes/extensionAttribute1 eq 'blue'`, rules{m: query.Load()}, ada); e != nil || !got {
		t.Errorf("a path into an object: %v %v", got, e)
	}
	sp := Object{"tags": []string{"WindowsAzureActiveDirectoryIntegratedApp"}}
	spRules := rules{m: query.Load(), resource: "servicePrincipal", advanced: true}
	for filter, want := range map[string]bool{
		`tags/any(t:t eq 'WindowsAzureActiveDirectoryIntegratedApp')`: true,
		`tags/any(t:t eq 'windowsazureactivedirectoryintegratedapp')`: false,
	} {
		if got, e := run(t, filter, spRules, sp); e != nil || got != want {
			t.Errorf("%s = %v %v, want %v", filter, got, e, want)
		}
	}
}

func TestFilter_ReadsOnlyWhatIsAFilter(t *testing.T) {
	for _, bad := range []string{
		`department eq`, `department frob 'x'`, `eq 'x'`, `department eq 'x' and`, `(department eq 'x'`, `department eq 'unclosed`,
		`department eq 'x' junk`, `startsWith(department)`, `startsWith(department,1)`, `unknown(department,'x')`, `p/any(x)`, `department in 'a'`, `a/b/frob(x:x)`,
		`createdDateTime ge 2024-13-45T99:99:99Z`,
	} {
		_, e := run(t, bad, rules{m: query.Load()}, ada)
		// the code is BadRequest, and the message says where: "Invalid filter clause: <what> at position <n> in '<filter>'."
		if e == nil || e.Code != "BadRequest" || !strings.HasPrefix(e.Message, "Invalid filter clause: ") || !strings.Contains(e.Message, " at position ") || !strings.HasSuffix(e.Message, " in '"+bad+"'.") {
			t.Errorf("%q must be an invalid filter clause: %+v", bad, e)
		}
	}
}

// The tables of Microsoft's article "Advanced query capabilities on Microsoft Entra ID objects", and the answers of the real
// Graph (checked through the Graph Explorer proxy on 2026-10-29): the examples, and the errors. mode is what the call has:
// "" (none), "header" (ConsistencyLevel: eventual), or "advanced" (the header and $count=true).
func TestFilter_RulesOfGraph(t *testing.T) {
	const unsupported = "Request_UnsupportedQuery"
	missing := func(op string) string { // not, endsWith: only the header is missing
		return "Operator '" + op + "' is not supported because the 'ConsistencyLevel:eventual' header is missing. Refer to https://aka.ms/graph-docs/advanced-queries for more information"
	}
	const notEquals = "Filter operator 'NotEqualsMatch' is not supported."
	clause := func(property, resource string) string {
		return "Unsupported or invalid query filter clause specified for property '" + property + "' of resource '" + resource + "'."
	}
	for _, c := range []struct {
		name, filter, resource string
		mode                   string
		want                   string // "" for no error, else the whole message
		code                   string // the code of the error, when it is not Request_UnsupportedQuery
	}{
		// works by default, and in an advanced query
		{"eq by default", `department eq 'Legal'`, "user", "", "", ""},
		{"eq in advanced", `department eq 'Legal'`, "user", "advanced", "", ""},
		{"startsWith by default", `startsWith(displayName,'A')`, "user", "", "", ""},
		{"a boolean", `accountEnabled eq true`, "user", "", "", ""},
		{"in follows eq", `department in ('a','b')`, "user", "", "", ""},
		{"a collection", `assignedLicenses/any(a:a/skuId eq 84a661c4-e949-4bd2-a560-ed7766fcaf2b)`, "user", "", "", ""},
		{"a date", `createdDateTime ge 2024-01-01T00:00:00Z`, "user", "", "", ""},
		{"eq on a date works where ge and le do", `createdDateTime eq 2024-01-01T00:00:00Z`, "user", "advanced", "", ""},
		{"a group's createdDateTime with eq, in an advanced query", `createdDateTime eq 2024-01-01T00:00:00Z`, "group", "advanced", "", ""},
		{"a declared property that the table does not list (the real Graph answers it)", `showInAddressList eq true`, "user", "advanced", "", ""},
		{"a group by name", `displayName eq 'grp.x'`, "group", "", "", ""},
		{"a service principal by appId", `appId eq '00000003-0000-0000-c000-000000000000'`, "servicePrincipal", "", "", ""},
		// ne needs the header and $count=true. The header alone is not enough.
		{"ne", `department ne 'Legal'`, "user", "", notEquals, ""},
		{"ne with the header only", `department ne 'Legal'`, "user", "header", notEquals, ""},
		{"ne in advanced", `department ne 'Legal'`, "user", "advanced", "", ""},
		{"ne null", `mail ne null`, "user", "", notEquals, ""},
		{"ne null and NOT eq", `companyName ne null and NOT(companyName eq 'Microsoft')`, "user", "", notEquals, ""},
		{"ne null and NOT eq in advanced", `companyName ne null and NOT(companyName eq 'Microsoft')`, "user", "advanced", "", ""},
		// not and endsWith need only the header
		{"not startsWith", `NOT startsWith(displayName, 'Conf')`, "user", "", missing("not"), ""},
		{"not startsWith with the header only", `NOT startsWith(displayName, 'Conf')`, "user", "header", "", ""},
		{"endsWith", `endsWith(mail,'@outlook.com')`, "user", "", missing("endsWith"), ""},
		{"endsWith with the header only", `endsWith(mail,'@outlook.com')`, "user", "header", "", ""},
		{"endsWith on a collection", `proxyAddresses/any(p:endsWith(p,'contoso.com'))`, "user", "header", "", ""},
		// a property that needs an advanced query is not valid without one
		{"startsWith on mobilePhone", `startsWith(mobilePhone,'25478') OR startsWith(mobilePhone,'25473')`, "user", "advanced", "", ""},
		{"startsWith on mobilePhone needs it", `startsWith(mobilePhone,'25478')`, "user", "", clause("mobilePhone", "User"), ""},
		{"eq on officeLocation needs it", `officeLocation eq 'x'`, "user", "", clause("officeLocation", "User"), ""},
		{"eq on officeLocation, advanced", `officeLocation eq 'x'`, "user", "advanced", "", ""},
		{"count in a filter", `assignedLicenses/$count ne 0`, "user", "advanced", "", ""},
		{"count needs it", `assignedLicenses/$count eq 0`, "user", "", "Expect simple name=value query, but observe property 'assignedLicenses' of complex type 'AssignedLicense'.", "Request_BadRequest"},
		// what Graph does not support at all
		{"endsWith on displayName", `endsWith(displayName,'x')`, "user", "advanced", clause("displayName", "User"), ""},
		{"a property that the resource does not declare", `nickname eq 'x'`, "user", "advanced", "Property 'nickname' does not exist as a declared property or extension property.", ""},
		{"a property that the table lists with no cell for the operator", `startsWith(isResourceAccount,'t')`, "user", "advanced", clause("isResourceAccount", "User"), ""},
		{"contains", `contains(displayName,'x')`, "user", "advanced", "Operator: 'Contains' is not supported.", ""},
		{"startsWith on a boolean", `startsWith(accountEnabled,'t')`, "user", "advanced", clause("accountEnabled", "User"), ""},
		{"count one that is not supported", `assignedLicenses/$count eq 1`, "user", "advanced", clause("assignedLicenses/$count", "User"), ""},
		// something that is not a directory object has no such rules
		{"places take any text property", `anything eq 'x'`, "place", "", "", ""},
	} {
		rl := rules{m: query.Load(), resource: c.resource, eventual: c.mode != "", advanced: c.mode == "advanced"}
		_, e := compileFilter(c.filter, rl)
		code := cmp.Or(c.code, unsupported)
		switch {
		case c.want == "" && e != nil:
			t.Errorf("%s: %q must work, and got %v", c.name, c.filter, e)
		case c.want != "" && (e == nil || e.Message != c.want || e.Code != code || e.Status != 400):
			t.Errorf("%s: %q\n  want %q (%s)\n  got  %+v", c.name, c.filter, c.want, code, e)
		}
	}
}
