package query_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph/query"
)

func readRaw(t *testing.T) (filter, orderby string) {
	t.Helper()
	f, err := os.ReadFile("raw/directory-objects-filter.md")
	if err != nil {
		t.Fatal(err)
	}
	o, err := os.ReadFile("raw/directory-objects-orderby.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(f), string(o)
}

// matrix.json is made from raw/ and nothing else: a rule that is changed by hand, or a raw file that is
// changed without the rules, is a failing test.
func TestMatrix_IsWhatTheRawFilesSay(t *testing.T) {
	f, o := readRaw(t)
	parsed, err := query.Parse(f, o)
	if err != nil {
		t.Fatal(err)
	}
	built := query.Load()
	if parsed.Commit = built.Commit; built.Commit == "" {
		t.Error("matrix.json has no commit")
	}
	if !reflect.DeepEqual(parsed.Filter, built.Filter) || !reflect.DeepEqual(parsed.OrderBy, built.OrderBy) {
		t.Error("matrix.json is not what raw/ says. Run: make update-query")
	}
}

// The examples of Microsoft's article "Advanced query capabilities on Microsoft Entra ID objects".
func TestMatrix_AnswersAsTheArticleDoes(t *testing.T) {
	m := query.Load()
	for _, c := range []struct {
		resource, property, op string
		want                   query.Mode
	}{
		// "endsWith ... only by mail, otherMails, userPrincipalName, and proxyAddresses": that is not a column.
		// startsWith on a property works by default, and on others it needs an advanced query
		{"user", "displayName", query.OpEq, query.DefaultAdvanced},
		{"user", "displayName", query.OpStartsWith, query.DefaultAdvanced},
		{"user", "accountEnabled", query.OpEq, query.DefaultAdvanced},
		{"user", "accountEnabled", query.OpStartsWith, query.None}, // a boolean has no startsWith
		{"user", "mail", query.OpEqNull, query.Advanced},
		{"user", "companyName", query.OpEqNull, query.Advanced}, // companyName ne null: the article's example needs $count=true
		{"user", "proxyAddresses/any:", query.OpStartsWith, query.DefaultAdvanced},
		{"user", "assignedLicenses/any:skuId", query.OpEq, query.DefaultAdvanced},
		{"user", "assignedLicenses/$count", query.OpCount0, query.Advanced},
		{"user", "assignedLicenses/$count", query.OpCount1, query.None},
		// the article: the createdDateTime of a group is not indexed for eq
		{"group", "createdDateTime", query.OpEq, query.None},
		{"group", "createdDateTime", query.OpGeLe, query.Advanced},
		{"group", "notAProperty", query.OpEq, query.None},
		{"servicePrincipal", "appId", query.OpEq, query.DefaultAdvanced},
		{"servicePrincipal", "tags/any:", query.OpEq, query.DefaultAdvanced},
		// the names of properties are not case sensitive in a filter
		{"user", "DISPLAYNAME", query.OpEq, query.DefaultAdvanced},
	} {
		if got := m.FilterMode(c.resource, c.property, c.op); got != c.want {
			t.Errorf("%s.%s %s = %q, want %q", c.resource, c.property, c.op, got, c.want)
		}
	}
	if !m.HasResource("user") || !m.HasResource("group") || !m.HasResource("servicePrincipal") || m.HasResource("place") {
		t.Error("the resources that the tables cover")
	}
}

func TestMatrix_OrderBy(t *testing.T) {
	m := query.Load()
	for _, c := range []struct {
		resource, property string
		want               query.Mode
	}{
		{"user", "displayName", query.Default},
		{"user", "userPrincipalName", query.Default},
		{"user", "createdDateTime", query.Advanced},
		{"user", "mail", query.None}, // "all others": not supported
		{"group", "displayName", query.Default},
		{"group", "createdDateTime", query.Advanced},
		{"servicePrincipal", "displayName", query.Advanced},
		{"application", "displayName", query.Advanced},
	} {
		if got := m.OrderByMode(c.resource, c.property); got != c.want {
			t.Errorf("orderby %s.%s = %q, want %q", c.resource, c.property, got, c.want)
		}
	}
}

func TestMatrix_RowsAreSortedAndComplete(t *testing.T) {
	rows := query.Load().Rows("user")
	if len(rows) < 50 {
		t.Fatalf("%d rows", len(rows))
	}
	for i := 1; i < len(rows); i++ {
		if strings.ToLower(rows[i-1].Property) > strings.ToLower(rows[i].Property) {
			t.Fatalf("%q is before %q", rows[i-1].Property, rows[i].Property)
		}
	}
	for _, r := range rows {
		if r.Property == "displayName" && (r.OrderBy != query.Default || r.Operators[query.OpEq] != query.DefaultAdvanced) {
			t.Errorf("displayName: %+v", r)
		}
	}
}

func TestParse_FailsOnAFormItDoesNotKnow(t *testing.T) {
	f, o := readRaw(t)
	for name, c := range map[string]struct{ filter, orderby, want string }{
		"a resource":      {strings.Replace(f, "## User properties", "## Printer properties", 1), o, "Printer properties"},
		"a column":        {strings.Replace(f, "| eq Null ", "| eq Nil  ", 1), o, "eq Nil"},
		"a cell":          {strings.Replace(f, "![Advanced][AQP]", "![Maybe][XX]", 1), o, "![Maybe][XX]"},
		"a row too long":  {strings.Replace(f, "| accountEnabled ", "| accountEnabled | ![Advanced][AQP] | ![Advanced][AQP] | ![Advanced][AQP] | ![Advanced][AQP] | ![Advanced][AQP] ", 1), o, "cells"},
		"a property form": {strings.Replace(f, "assignedLicenses/any(a:a/skuId)", "assignedLicenses/some(a:a/skuId)", 1), o, "some("},
		"no tables":       {"nothing here", o, "document has changed its form"},
		"no orderby":      {f, "nothing here", "orderby table"},
	} {
		if _, err := query.Parse(c.filter, c.orderby); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}
