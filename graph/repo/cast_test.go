package repo

import (
	"strings"
	"testing"
)

// The rest of the cast of A Doll's House, besides Nora Helmer: Torvald Helmer is her boss, Nils Krogstad is a colleague with a disabled account, and Dr. Rank is a
// guest. They are made after the users and before the managers and the groups, so the departments, the manager tree, and the groups agree with them.
func userBy(w *World, upn string) Object {
	for _, u := range w.Users {
		if strings.EqualFold(str(u["userPrincipalName"]), upn) {
			return u
		}
	}
	return nil
}

func managerOf(w *World, u Object) string { return w.Managers[strings.ToLower(str(u["id"]))] }

func TestCast_EveryWorldWithRoomHasThem(t *testing.T) {
	for _, c := range []struct {
		users int
		seed  int64
		prof  func() *Profile
	}{{6, 1, DefaultProfile}, {10, 7, DefaultProfile}, {60, 1, DefaultProfile}, {500, 1, DefaultProfile}, {6, 1, AcmeProfile}, {200, 5, AcmeProfile}} {
		p := c.prof()
		w := Generate(Options{Users: c.users, Seed: c.seed, Profile: p})
		dom := p.Organization.Domain
		nora, torvald, nils := userBy(w, "nora.helmer@"+dom), userBy(w, "torvald.helmer@"+dom), userBy(w, "nils.krogstad@"+dom)
		var rank Object
		for _, u := range w.Users {
			if u["displayName"] == "Dr. Rank" {
				rank = u
			}
		}
		if len(w.Users) != c.users || nora == nil || torvald == nil || nils == nil || rank == nil {
			t.Fatalf("%s users=%d: %d users, nora %v torvald %v nils %v rank %v", p.Name, c.users, len(w.Users), nora != nil, torvald != nil, nils != nil, rank != nil)
		}

		// Torvald is Nora's boss, and so is Nils's. Nora and Nils are colleagues: the same department.
		if managerOf(w, nora) != strings.ToLower(str(torvald["id"])) || managerOf(w, nils) != strings.ToLower(str(torvald["id"])) {
			t.Errorf("%s: Torvald is the manager of Nora and Nils: %q %q", p.Name, managerOf(w, nora), managerOf(w, nils))
		}
		if nora["department"] != torvald["department"] || nora["department"] != nils["department"] {
			t.Errorf("%s: the same department: %v %v %v", p.Name, nora["department"], torvald["department"], nils["department"])
		}
		for _, u := range []Object{torvald, nora} {
			if u["accountEnabled"] != true || u["userType"] != "Member" {
				t.Errorf("%s: %v is an enabled member: %v %v", p.Name, u["displayName"], u["accountEnabled"], u["userType"])
			}
		}
		if nils["accountEnabled"] != false || nils["userType"] != "Member" {
			t.Errorf("%s: Nils has a disabled account and is a member: %v %v", p.Name, nils["accountEnabled"], nils["userType"])
		}
		for k, want := range map[string]any{"givenName": "Torvald", "surname": "Helmer", "mailNickname": "torvald.helmer"} {
			if torvald[k] != want {
				t.Errorf("%s: Torvald %s = %v", p.Name, k, torvald[k])
			}
		}
		if nils["givenName"] != "Nils" || nils["surname"] != "Krogstad" || nils["mail"] != "nils.krogstad@"+dom {
			t.Errorf("%s: Nils: %v %v %v", p.Name, nils["givenName"], nils["surname"], nils["mail"])
		}

		// Dr. Rank is a guest as a generated guest is: invited, outside the tenant's domain, not in the address list, not synced, and with no manager
		guestDomain := p.guestDomains()[0].Name
		if rank["userType"] != "Guest" || rank["creationType"] != "Invitation" || rank["externalUserState"] != "Accepted" || rank["showInAddressList"] != false ||
			rank["accountEnabled"] != true || rank["onPremisesSyncEnabled"] != nil {
			t.Errorf("%s: Dr. Rank is an invited guest: %v %v %v %v", p.Name, rank["userType"], rank["creationType"], rank["externalUserState"], rank["showInAddressList"])
		}
		if want := "dr.rank_" + guestDomain + "#EXT#@" + p.Organization.InitialDomain; rank["userPrincipalName"] != want || rank["mail"] != "dr.rank@"+guestDomain {
			t.Errorf("%s: the address of the guest: %v %v, want %s", p.Name, rank["userPrincipalName"], rank["mail"], want)
		}
		if _, has := rank["onPremisesDistinguishedName"]; has {
			t.Errorf("%s: a guest has no on-premises name", p.Name)
		}
		if managerOf(w, rank) != "" {
			t.Errorf("%s: a guest has no manager", p.Name)
		}

		// the manager tree has no loop, and everyone is found once
		for _, u := range w.Users {
			seen, cur := map[string]bool{}, strings.ToLower(str(u["id"]))
			for cur != "" {
				if seen[cur] {
					t.Fatalf("%s users=%d: a loop in the managers, at %v", p.Name, c.users, u["displayName"])
				}
				seen[cur] = true
				cur = w.Managers[cur]
			}
		}
		upns := map[string]bool{}
		for _, u := range w.Users {
			k := strings.ToLower(str(u["userPrincipalName"]))
			if upns[k] {
				t.Fatalf("%s users=%d: the address %s is twice", p.Name, c.users, k)
			}
			upns[k] = true
		}

		// the groups agree with the departments: the department group of Nora has her, Torvald, and Nils, and nobody of another department
		var found bool
		for _, g := range w.Groups {
			if g.Name != p.Groups.Prefix+"dept."+slug(str(nora["department"])) {
				continue
			}
			found = true
			for _, id := range []string{str(nora["id"]), str(torvald["id"]), str(nils["id"])} {
				if !g.has(id) {
					t.Errorf("%s: the department group %s lacks a user of the department", p.Name, g.Name)
				}
			}
			for _, id := range g.Members {
				if u := w.users[strings.ToLower(id)]; u != nil && u["department"] != nora["department"] {
					t.Errorf("%s: %v is in the group %s, and is in %v", p.Name, u["displayName"], g.Name, u["department"])
				}
			}
		}
		if p.Groups.DepartmentGroups && !found {
			t.Errorf("%s: no department group for %v", p.Name, nora["department"])
		}
	}
}

