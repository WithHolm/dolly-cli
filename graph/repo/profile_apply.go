package repo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// What a person does with a profile before and when it is used: look at what it would make (PreviewProfile), and make a new world from it in
// place of the one that a running dolly has (World.Regenerate).

// ProfilePreview is what a profile would make. It is made in memory, so nothing changes.
type ProfilePreview struct {
	State                   // the counts, and the departments, the job titles, and the offices
	Languages  []ValueCount `json:"languages"`
	Guests     int          `json:"guests"`
	Synced     int          `json:"synced"`
	Disabled   int          `json:"disabled"`
	Domain     string       `json:"domain"`
	Owner      string       `json:"owner"`        // the user principal name of the owner
	Admin      string       `json:"adminAccount"` // and of the admin account of the owner ("" in a world of one user)
	Sample     []Object     `json:"sample"`       // a few people, with the properties that the profile shapes
	Extensions []string     `json:"extensionAttributes,omitempty"`
}

// maxPreviewUsers keeps a preview quick. The counts are the ones of the size that was asked, up to this many.
const maxPreviewUsers = 2000

// PreviewProfile makes a world from the profile in memory, and says what it has. The profile is checked first.
func PreviewProfile(p *Profile, users int, seed int64) (*ProfilePreview, error) {
	if users <= 0 {
		users = 200
	}
	w, err := GenerateChecked(Options{Users: min(users, maxPreviewUsers), Seed: seed, Profile: p})
	if err != nil {
		return nil, err
	}
	pv := &ProfilePreview{State: w.Snapshot(), Domain: p.Organization.Domain, Owner: "philip.meholm@" + p.Organization.Domain}
	if len(w.Users) >= 2 {
		pv.Admin = adminUPN(p)
	}
	langs := map[string]int{}
	for i, u := range w.Users {
		langs[str(u["preferredLanguage"])]++
		switch {
		case u["userType"] == "Guest":
			pv.Guests++
		case u["onPremisesSyncEnabled"] == true:
			pv.Synced++
		}
		if u["accountEnabled"] != true {
			pv.Disabled++
		}
		if i < 6 && len(pv.Sample) < 6 {
			ext := Object{}
			for k, v := range u["onPremisesExtensionAttributes"].(Object) {
				if v != nil {
					ext[k] = v
				}
			}
			pv.Sample = append(pv.Sample, Object{
				"displayName": u["displayName"], "userPrincipalName": u["userPrincipalName"], "department": u["department"],
				"jobTitle": u["jobTitle"], "officeLocation": u["officeLocation"], "employeeId": u["employeeId"],
				"mobilePhone": u["mobilePhone"], "businessPhone": firstOf(u["businessPhones"]), "preferredLanguage": u["preferredLanguage"],
				"userType": u["userType"], "extensionAttributes": ext,
			})
		}
	}
	for code, n := range langs {
		pv.Languages = append(pv.Languages, ValueCount{Value: code, Count: n})
	}
	sort.Slice(pv.Languages, func(i, j int) bool {
		return pv.Languages[i].Count > pv.Languages[j].Count || pv.Languages[i].Count == pv.Languages[j].Count && pv.Languages[i].Value < pv.Languages[j].Value
	})
	for _, e := range p.Extension {
		pv.Extensions = append(pv.Extensions, fmt.Sprintf("extensionAttribute%d", e.Index))
	}
	sort.Strings(pv.Extensions)
	return pv, nil
}

func firstOf(v any) any {
	if list, ok := v.([]any); ok && len(list) > 0 {
		return list[0]
	}
	return nil
}

// Regenerate makes a new world from the options, in place of the data of this world: a new seed, and no history. It is what
// `dolly graph generate --force` does to a folder, done to a world that is running. The old data and its history are gone, and a rollback
// cannot bring them back. A token of delta query from before is refused (the data is another epoch). The profile is checked first.
func (w *World) Regenerate(o Options) (State, error) {
	if o.Profile == nil { // with no profile the data is made again from the one it has
		w.mu.RLock()
		o.Profile = w.Profile
		w.mu.RUnlock()
	}
	fresh, err := GenerateChecked(o)
	if err != nil {
		return State{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.j != nil {
		baseline, err := json.MarshalIndent(fresh, "", "  ")
		if err != nil {
			return State{}, err
		}
		seedPath := filepath.Join(w.j.dir, seedFile)
		if err := os.WriteFile(seedPath, baseline, 0o644); err != nil {
			return State{}, fmt.Errorf("dolly: writing %q: %w", seedPath, err)
		}
		w.j.mu.Lock()
		defer w.j.mu.Unlock()
		if err := w.j.f.Truncate(0); err != nil { // the file is open for append, so the next change goes to the start
			return State{}, fmt.Errorf("dolly: emptying the state file: %w", err)
		}
		w.j.events, w.j.baseline = nil, baseline
		w.delta.restart(seedBase(baseline), 0, 0)
	} else {
		w.delta = changeLog{} // a new epoch
	}
	w.replaceWith(fresh)
	return w.snapshotLocked(), nil
}

// CurrentProfile is a copy of the profile that the data was made from. It is nil for data that was made with no profile.
func (w *World) CurrentProfile() *Profile {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.Profile == nil {
		return nil
	}
	return w.Profile.Clone()
}

// ProfileName says in a line what the world was made from. A world that was read from a seed file does not know (it has the data, and
// no profile), so it gives "".
func (w *World) ProfileName() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.Profile == nil {
		return ""
	}
	return strings.TrimSpace(w.Profile.Name)
}
