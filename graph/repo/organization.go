package repo

import "strings"

// The tenant: the organization that Graph tells about in /organization. A tool asks it
// for the name of the tenant it is signed in to. A tenant has exactly one organization.
// The data side of api/organization.go. It satisfies api.OrganizationBackend.

// FakeTenantID is the ID of the fake tenant. It is all zeros, so nobody takes it for the ID of a
// real tenant, and it is the same in every dolly.
const FakeTenantID = "00000000-0000-0000-0000-000000000000"

// newOrganization makes the fake tenant. It is part of the seed.
func newOrganization() Object {
	return Object{
		"id":                    FakeTenantID,
		"displayName":           "Acme Corporation (fake)",
		"tenantType":            "AAD",
		"city":                  "Stockholm",
		"country":               "Sweden",
		"countryLetterCode":     "SE",
		"preferredLanguage":     "en",
		"createdDateTime":       seedEpoch.Format("2006-01-02T15:04:05Z"),
		"onPremisesSyncEnabled": nil,
		"verifiedDomains": []any{
			Object{"name": "acme.onmicrosoft.com", "type": "Managed", "isDefault": false, "isInitial": true, "capabilities": "Email, OfficeCommunicationsOnline"},
			Object{"name": "acme.example", "type": "Managed", "isDefault": true, "isInitial": false, "capabilities": "Email, OfficeCommunicationsOnline"},
		},
	}
}

// ensureOrganization gives a seed that has no organization (a seed from before there was one)
// the fake tenant.
func (w *World) ensureOrganization() {
	if w.Organization == nil {
		w.Organization = newOrganization()
	}
}

// ListOrganizations gives the organizations of the tenant: one.
func (w *World) ListOrganizations() []Object {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return []Object{deepCopy(w.Organization)}
}

// GetOrganization gives the organization with this ID.
func (w *World) GetOrganization(id string) (Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if !strings.EqualFold(str(w.Organization["id"]), id) {
		return nil, false
	}
	return deepCopy(w.Organization), true
}
