package permissions

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// raw/names.json is a newer list of permissions than raw/permissions-descriptions.json: more names, with their texts, and no IDs (see fetch). A
// permission that only this list has gets an ID that dollycli makes from its kind and name: the same one every time. People pick permissions by name, so
// the ID is only the key inside dollycli. It is not Microsoft's ID, and a script that carries the real ID of such a permission will not find it.

//go:embed raw/names.json
var namesData []byte

type nameScheme struct {
	AdminDisplayName     string `json:"adminDisplayName"`
	AdminDescription     string `json:"adminDescription"`
	UserDisplayName      string `json:"userDisplayName"`
	UserDescription      string `json:"userDescription"`
	RequiresAdminConsent bool   `json:"requiresAdminConsent"`
}

// MadeID gives the ID that dollycli makes for a permission that has none: the same text always gives the same ID.
func MadeID(k Kind, value string) string {
	b := sha256.Sum256([]byte("dollycli permission|" + string(k) + "|" + strings.ToLower(value)))
	b[6] = b[6]&0x0f | 0x50
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// addNames adds the permissions of names.json that the catalog does not have, with an ID that dollycli makes. DelegatedPersonal (a personal
// Microsoft account) is left out: a tenant of dollycli has no such account.
func (c *Catalog) addNames() error {
	var f struct {
		Permissions map[string]struct {
			Schemes map[string]nameScheme `json:"schemes"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(namesData, &f); err != nil {
		return fmt.Errorf("permissions: names.json: %w", err)
	}
	for value, p := range f.Permissions {
		for scheme, s := range p.Schemes {
			var k Kind
			switch scheme {
			case "DelegatedWork":
				k = Delegated
			case "Application":
				k = Application
			default:
				continue
			}
			if _, ok := c.Find(k, value); ok {
				continue
			}
			perm := Permission{
				ID: MadeID(k, value), IDMade: true, Value: value, IsAdmin: s.RequiresAdminConsent, IsEnabled: true,
				AdminConsentDisplayName: s.AdminDisplayName, AdminConsentDescription: s.AdminDescription,
				ConsentDisplayName: s.UserDisplayName, ConsentDescription: s.UserDescription,
			}
			if k == Application {
				perm.AllowedMemberTypes = []string{"Application"}
				c.Application = append(c.Application, perm)
			} else {
				c.Delegated = append(c.Delegated, perm)
			}
			c.Named++
		}
	}
	return c.index()
}
