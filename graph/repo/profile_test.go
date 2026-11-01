package repo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func hashOf(t *testing.T, w *World) string {
	t.Helper()
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func groupNamed(w *World, name string) *Group {
	for _, g := range w.Groups {
		if g.Name == name {
			return g
		}
	}
	return nil
}

func norway(t *testing.T) *Profile {
	t.Helper()
	f, err := os.Open("../../docs/profiles/norway.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p, err := ParseProfile(f)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// The default profile is valid, it survives a trip through JSON, and a world made from the copy is the world made from the default.
func TestProfile_DefaultIsValidAndSurvivesJSON(t *testing.T) {
	if err := DefaultProfile().Validate(); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(DefaultProfile())
	p, err := ParseProfile(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	a, c := Generate(Options{Users: 60, Seed: 1}), Generate(Options{Users: 60, Seed: 1, Profile: p})
	if a.Profile != nil || c.Profile == nil {
		t.Error("the seed keeps a profile that was given, and not the default one")
	}
	c.Profile = nil
	if hashOf(t, a) != hashOf(t, c) {
		t.Error("a profile that came from JSON makes other data than the default one")
	}
	if hashOf(t, a) == hashOf(t, Generate(Options{Users: 60, Seed: 2})) {
		t.Error("another seed must make other data")
	}
}

// A profile from outside is not trusted: every field has a limit, and the error says which field.
func TestProfile_RefusesWhatIsNotValid(t *testing.T) {
	change := func(f func(p *Profile)) []byte {
		p := DefaultProfile()
		f(p)
		b, _ := json.Marshal(p)
		return b
	}
	for _, c := range []struct {
		name string
		body []byte
		want string // a piece of the error
	}{
		{"not JSON", []byte(`{`), "not valid JSON"},
		{"a field that is not known", []byte(`{"version":1,"surprise":true}`), "unknown field"},
		{"the version", change(func(p *Profile) { p.Version = 2 }), "version: must be 1"},
		{"no domain", change(func(p *Profile) { p.Organization.Domain = "" }), "organization.domain"},
		{"a domain that is not one", change(func(p *Profile) { p.Organization.Domain = "no spaces.example" }), "not a domain name"},
		{"a country code", change(func(p *Profile) { p.Organization.CountryLetterCode = "SWE" }), "two letters"},
		{"one office", change(func(p *Profile) { p.Offices = p.Offices[:1] }), "offices: needs 2"},
		{"a calling code", change(func(p *Profile) { p.Offices[0].CallingCode = "+46" }), "callingCode"},
		{"a weight that is too big", change(func(p *Profile) { p.Departments[0].Weight = 5000 }), "departments[0].weight"},
		{"a department twice", change(func(p *Profile) { p.Departments[1].Name = p.Departments[0].Name }), "is there twice"},
		{"no titles", change(func(p *Profile) { p.Departments[0].Titles = nil }), "departments[0].titles"},
		{"a percent over 100", change(func(p *Profile) { p.People.GuestPercent = 101 }), "people.guestPercent"},
		{"a name that is too long", change(func(p *Profile) { p.Names.First[0] = strings.Repeat("a", 201) }), "names.first[0]"},
		{"a control character", change(func(p *Profile) { p.Names.Last[0] = "Ol\nsen" }), "control character"},
		{"a pattern with a brace that is not a range", change(func(p *Profile) { p.Formats.MobilePhone = "+{x} ###" }), "formats.mobilePhone"},
		{"a run of # that is too long", change(func(p *Profile) { p.Formats.CostCenter = "##########" }), "more than 9"},
		{"a backwards range", change(func(p *Profile) { p.Formats.CostCenter = "{9-1}" }), "formats.costCenter"},
		{"an employee id with no number", change(func(p *Profile) { p.Formats.EmployeeID.Pattern = "E" }), "formats.employeeId.pattern"},
		{"an employee id with two numbers", change(func(p *Profile) { p.Formats.EmployeeID.Pattern = "##-##" }), "exactly one run"},
		{"an attribute with both", change(func(p *Profile) { p.Extension[0].Values = []ProfileValue{{"x", 1}} }), "values and a pattern"},
		{"an attribute with neither", change(func(p *Profile) { p.Extension[0].Pattern = "" }), "needs values or a pattern"},
		{"an attribute index", change(func(p *Profile) { p.Extension[0].Index = 16 }), "from 1 to 15"},
		{"an attribute twice", change(func(p *Profile) { p.Extension[1].Index = 1 }), "is there twice"},
		{"a department that is not there", change(func(p *Profile) { p.Groups.DynamicDepartment = "Nowhere" }), "not a department"},
		{"an app name with a dot", change(func(p *Profile) { p.Groups.Apps[0].Name = "a.b" }), "lowercase letters"},
		{"an owner in no department", change(func(p *Profile) { p.Owner.Department = "Nowhere" }), "owner.department"},
		{"an admin pattern with no name", change(func(p *Profile) { p.Admin.UserPrincipalName = "admin" }), "needs {first} or {last}"},
		{"an admin pattern with a space", change(func(p *Profile) { p.Admin.UserPrincipalName = "a {first}" }), "admin.userPrincipalName"},
		{"an admin pattern with another token", change(func(p *Profile) { p.Admin.UserPrincipalName = "a.{middle}" }), "{middle}"},
		{"a file that is too big", bytes.Repeat([]byte(" "), maxProfileBytes+10), "bigger than"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseProfile(bytes.NewReader(c.body))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("want an error with %q, got %v", c.want, err)
			}
		})
	}
	// Generate never takes a profile that is not valid through the checked way
	bad := DefaultProfile()
	bad.Offices = nil
	if _, err := GenerateChecked(Options{Users: 5, Profile: bad}); err == nil {
		t.Error("GenerateChecked must refuse a profile with no offices")
	}
}

// The pattern language: # is a number with that many digits (one draw for each run), {a-b} is a range, {cc} is the calling code.
func TestPattern_Language(t *testing.T) {
	g := &gen{rng: rand.New(rand.NewPCG(1, 2))}
	for _, c := range []struct{ pattern, re string }{
		{"E#####", `^E\d{5}$`},
		{"+{cc} 7# ### ## ##", `^\+47 7\d \d{3} \d{2} \d{2}$`},
		{"+4# 8 ### ###", `^\+4\d 8 \d{3} \d{3}$`},
		{"CC-{100-119}", `^CC-1(0\d|1\d)$`},
		{"SITE-{01-12}", `^SITE-(0[1-9]|1[0-2])$`},
		{"PRJ-{000-059}", `^PRJ-0[0-5]\d$`},
		{"plain text", `^plain text$`},
	} {
		for i := 0; i < 200; i++ {
			got := g.pattern(c.pattern, map[string]string{"cc": "47"})
			if !regexp.MustCompile(c.re).MatchString(got) {
				t.Fatalf("%s gave %q, which does not match %s", c.pattern, got, c.re)
			}
		}
		if err := checkPattern(c.pattern, patternFree); err != nil {
			t.Errorf("%s: %v", c.pattern, err)
		}
	}
	if got := numbered("E#####", 10007); got != "E10007" {
		t.Errorf("numbered: %s", got)
	}
	if got := numbered("N-###", 7); got != "N-007" {
		t.Errorf("numbered pads with zeros: %s", got)
	}
}

// The admin account is named by the profile, from the name of the owner: Philip Meholm.
func TestAdminName_Patterns(t *testing.T) {
	for pattern, want := range map[string]string{
		"a.{first}":          "a.philip",
		"adm.{first}":        "adm.philip",
		"{first}.a.{last}":   "philip.a.meholm",
		"{first}.{last}.adm": "philip.meholm.adm",
		"adm-{f}{last}":      "adm-pmeholm",
		"{First} (admin)":    "Philip (admin)",
		"{Last}, {First}":    "Meholm, Philip",
	} {
		if got := adminName(pattern, "Philip", "Meholm"); got != want {
			t.Errorf("%s: %s, want %s", pattern, got, want)
		}
	}
	if got := adminName("a.{first}", "Håkon", "Ødegård"); got != "a.hakon" {
		t.Errorf("a user principal name has no accents: %s", got)
	}
	if got := ascii("Bjørn Åse-Æble_ß"); got != "BjornAse-AEble_ss" {
		t.Errorf("ascii: %s", got)
	}
}

// A Norwegian world: everything comes from the profile (the domain, the offices, the departments, the names, the formats), the addresses
// have no accents but the names do, and the admin account is named as the profile says.
func TestProfile_NorwayMakesANorwegianTenant(t *testing.T) {
	p := norway(t)
	w := Generate(Options{Users: 300, Seed: 3, Profile: p})
	if len(w.Users) != 300 {
		t.Fatalf("%d users", len(w.Users))
	}
	depts, offices, langs := map[string]bool{}, map[string]bool{}, map[string]bool{"nb-NO": true, "en-US": true}
	for _, d := range p.Departments {
		depts[d.Name] = true
	}
	for _, o := range p.Offices {
		offices[o.Name] = true
	}
	ascii := regexp.MustCompile(`^[a-z0-9._-]+(#EXT#)?$`)
	mobile, empID := regexp.MustCompile(`^\+47 9\d{2} \d{2} \d{3}$`), regexp.MustCompile(`^\d{6}$`)
	accents := 0
	for _, u := range w.Users[:len(w.Users)-2] { // the owner and the admin account are checked below
		if !depts[str(u["department"])] || !offices[str(u["officeLocation"])] || !langs[str(u["preferredLanguage"])] {
			t.Fatalf("a user from outside the profile: %v", u)
		}
		local, domain, _ := strings.Cut(str(u["userPrincipalName"]), "@")
		if u["userType"] == "Member" && (domain != "fjordvik.example" || !ascii.MatchString(local)) {
			t.Fatalf("an address with an accent or the wrong domain: %v", u["userPrincipalName"])
		}
		if u["userType"] == "Guest" && (domain != "fjordvik.onmicrosoft.com" || !strings.Contains(local, "#EXT#")) {
			t.Fatalf("a guest: %v", u["userPrincipalName"])
		}
		if !mobile.MatchString(str(u["mobilePhone"])) || !empID.MatchString(str(u["employeeId"])) || u["companyName"] != "Fjordvik" {
			t.Fatalf("a format: %v %v %v", u["mobilePhone"], u["employeeId"], u["companyName"])
		}
		if strings.ContainsAny(str(u["displayName"]), "øåæØÅÆ") {
			accents++
		}
	}
	if accents == 0 {
		t.Error("the names have their accents: Håkon and Bjørn are not Hakon and Bjorn")
	}
	if str(w.Users[0]["employeeId"]) != "100001" {
		t.Errorf("the first employee id: %v", w.Users[0]["employeeId"])
	}

	owner, admin := w.Users[len(w.Users)-1], w.Users[len(w.Users)-2]
	if owner["displayName"] != "Philip Meholm" || owner["userPrincipalName"] != "philip.meholm@fjordvik.example" ||
		owner["department"] != "IT og digitalisering" || owner["jobTitle"] != "Identitetsarkitekt" {
		t.Errorf("the owner: %v", owner)
	}
	if admin["userPrincipalName"] != "adm.philip@fjordvik.example" || admin["displayName"] != "Philip (adm)" || admin["jobTitle"] != "Administrator" ||
		admin["mailNickname"] != "adm.philip" {
		t.Errorf("the admin account: %v", admin)
	}
	if w.AdminAccount() != "adm.philip@fjordvik.example" {
		t.Errorf("AdminAccount: %s", w.AdminAccount())
	}
	if got, ok := w.GetUser("adm.philip@fjordvik.example"); !ok || got["id"] != admin["id"] {
		t.Error("the admin account is found by its address")
	}

	org := w.ListOrganizations()[0]
	if org["displayName"] != "Fjordvik Energi AS (fake)" || org["city"] != "Oslo" || org["countryLetterCode"] != "NO" ||
		org["technicalNotificationMails"].([]any)[0] != "adm.philip@fjordvik.example" {
		t.Errorf("the organization: %v", org)
	}
	var names []string
	for _, d := range org["verifiedDomains"].([]any) {
		names = append(names, d.(Object)["name"].(string))
	}
	if !slices.Equal(names, []string{"fjordvik.onmicrosoft.com", "fjordvik.example"}) {
		t.Errorf("the domains: %v", names)
	}

	// the groups: the prefix, the departments of the profile, and the dynamic group
	prefixed := 0
	for _, g := range w.Groups {
		if strings.HasPrefix(g.Name, "sg-") {
			prefixed++
		}
	}
	if prefixed != len(w.Groups) || groupNamed(w, "sg-dyn.it-og-digitalisering") == nil || groupNamed(w, "sg-app.kundeportal.contributor") == nil {
		t.Errorf("%d groups, %d with the prefix", len(w.Groups), prefixed)
	}
	if groupNamed(w, "sg-dyn.it-og-digitalisering").Rule != `(user.department -eq "IT og digitalisering")` {
		t.Errorf("the rule: %s", groupNamed(w, "sg-dyn.it-og-digitalisering").Rule)
	}

	// the same profile and seed make the same world, and a world with no folder makes the same kind of data again at a reset
	if hashOf(t, w) != hashOf(t, Generate(Options{Users: 300, Seed: 3, Profile: norway(t)})) {
		t.Error("the same profile and seed must make the same world")
	}
	if err := w.Reset(Actor{Source: "admin"}); err != nil {
		t.Fatal(err)
	}
	if w.AdminAccount() != "adm.philip@fjordvik.example" || len(w.ListUsers()) != 300 || w.ListOrganizations()[0]["countryLetterCode"] != "NO" {
		t.Error("a reset must keep the profile of a world with no folder")
	}
}

// Without a profile the world is the default one, and its admin account is the one that the quick connect has always used.
func TestProfile_DefaultAdminAccount(t *testing.T) {
	w := Generate(Options{Users: 20, Seed: 1})
	if w.AdminAccount() != AdminUPN || w.Admin != "" {
		t.Errorf("%s %q", w.AdminAccount(), w.Admin)
	}
	if _, ok := w.GetUser(AdminUPN); !ok {
		t.Error("the default admin account is there")
	}
}

// A profile can ask for the admin accounts to be named in any of the usual ways, and the owner and the admin keep working with a
// profile that has other departments.
func TestProfile_AdminNamingFollowsTheProfile(t *testing.T) {
	for pattern, want := range map[string]string{"adm.{first}": "adm.philip@acme.example", "{first}.a.{last}": "philip.a.meholm@acme.example"} {
		p := DefaultProfile()
		p.Admin.UserPrincipalName = pattern
		w, err := GenerateChecked(Options{Users: 10, Seed: 1, Profile: p})
		if err != nil {
			t.Fatal(err)
		}
		if w.AdminAccount() != want {
			t.Errorf("%s: %s", pattern, w.AdminAccount())
		}
		if got, ok := w.GetUser(want); !ok || got["displayName"] != "Philip (admin)" {
			t.Errorf("%s: the account is %v", pattern, got["displayName"])
		}
	}
}

// What a profile would make, in memory: counts, the shape of the data, and a few people.
func TestProfile_Preview(t *testing.T) {
	pv, err := PreviewProfile(norway(t), 400, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Users != 400 || pv.Domain != "fjordvik.example" || pv.Owner != "philip.meholm@fjordvik.example" || pv.Admin != "adm.philip@fjordvik.example" ||
		len(pv.Offices) != 6 || len(pv.Departments) != 8 || len(pv.Sample) != 6 || pv.Guests == 0 || pv.Synced == 0 {
		t.Errorf("%+v", pv)
	}
	if pv.Languages[0].Value != "nb-NO" || pv.Sample[0]["mobilePhone"] == nil || pv.Sample[0]["employeeId"] == nil {
		t.Errorf("languages %v, a person %v", pv.Languages, pv.Sample[0])
	}
	if big, _ := PreviewProfile(DefaultProfile(), 50000, 1); big.Users != maxPreviewUsers {
		t.Errorf("a preview is made small: %d users", big.Users)
	}
	bad := DefaultProfile()
	bad.Names.First = nil
	if _, err := PreviewProfile(bad, 10, 1); err == nil || !strings.Contains(err.Error(), "names.first") {
		t.Errorf("a profile that is not valid: %v", err)
	}
}

// A running world gets new data from a profile: the new seed is in the folder, the history is gone, and a restart has the new data.
func TestProfile_RegenerateAFolder(t *testing.T) {
	dir := t.TempDir()
	w, err := Open(dir, OpenOptions{Users: 30, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.SetServicePrincipalFlag(w.ListServicePrincipals()[0]["appId"].(string), "hideCalls", true, Actor{Source: "admin"}); err != nil {
		t.Fatal(err)
	}
	w.Rename("department", "Sales", "Selling", Actor{Source: "admin"})
	if len(w.History()) == 0 {
		t.Fatal("a history to clear")
	}
	oldEpoch, _ := w.DeltaNow()

	st, err := w.Regenerate(Options{Users: 120, Seed: 9, Profile: norway(t)})
	if err != nil {
		t.Fatal(err)
	}
	if st.Users != 120 || len(w.History()) != 0 || w.AdminAccount() != "adm.philip@fjordvik.example" {
		t.Fatalf("%+v, history %d, admin %s", st, len(w.History()), w.AdminAccount())
	}
	if epoch, _ := w.DeltaNow(); epoch == oldEpoch {
		t.Error("the data is another epoch, so an old delta token is refused")
	}
	// a change after the new data is in the new history, which starts at 1
	w.Rename("department", "Drift", "Operations", Actor{Source: "admin"})
	if h := w.History(); len(h) != 1 || h[0].Seq != 1 {
		t.Errorf("the new history: %v", h)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := Open(dir, OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if len(again.ListUsers()) != 120 || again.AdminAccount() != "adm.philip@fjordvik.example" || again.ListOrganizations()[0]["displayName"] != "Fjordvik Energi AS (fake)" {
		t.Errorf("after a restart: %d users, admin %s", len(again.ListUsers()), again.AdminAccount())
	}
	found := false
	for _, u := range again.ListUsers() {
		found = found || u["department"] == "Operations"
	}
	if !found {
		t.Error("the change after the new data is replayed")
	}

	// a profile that is not valid changes nothing
	bad := DefaultProfile()
	bad.Offices = nil
	if _, err := again.Regenerate(Options{Users: 10, Profile: bad}); err == nil || len(again.ListUsers()) != 120 {
		t.Errorf("a bad profile: %v, %d users", err, len(again.ListUsers()))
	}
}

// A world with no folder is made again too.
func TestProfile_RegenerateInMemory(t *testing.T) {
	w := Generate(Options{Users: 30, Seed: 1})
	old, _ := w.DeltaNow()
	if _, err := w.Regenerate(Options{Users: 50, Seed: 2, Profile: norway(t)}); err != nil {
		t.Fatal(err)
	}
	if epoch, _ := w.DeltaNow(); epoch == old || len(w.ListUsers()) != 50 || w.AdminAccount() != "adm.philip@fjordvik.example" {
		t.Errorf("%d users, admin %s, epoch %s", len(w.ListUsers()), w.AdminAccount(), epoch)
	}
}

// More than one domain: the addresses of the members follow the weights, the default domain stays the one of the owner and the admin,
// and the guests have their own domains. A profile with one domain is not touched (the golden test holds that).
func TestProfile_ManyDomains(t *testing.T) {
	p := DefaultProfile()
	p.Organization.Domain, p.Organization.DomainWeight = "acme.example", 6
	p.Organization.Domains = []ProfileDomain{{"acme.com", 3}, {"acme.onmicrosoft.com", 1}}
	p.Organization.GuestDomain = ""
	p.People.GuestPercent = 30
	p.People.GuestDomains = []ProfileDomain{{"partner.example", 1}, {"supplier.example", 1}}
	w, err := GenerateChecked(Options{Users: 400, Seed: 2, Profile: p})
	if err != nil {
		t.Fatal(err)
	}
	members, guests := map[string]int{}, map[string]int{}
	for _, u := range w.Users {
		_, domain, _ := strings.Cut(str(u["userPrincipalName"]), "@")
		if u["userType"] == "Guest" {
			_, mail, _ := strings.Cut(str(u["mail"]), "@")
			guests[mail]++
			if domain != "acme.onmicrosoft.com" {
				t.Fatalf("a guest signs in at the initial domain: %s", u["userPrincipalName"])
			}
			continue
		}
		members[domain]++
	}
	if members["acme.example"] <= members["acme.com"] || members["acme.com"] <= members["acme.onmicrosoft.com"] || members["acme.onmicrosoft.com"] == 0 {
		t.Errorf("the weights 6, 3, and 1 give %v", members)
	}
	if guests["partner.example"] == 0 || guests["supplier.example"] == 0 || len(guests) != 2 {
		t.Errorf("guest domains: %v", guests)
	}
	if w.AdminAccount() != "a.philip@acme.example" {
		t.Errorf("the admin account is at the default domain: %s", w.AdminAccount())
	}
	var listed []string
	for _, d := range w.ListOrganizations()[0]["verifiedDomains"].([]any) {
		m := d.(Object)
		listed = append(listed, fmt.Sprintf("%s default=%v initial=%v", m["name"], m["isDefault"], m["isInitial"]))
	}
	want := []string{"acme.onmicrosoft.com default=false initial=true", "acme.example default=true initial=false", "acme.com default=false initial=false"}
	if !slices.Equal(listed, want) {
		t.Errorf("verifiedDomains: %v", listed)
	}

	// the default domain may be the initial one: one entry
	q := DefaultProfile()
	q.Organization.Domain = q.Organization.InitialDomain
	if org := newOrganization(q); len(org["verifiedDomains"].([]any)) != 1 || org["verifiedDomains"].([]any)[0].(Object)["isDefault"] != true {
		t.Errorf("one domain that is both: %v", org["verifiedDomains"])
	}
	for name, change := range map[string]func(p *Profile){
		"a domain twice":         func(p *Profile) { p.Organization.Domains = []ProfileDomain{{p.Organization.Domain, 1}} },
		"a domain that is bad":   func(p *Profile) { p.Organization.Domains = []ProfileDomain{{"no good", 1}} },
		"a guest domain twice":   func(p *Profile) { p.People.GuestDomains = []ProfileDomain{{"a.example", 1}, {"a.example", 1}} },
		"a weight that is big":   func(p *Profile) { p.Organization.DomainWeight = 5000 },
		"no guest domain at all": func(p *Profile) { p.Organization.GuestDomain = "" },
	} {
		bad := DefaultProfile()
		change(bad)
		if _, err := GenerateChecked(Options{Users: 5, Profile: bad}); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}

// Names follow the languages: with 85 and 15 for Norwegian and English, about 85 of 100 people have a Norwegian name and a Norwegian language.
func TestProfile_NamesFollowTheLanguages(t *testing.T) {
	p := DefaultProfile()
	p.People.GuestPercent = 0
	p.People.Languages = []ProfileLanguage{{"nb-NO", 85}, {"en-GB", 15}}
	p.Names = ProfileNames{ByLanguage: map[string]ProfileNameSet{
		"nb-NO": {First: []string{"Ola", "Kari", "Nils"}, Last: []string{"Hansen", "Berg"}},
		"en-GB": {First: []string{"Oliver", "Amelia"}, Last: []string{"Smith", "Jones", "Taylor"}},
	}}
	w, err := GenerateChecked(Options{Users: 800, Seed: 5, Profile: p})
	if err != nil {
		t.Fatal(err)
	}
	norwegian := 0
	for _, u := range w.Users[:len(w.Users)-2] { // the owner and the admin account are not drawn
		no := slices.Contains([]string{"Ola", "Kari", "Nils"}, str(u["givenName"]))
		en := slices.Contains([]string{"Oliver", "Amelia"}, str(u["givenName"]))
		if no == en || (no && u["preferredLanguage"] != "nb-NO") || (en && u["preferredLanguage"] != "en-GB") {
			t.Fatalf("a name and a language that do not go together: %v %v", u["givenName"], u["preferredLanguage"])
		}
		if no {
			norwegian++
		}
	}
	if share := float64(norwegian) / 798; share < 0.80 || share > 0.90 {
		t.Errorf("%.0f%% have a Norwegian name, want about 85%%", share*100)
	}

	for name, change := range map[string]func(p *Profile){
		"a language that is not spoken": func(p *Profile) {
			p.Names.ByLanguage["fr-FR"] = ProfileNameSet{First: []string{"A"}, Last: []string{"B"}}
		},
		"a language with no names":        func(p *Profile) { delete(p.Names.ByLanguage, "en-GB") },
		"a list with nothing in it":       func(p *Profile) { p.Names.ByLanguage["nb-NO"] = ProfileNameSet{Last: []string{"Berg"}} },
		"a name with a control character": func(p *Profile) { p.Names.ByLanguage["nb-NO"].First[0] = "O\nla" },
	} {
		q := *p
		q.Names.ByLanguage = map[string]ProfileNameSet{}
		for k, v := range p.Names.ByLanguage {
			q.Names.ByLanguage[k] = ProfileNameSet{First: slices.Clone(v.First), Last: slices.Clone(v.Last)}
		}
		change(&q)
		if _, err := GenerateChecked(Options{Users: 5, Profile: &q}); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	// a language with no list of its own uses the general lists
	p.Names.First, p.Names.Last = []string{"Zoe"}, []string{"Zed"}
	delete(p.Names.ByLanguage, "en-GB")
	if w, err := GenerateChecked(Options{Users: 40, Seed: 5, Profile: p}); err != nil {
		t.Fatal(err)
	} else if !slices.ContainsFunc(w.Users, func(u Object) bool { return u["givenName"] == "Zoe" }) {
		t.Error("the general names are used for a language with no list")
	}
}

// The seed keeps the profile, so a restart, a reset, and a new generation with no profile make the same kind of data.
func TestProfile_TheSeedKeepsItsProfile(t *testing.T) {
	dir := t.TempDir()
	w, err := Open(dir, OpenOptions{Users: 40, Seed: 3, Profile: norway(t)})
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	again, err := Open(dir, OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if p := again.CurrentProfile(); p == nil || p.Organization.Domain != "fjordvik.example" {
		t.Fatalf("the profile after a restart: %+v", p)
	}
	if err := again.Reset(Actor{Source: "admin"}); err != nil || again.AdminAccount() != "adm.philip@fjordvik.example" {
		t.Errorf("after a reset: %v %s", err, again.AdminAccount())
	}
	if _, err := again.Regenerate(Options{Users: 60, Seed: 8}); err != nil { // no profile: the stored one
		t.Fatal(err)
	}
	if len(again.ListUsers()) != 60 || again.AdminAccount() != "adm.philip@fjordvik.example" || again.CurrentProfile() == nil {
		t.Errorf("regenerate with no profile: %d users, admin %s", len(again.ListUsers()), again.AdminAccount())
	}
	// the default profile is not kept
	plain := Generate(Options{Users: 10})
	if plain.CurrentProfile() != nil {
		t.Error("data from the default profile keeps none")
	}
}

// A folder that has data leaves a profile out (and says so): the profile makes a new seed only.
func TestProfile_AFolderWithDataKeepsItsData(t *testing.T) {
	dir := t.TempDir()
	w, err := Open(dir, OpenOptions{Users: 20, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	again, err := Open(dir, OpenOptions{Profile: norway(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if again.AdminAccount() != AdminUPN || len(again.ListUsers()) != 20 {
		t.Errorf("the data that was there stays: %s, %d users", again.AdminAccount(), len(again.ListUsers()))
	}
}
