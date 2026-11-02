package repo

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Every profile file in the program is a valid profile, and makes the same data twice. (They are the ones that people start from: the default, the
// English example in the wizard, and the old default that old seeds are read as.)
func TestProfiles_EveryFileIsValidAndStable(t *testing.T) {
	files, err := filepath.Glob("profiles/*.json")
	if err != nil || len(files) < 3 {
		t.Fatalf("the profile files: %v %v", files, err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p, err := ParseProfile(strings.NewReader(string(data)))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		a := Generate(Options{Users: 150, Seed: 11, Profile: p})
		b := Generate(Options{Users: 150, Seed: 11, Profile: p.Clone()})
		if hashOf(t, a) != hashOf(t, b) || len(a.Users) != 150 {
			t.Errorf("%s: the same seed must make the same data, and as many users as asked", f)
		}
		var nora int
		for _, u := range a.Users {
			if u["displayName"] == NoraName {
				nora++
			}
		}
		if nora != 1 {
			t.Errorf("%s: %d users called %s", f, nora, NoraName)
		}
	}
}

// The English example is for people who are not Norwegian: English-speaking offices and names, the domain acme.example, the owner as Tech Lead in Engineering, and
// admin accounts named a.<first name>.
func TestProfiles_AcmeEnglish(t *testing.T) {
	f, err := os.Open("profiles/acme-english.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p, err := ParseProfile(f)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Acme (English)" || p.Organization.Domain != "acme.example" || len(p.Offices) != 6 || len(p.Departments) < 8 {
		t.Fatalf("the profile: %q %s %d offices", p.Name, p.Organization.Domain, len(p.Offices))
	}
	for _, o := range p.Offices {
		if !slices.Contains([]string{"US", "GB", "IE", "CA", "AU"}, o.Usage) {
			t.Errorf("the office %s is in %s: an English-speaking country is expected", o.Name, o.Usage)
		}
	}
	w := Generate(Options{Users: 400, Seed: 3, Profile: p})
	plain := regexp.MustCompile(`^[a-z0-9._-]+(#EXT#)?$`)
	for _, u := range w.Users {
		local, _, _ := strings.Cut(str(u["userPrincipalName"]), "@")
		if !plain.MatchString(local) && !strings.Contains(local, "#EXT#") {
			t.Fatalf("an address with a character that is not plain: %v", u["userPrincipalName"])
		}
		if lang := str(u["preferredLanguage"]); !strings.HasPrefix(lang, "en-") {
			t.Fatalf("%v speaks %q: this profile is English", u["displayName"], lang)
		}
		for _, name := range []string{str(u["givenName"]), str(u["surname"])} {
			for _, r := range name {
				if r > 127 {
					t.Fatalf("%v has a name with a letter that is not plain: %q", u["displayName"], name)
				}
			}
		}
	}
	if owner, ok := w.GetUser("philip.meholm@acme.example"); !ok || owner["jobTitle"] != "Tech Lead" || owner["department"] != "Engineering" {
		t.Errorf("the owner: %v %v", ok, owner["jobTitle"])
	}
	if w.AdminAccount() != "a.philip@acme.example" {
		t.Errorf("the admin account: %s", w.AdminAccount())
	}
	if nora, ok := w.GetUser("nora.helmer@acme.example"); !ok || nora["displayName"] != NoraName {
		t.Errorf("Nora Helmer: %v", ok)
	}
	// the groups that Acme has: department groups and the three demo apps
	if len(p.Groups.Apps) != 3 || p.Groups.Apps[0].Title != "Acme Portal" {
		t.Errorf("the demo apps: %+v", p.Groups.Apps)
	}
}
