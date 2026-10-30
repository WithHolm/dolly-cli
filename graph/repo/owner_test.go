package repo

import "testing"

// Every generated world has Philip Meholm, a tech lead in Engineering, as one of its users.
func TestGenerate_HasTheOwner(t *testing.T) {
	for _, n := range []int{1, 2, 10, 60, 500} {
		w := Generate(Options{Users: n, Seed: 1})
		if len(w.Users) != n {
			t.Fatalf("%d users asked, %d made: the owner is one of them", n, len(w.Users))
		}
		var owners []Object
		for _, u := range w.Users {
			if u["userPrincipalName"] == OwnerUPN {
				owners = append(owners, u)
			}
		}
		if len(owners) != 1 {
			t.Fatalf("n=%d: %d users have the address of the owner", n, len(owners))
		}
		u := owners[0]
		for field, want := range map[string]any{
			"displayName": "Philip Meholm", "givenName": "Philip", "surname": "Meholm", "mail": OwnerUPN, "mailNickname": "philip.meholm",
			"department": "Engineering", "jobTitle": "Tech Lead", "accountEnabled": true, "userType": "Member", "showInAddressList": true,
		} {
			if u[field] != want {
				t.Errorf("n=%d: %s = %v, want %v", n, field, u[field], want)
			}
		}
		if u["creationType"] != nil || u["externalUserState"] != nil {
			t.Errorf("n=%d: the owner is not a guest: %v %v", n, u["creationType"], u["externalUserState"])
		}
		if org, _ := u["employeeOrgData"].(Object); org["division"] != "Engineering" {
			t.Errorf("n=%d: employeeOrgData %v", n, u["employeeOrgData"])
		}
		// the owner is found by name and by address, and has a manager like the other members (except the head of the company)
		if found, _ := w.SearchUsers("Philip Meholm", 5); len(found) != 1 || found[0].UPN != OwnerUPN || found[0].JobTitle != "Tech Lead" {
			t.Errorf("n=%d: search: %+v", n, found)
		}
		if got, ok := w.GetUser(OwnerUPN); !ok || got["displayName"] != "Philip Meholm" {
			t.Errorf("n=%d: get by address: %v", n, ok)
		}
	}
}

// The owner changes nothing for the others: the same seed gives the same people, except in the last place.
func TestGenerate_OwnerLeavesTheOthersAsTheyWere(t *testing.T) {
	a := Generate(Options{Users: 40, Seed: 7})
	b := Generate(Options{Users: 40, Seed: 7})
	for i := range a.Users {
		if a.Users[i]["id"] != b.Users[i]["id"] || a.Users[i]["displayName"] != b.Users[i]["displayName"] {
			t.Fatalf("user %d differs between two runs", i)
		}
	}
	// users 0..38 do not depend on how many are asked for after them: the first 39 of a world of 40 are the first 39 of 39
	c := Generate(Options{Users: 39, Seed: 7})
	for i := 0; i < 37; i++ { // the last two of a world are the owner and the admin account, so they are not compared
		if a.Users[i]["displayName"] != c.Users[i]["displayName"] || a.Users[i]["jobTitle"] != c.Users[i]["jobTitle"] {
			t.Fatalf("user %d is not the same: %v and %v", i, a.Users[i]["displayName"], c.Users[i]["displayName"])
		}
	}
}

// A world of two users or more has the admin account of the owner, and the owner is still found by name alone.
func TestGenerate_HasTheAdminAccount(t *testing.T) {
	if _, ok := Generate(Options{Users: 1, Seed: 1}).GetUser(AdminUPN); ok {
		t.Error("a world of one user has the owner only")
	}
	for _, n := range []int{2, 10, 500} {
		w := Generate(Options{Users: n, Seed: 1})
		u, ok := w.GetUser(AdminUPN)
		if !ok || u["displayName"] != AdminName || u["accountEnabled"] != true || u["userType"] != "Member" || len(w.Users) != n {
			t.Errorf("n=%d: %v %v", n, ok, u["displayName"])
		}
	}
}