// Whatever the seed and the size: Torvald is the boss, the tree has no loop, nobody has two addresses, and the count of users is as asked.
func TestCast_ManySeedsAndSizes(t *testing.T) {
	for _, prof := range []func() *Profile{DefaultProfile, AcmeProfile} {
		p := prof()
		dom := p.Organization.Domain
		for seed := int64(1); seed <= 40; seed++ {
			for _, n := range []int{6, 7, 9, 15, 40} {
				w := Generate(Options{Users: n, Seed: seed, Profile: prof()})
				nora, torvald, nils := userBy(w, "nora.helmer@"+dom), userBy(w, "torvald.helmer@"+dom), userBy(w, "nils.krogstad@"+dom)
				if len(w.Users) != n || nora == nil || torvald == nil || nils == nil {
					t.Fatalf("%s seed=%d n=%d: the cast is not all there", p.Name, seed, n)
				}
				if managerOf(w, nora) != strings.ToLower(str(torvald["id"])) || managerOf(w, nils) != strings.ToLower(str(torvald["id"])) || nils["accountEnabled"] != false {
					t.Fatalf("%s seed=%d n=%d: Torvald is the boss of Nora and Nils, and Nils is disabled", p.Name, seed, n)
				}
				upns := map[string]bool{}
				for _, u := range w.Users {
					k := strings.ToLower(str(u["userPrincipalName"]))
					if upns[k] {
						t.Fatalf("%s seed=%d n=%d: the address %s is twice", p.Name, seed, n, k)
					}
					upns[k] = true
					seen := map[string]bool{}
					for cur := strings.ToLower(str(u["id"])); cur != ""; cur = w.Managers[cur] {
						if seen[cur] {
							t.Fatalf("%s seed=%d n=%d: a loop in the managers at %v", p.Name, seed, n, u["displayName"])
						}
						seen[cur] = true
					}
				}
			}
		}
	}
}

// With less room, the cast is the first of them: Nora needs 3 users, Torvald 4, Nils 5, and Dr. Rank 6. The count of users is as asked.
func TestCast_NeedsRoom(t *testing.T) {
	for n, want := range map[int]int{3: 1, 4: 2, 5: 3, 6: 4} {
		w := Generate(Options{Users: n, Seed: 1})
		got := 0
		for _, u := range w.Users {
			switch u["displayName"] {
			case NoraName, "Torvald Helmer", "Nils Krogstad", "Dr. Rank":
				got++
			}
		}
		if len(w.Users) != n || got != want {
			t.Errorf("%d users: %d made, %d of the cast, want %d", n, len(w.Users), got, want)
		}
	}
}
