package rule_test

import (
	"strings"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph/rule"
)

func user(extra map[string]any) map[string]any {
	u := map[string]any{
		"id": "aaaa-1", "displayName": "David Berg", "department": "Sales", "country": "Norway", "city": "Oslo",
		"jobTitle": "Account Executive", "mail": "david@acme.example", "userType": "Member", "accountEnabled": true,
		"officeLocation": "Oslo", "mobilePhone": "+47 123", "businessPhones": []any{"+47 999"},
		"onPremisesSyncEnabled": nil, "employeeHireDate": "2026-01-10T00:00:00Z",
		"otherMails":                    []any{"dave@home.example", "d@other.example"},
		"onPremisesExtensionAttributes": map[string]any{"extensionAttribute1": "CC-100", "extensionAttribute2": nil},
	}
	for k, v := range extra {
		u[k] = v
	}
	return u
}

func match(t *testing.T, text string, u map[string]any, env rule.Env) bool {
	t.Helper()
	r, err := rule.Parse(text)
	if err != nil {
		t.Fatalf("%q: %v", text, err)
	}
	return r.Match(u, env)
}

// The examples of the Learn page "Manage rules for dynamic membership groups", and the forms that
// cohort writes.
func TestMatch(t *testing.T) {
	u := user(nil)
	for rule, want := range map[string]bool{
		`user.department -eq "Sales"`:                                                     true,
		`user.department -eq "sales"`:                                                     true, // strings are compared without regard to case
		`(user.department -eq "Sales")`:                                                   true,
		`user.department -ne "Sales"`:                                                     false,
		`user.department -eq "Marketing"`:                                                 false,
		`user.department eq "Sales"`:                                                      true, // the hyphen is optional
		`user.department –eq "Sales"`:                                                     true, // the Learn page also writes an en dash
		`user.displayName -startsWith "Da"`:                                               true,
		`user.displayName -notStartsWith "Da"`:                                            false,
		`user.mail -endsWith "@acme.example"`:                                             true,
		`user.mail -notEndsWith "@contoso.com"`:                                           true,
		`user.jobTitle -contains "Account"`:                                               true,
		`user.jobTitle -notContains "Manager"`:                                            true,
		`user.displayName -match "^Da.*"`:                                                 true,
		`user.displayName -match ".*vid"`:                                                 true,  // David Berg has vid in it (a search, not the whole value)
		`user.displayName -match "^.*vid$"`:                                               false, // and it does not end in vid
		`user.displayName -match "Berg$"`:                                                 true,
		`user.displayName -notMatch "^Da"`:                                                false,
		`user.country -in ["Sweden","Norway"]`:                                            true,
		`user.country -notIn ["Sweden","Norway"]`:                                         false,
		`user.country -in ["Sweden", "Denmark"]`:                                          false,
		`user.accountEnabled -eq true`:                                                    true,
		`user.accountEnabled -eq false`:                                                   false,
		`user.accountEnabled -ne true`:                                                    false,
		`user.dirSyncEnabled -eq true`:                                                    false,
		`user.dirSyncEnabled -eq false`:                                                   true, // a cloud user has null, which is not true
		`user.userType -eq "Member"`:                                                      true,
		`user.userType -eq "member"`:                                                      true,
		`user.physicalDeliveryOfficeName -eq "Oslo"`:                                      true, // Graph calls it officeLocation
		`user.mobile -startsWith "+47"`:                                                   true, // Graph calls it mobilePhone
		`user.objectId -eq "aaaa-1"`:                                                      true,
		`user.telephoneNumber -eq "+47 999"`:                                              true,
		`user.extensionAttribute1 -eq "CC-100"`:                                           true,
		`user.extensionAttribute2 -eq null`:                                               true,
		`user.extensionAttribute3 -eq null`:                                               true,
		`user.extensionAttribute1 -ne null`:                                               true,
		`user.city -ne null`:                                                              true,
		`user.state -eq null`:                                                             true, // not set
		`user.state -ne "Oslo"`:                                                           true, // null is not Oslo
		`user.state -notContains "x"`:                                                     true,
		`user.state -contains "x"`:                                                        false,
		`user.state -eq "x"`:                                                              false,
		`user.objectId -ne null`:                                                          true, // the rule for all users
		`(user.objectId -ne null) -and (user.userType -eq "Member")`:                      true,
		`(user.department -eq "Sales") -or (user.department -eq "Marketing")`:             true,
		`(user.department -eq "Sales") -and -not (user.jobTitle -startsWith "Account")`:   false,
		`(user.department -eq "Sales") -and -not (user.jobTitle -startsWith "SDE")`:       true,
		`user.country -eq "US" -or user.department -eq "Sales" -and user.city -eq "Oslo"`: true, // -and before -or
		`user.country -eq "Norway" -and (user.department -eq "Marketing" -or user.department -eq "Sales")`: true,
		`-not user.department -eq "Marketing"`:                                          true,
		`(user.otherMails -any (_ -startsWith "dave@"))`:                                true,
		`(user.otherMails -any (_ -endsWith "@nowhere.example"))`:                       false,
		`(user.otherMails -all (_ -contains "@"))`:                                      true,
		`(user.otherMails -all (_ -startsWith "dave"))`:                                 false,
		`(user.otherMails -any (_ -startsWith "dave" -and _ -endsWith "home.example"))`: true,
		`user.proxyAddresses -any (_ -startsWith "smtp:")`:                              false, // no proxy addresses
		`user.department -eq "Sa` + "`" + `"les"`:                                       false, // a backtick escapes a quote
	} {
		if got := match(t, rule, u, rule2env()); got != want {
			t.Errorf("%s\n  got %v, want %v", rule, got, want)
		}
	}
}

