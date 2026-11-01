package cost

import (
	"net/url"
	"strings"
	"testing"
)

func withOverrides(t *testing.T, json string) *Data {
	t.Helper()
	d, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.addOverrides([]byte(json)); err != nil {
		t.Fatal(err)
	}
	return d
}

// A reported cost beats the default and the table of Microsoft, the options of the query still add to it, and the route says that it is reported.
func TestOverrides_BeatTheTableAndTheDefault(t *testing.T) {
	d := withOverrides(t, `{"overrides":[
	  {"method":"post","path":"/applications","ru":4,"write":1,"source":"reported by a person, issue 12"},
	  {"method":"GET","path":"widgets/{id}","ru":3,"write":0,"source":"reported by a person, issue 13"},
	  {"method":"GET","path":"applications","ru":9,"write":0,"source":"a person","note":"Microsoft's table says 2, and a person measured 9"}]}`)

	p := d.Price("GET", "widgets/42", url.Values{})
	if p.RU != 3 || p.Route != "GET widgets/{id} (reported)" {
		t.Errorf("a route that the table does not name: %+v", p)
	}
	// the options of the query change it as they change a row of the table ($select has its own modifier)
	want := 3
	for _, m := range d.Identity.Modifiers {
		if m.When == "$select" && m.RUSet == 0 {
			want += m.RU
		}
	}
	if p := d.Price("GET", "widgets/42", url.Values{"$select": {"id"}}); p.RU != max(want, d.Identity.MinimumRU) {
		t.Errorf("$select: %+v, want %d", p, want)
	}
	if p := d.Price("POST", "applications", url.Values{}); p.RU != 4 || p.Write != 1 || !strings.HasSuffix(p.Route, "(reported)") {
		t.Errorf("a write: %+v", p)
	}
	if p := d.Price("GET", "applications", url.Values{}); p.RU != 9 || !strings.HasSuffix(p.Route, "(reported)") {
		t.Errorf("it beats a row of Microsoft's table: %+v", p)
	}
	// a route with no override is as it was
	if p := d.Price("GET", "users", url.Values{}); strings.HasSuffix(p.Route, "(reported)") {
		t.Errorf("users: %+v", p)
	}
	if p := d.Price("GET", "groups/1/nothing", url.Values{}); p.Route != "GET (other)" || p.RU != 1 {
		t.Errorf("the default: %+v", p)
	}
}

func TestOverrides_AreChecked(t *testing.T) {
	for name, c := range map[string]struct{ json, want string }{
		"no source":    {`{"overrides":[{"method":"GET","path":"a","ru":2}]}`, "no source"},
		"no path":      {`{"overrides":[{"method":"GET","path":"/","ru":2,"source":"x"}]}`, "no path"},
		"bad method":   {`{"overrides":[{"method":"FETCH","path":"a","ru":2,"source":"x"}]}`, "not a method"},
		"ru too small": {`{"overrides":[{"method":"GET","path":"a","ru":0,"source":"x"}]}`, "1 or more"},
		"twice":        {`{"overrides":[{"method":"GET","path":"a","ru":2,"source":"x"},{"method":"get","path":"/A/","ru":3,"source":"y"}]}`, "twice"},
		"unknown":      {`{"overrides":[{"method":"GET","path":"a","ru":2,"source":"x","cost":1}]}`, "unknown field"},
	} {
		d, _ := Parse(raw)
		if err := d.addOverrides([]byte(c.json)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The overrides that are in the program: they load, and an override for a route that Microsoft's table names says why.
func TestOverrides_InTheProgram(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range d.Overrides() {
		if d.HasRow(o.Method, o.Path) && strings.TrimSpace(o.Note) == "" {
			t.Errorf("%s %s is in Microsoft's table: the override needs a note that says why", o.Method, o.Path)
		}
	}
}
