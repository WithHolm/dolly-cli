package repo

import "testing"

// Every generated tenant with room for her has Nora Helmer (a Doll's House: the name of this tool). She is made like the owner, from a generated user,
// so the count of users is as asked and no other user changes.
func TestSeed_EveryWorldHasNoraHelmer(t *testing.T) {
	for _, c := range []struct {
		users int
		seed  int64
	}{{3, 1}, {5, 2}, {10, 7}, {60, 1}, {200, 5}, {500, 1}} {
		w := Generate(Options{Users: c.users, Seed: c.seed})
		var found []Object
		for _, u := range w.Users {
			if u["displayName"] == NoraName {
				found = append(found, u)
			}
		}
		if len(w.Users) != c.users || len(found) != 1 {
			t.Fatalf("users=%d seed=%d: %d users, %d called %s", c.users, c.seed, len(w.Users), len(found), NoraName)
		}
		n := found[0]
		upn := "nora.helmer@" + DefaultProfile().Organization.Domain
		for k, want := range map[string]any{
			"givenName": "Nora", "surname": "Helmer", "userPrincipalName": upn, "mail": upn, "mailNickname": "nora.helmer",
			"accountEnabled": true, "userType": "Member", "externalUserState": nil, "creationType": nil,
		} {
			if n[k] != want {
				t.Errorf("users=%d: %s is %v, want %v", c.users, k, n[k], want)
			}
		}
		if n["department"] == nil || n["jobTitle"] == nil {
			t.Errorf("she keeps the department and the job title that the seed gave the user: %v %v", n["department"], n["jobTitle"])
		}
		if got, ok := w.GetUser(upn); !ok || got["id"] != n["id"] {
			t.Errorf("users=%d: found by her address: %v %v", c.users, ok, got["id"])
		}
		if found, _ := w.SearchUsers("Nora Helmer", 5); len(found) != 1 || found[0].UPN != upn {
			t.Errorf("users=%d: the search finds her once: %v", c.users, found)
		}
		// she is not the owner, and not the admin account of the owner
		if upn == OwnerUPN || n["displayName"] == OwnerName {
			t.Error("she is another person")
		}
	}
}

// A world with no room for her (fewer than three users) is as many users as asked: the owner, and the admin account when there is room.
func TestSeed_NoraNeedsRoom(t *testing.T) {
	for _, n := range []int{1, 2} {
		w := Generate(Options{Users: n, Seed: 1})
		if len(w.Users) != n {
			t.Errorf("%d users asked, %d made", n, len(w.Users))
		}
		for _, u := range w.Users {
			if u["displayName"] == NoraName {
				t.Errorf("%d users: no room for her, and she is there", n)
			}
		}
	}
}