func rule2env() rule.Env { return rule.Env{} }

func TestMatch_EmployeeHireDate(t *testing.T) {
	env := rule.Env{Now: func() time.Time { return time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC) }}
	u := user(nil) // hired 2026-01-10
	for rule, want := range map[string]bool{
		`user.employeeHireDate -le 2026-01-10T00:00:00Z`:   true,
		`user.employeeHireDate -ge 2026-01-11T00:00:00Z`:   false,
		`user.employeeHireDate -eq "2026-01-10T00:00:00Z"`: true,
		`user.employeeHireDate -le system.now`:             true,
		`user.employeeHireDate -ge system.now -minus p2d`:  true,
		`user.employeeHireDate -ge system.now -minus p1h`:  false,
		`user.employeeHireDate -ge system.now -plus p1d`:   false,
		`user.employeeHireDate -ne null`:                   true,
	} {
		if got := match(t, rule, u, env); got != want {
			t.Errorf("%s: got %v, want %v", rule, got, want)
		}
	}
	if match(t, `user.employeeHireDate -ne null`, user(map[string]any{"employeeHireDate": nil}), env) {
		t.Error("a user with no hire date has null")
	}
}

func TestMatch_DirectReports(t *testing.T) {
	managers := map[string]string{"emp-1": "boss-1", "emp-2": "boss-2"}
	env := rule.Env{Manager: func(id string) string { return managers[id] }}
	r, err := rule.Parse(`Direct Reports for "BOSS-1"`)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"emp-1": true, "emp-2": false, "boss-1": true /* the manager too */, "other": false} {
		if got := r.Match(map[string]any{"id": id}, env); got != want {
			t.Errorf("%s: got %v, want %v", id, got, want)
		}
	}
}

// What Entra refuses when a rule is saved.
func TestParse_Errors(t *testing.T) {
	for text, want := range map[string]string{
		``:                                                       "empty",
		`   `:                                                    "empty",
		`user.department -eq`:                                    "value is needed",
		`user.department "Sales"`:                                "needs an operator",
		`user.department -eq "Sales`:                             "not closed",
		`(user.department -eq "Sales"`:                           "closing parenthesis",
		`user.department -eq "Sales")`:                           "after the expression",
		`user.nosuchthing -eq "x"`:                               "not a property that a membership rule can use",
		`department -eq "x"`:                                     "user.<property>",
		`device.displayName -eq "x"`:                             "users only",
		`user.department -foo "x"`:                               "not known",
		`user.department -ge "x"`:                                "only for employeeHireDate",
		`user.department -in "x"`:                                "takes a list",
		`user.department -eq ["a"]`:                              "takes one value",
		`user.department -startsWith null`:                       "null can only be used",
		`user.accountEnabled -eq "maybe"`:                        "true or false",
		`user.accountEnabled -startsWith "t"`:                    "takes -eq and -ne",
		`user.employeeHireDate -eq "yesterday"`:                  "not one",
		`user.department -match "("`:                             "not valid",
		`user.otherMails -eq "x"`:                                "-any or -all",
		`user.department -any (_ -eq "x")`:                       "not one",
		`user.memberOf -any (group.objectId -in ["x"])`:          "group membership",
		`user.assignedPlans -any (assignedPlan.service -eq "x")`: "assigned plans",
		`user.country -in ["a" "b"]`:                             "separated with commas",
		`user.country -in ["a"`:                                  "not closed",
		strings.Repeat("a", rule.MaxLength+1):                    "the most is",
	} {
		_, err := rule.Parse(text)
		if err == nil {
			t.Errorf("%q: want an error about %q, got none", text, want)
		} else if !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %q does not say %q", text, err, want)
		}
	}
}
