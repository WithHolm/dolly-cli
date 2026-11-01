package repo

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// The tenant: the organization that Graph tells about in /organization. A tool asks it
// for the name of the tenant it is signed in to. A tenant has exactly one organization.
// The data side of api/organization.go. It satisfies api.OrganizationBackend.

// FakeTenantID is the ID of the fake tenant. It is all zeros, so nobody takes it for the ID of a
// real tenant, and it is the same in every dolly.
const FakeTenantID = "00000000-0000-0000-0000-000000000000"

// directoryQuota is the size of the directory that the fake tenant may have: the number of objects. Entra gives 50000 to a tenant of
// the kind that most have.
const directoryQuota = 50000

// newOrganization makes the fake tenant. It is part of the seed. It has every property of the organization resource
// (see https://learn.microsoft.com/graph/api/resources/organization), as the real Graph returns them, except
// directorySizeQuota, which is worked out when the organization is read (see orgView).
//
// The plans are real service plans of Microsoft 365 (the names and the IDs are the ones of Microsoft), so a tool that looks for a
// plan by ID finds it. The dates are fixed, so a seed always has the same data.
func newOrganization(p *Profile) Object {
	o := p.Organization
	assigned := func(service, id string) Object {
		return Object{"assignedDateTime": seedEpoch.Format("2006-01-02T15:04:05Z"), "capabilityStatus": "Enabled", "service": service, "servicePlanId": id}
	}
	provisioned := func(service string) Object {
		return Object{"capabilityStatus": "Enabled", "provisioningStatus": "Success", "service": service}
	}
	return Object{
		"id":                   FakeTenantID,
		"deletedDateTime":      nil,
		"businessPhones":       []any{o.Phone},
		"city":                 o.City,
		"country":              o.Country,
		"countryLetterCode":    o.CountryLetterCode,
		"createdDateTime":      seedEpoch.Format("2006-01-02T15:04:05Z"),
		"defaultUsageLocation": o.UsageLocation,
		"displayName":          o.DisplayName,
		"isMultipleDataLocationsForServicesEnabled": nil,
		"marketingNotificationEmails":               []any{},
		"onPremisesLastSyncDateTime":                "2026-10-30T06:00:00Z",
		"onPremisesSyncEnabled":                     true,
		"partnerTenantType":                         nil,
		"postalCode":                                o.PostalCode,
		"preferredLanguage":                         o.PreferredLanguage,
		"securityComplianceNotificationMails":       []any{},
		"securityComplianceNotificationPhones":      []any{},
		"state":                                     o.State,
		"street":                                    o.Street,
		"technicalNotificationMails":                []any{adminUPN(p)},
		"tenantType":                                "AAD",
		"assignedPlans": []any{
			assigned("exchange", "113feb6c-3fe4-4440-bddc-54d774bf0318"),
			assigned("SharePoint", "e95bec33-7c88-4a70-8e19-b10bd9d0c014"),
			assigned("MicrosoftCommunicationsOnline", "3e26ee1f-8a5f-4d52-aee2-b81ce45c8f40"),
			assigned("AADPremiumService", "eec0eb4f-6444-4f95-aba0-50c24d67f998"),
			assigned("MultiFactorService", "8a256a2b-b617-496d-b51b-e76466e88db0"),
			assigned("OfficeForms", "e212cbc7-0961-4c40-9825-01117710dcb1"),
			assigned("To-Do", "3fb82609-8c27-4f7b-bd51-30634711ee67"),
			assigned("YammerEnterprise", "7547a3fe-08ee-4ccb-b430-5077c5041653"),
			assigned("RMSOnline", "6c57d4b6-3b23-47a5-9bc9-69f17b4947b3"),
			assigned("PowerAppsService", "9c0dab89-a30c-4117-86e7-97bda240acd2"),
			assigned("PowerBI", "70d33638-9c74-4d01-bfd3-562de28bd4ba"),
			assigned("WhiteboardServices", "4a51bca5-1eff-43f5-878c-177680f191af"),
		},
		"onPremisesSyncStatus": []any{},
		"privacyProfile":       Object{"contactEmail": "privacy@" + o.Domain, "statementUrl": "https://" + o.Domain + "/privacy"},
		"provisionedPlans": []any{
			provisioned("exchange"), provisioned("SharePoint"), provisioned("MicrosoftCommunicationsOnline"),
			provisioned("RMSOnline"), provisioned("YammerEnterprise"), provisioned("ProjectWorkManagement"),
		},
		"verifiedDomains": verifiedDomains(o, p.Organization.Domains),
	}
}

// verifiedDomains lists the initial domain, the default domain, and the others. The default domain may be the initial one: then it is one entry.
func verifiedDomains(o ProfileOrganization, others []ProfileDomain) []any {
	out := []any{Object{"name": o.InitialDomain, "type": "Managed", "isDefault": o.Domain == o.InitialDomain, "isInitial": true, "capabilities": "Email, OfficeCommunicationsOnline"}}
	seen := map[string]bool{o.InitialDomain: true}
	for _, name := range append([]string{o.Domain}, domainNames(others)...) {
		if !seen[name] {
			seen[name] = true
			out = append(out, Object{"name": name, "type": "Managed", "isDefault": name == o.Domain, "isInitial": false, "capabilities": "Email, OfficeCommunicationsOnline"})
		}
	}
	return out
}

func domainNames(ds []ProfileDomain) []string {
	names := make([]string, len(ds))
	for i, d := range ds {
		names[i] = d.Name
	}
	return names
}

// ensureOrganization gives a seed that has no organization (a seed from before there was one) the fake tenant. A seed that has
// one with fewer properties (from before it was complete) gets the properties that it lacks, and keeps the ones that it has.
func (w *World) ensureOrganization() {
	if w.Organization == nil {
		w.Organization = newOrganization(DefaultProfile())
		return
	}
	for k, v := range newOrganization(DefaultProfile()) {
		if _, has := w.Organization[k]; !has {
			w.Organization[k] = v
		}
	}
}

// defaultDomain is the default verified domain of the tenant, where the addresses of the users are. Call it with the lock held, or
// while the world is made (the organization may be nil then, and the default profile's domain is used).
func (w *World) defaultDomain() string {
	if domains, ok := w.Organization["verifiedDomains"].([]any); ok {
		for _, d := range domains {
			if m, ok := d.(Object); ok && m["isDefault"] == true {
				return str(m["name"])
			}
		}
	}
	if w.Profile != nil {
		return w.Profile.Organization.Domain
	}
	return DefaultProfile().Organization.Domain
}

// AdminAccount is the user principal name of the admin account of the owner. The quick connect signs in as it.
func (w *World) AdminAccount() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.Admin != "" {
		return w.Admin
	}
	return AdminUPN
}

// orgView is the organization as Graph tells it: what the data has, and directorySizeQuota, which is the number of objects of the
// directory and the quota. The directory has the users, the groups, and the service principals. The call holds the read lock.
func (w *World) orgView() Object {
	o := deepCopy(w.Organization)
	o["directorySizeQuota"] = Object{"used": len(w.Users) + len(w.Groups) + len(w.SPs), "total": directoryQuota}
	return o
}

// ListOrganizations gives the organizations of the tenant: one.
func (w *World) ListOrganizations() []Object {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return []Object{w.orgView()}
}

// GetOrganization gives the organization with this ID.
func (w *World) GetOrganization(id string) (Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if !strings.EqualFold(str(w.Organization["id"]), id) {
		return nil, false
	}
	return w.orgView(), true
}

// evOrgSet is the event of a change of the properties of the organization (Set has them).
const evOrgSet = "organization.set"

// OrganizationUpdatable are the properties that PATCH /organization/{id} can change, from the page of Microsoft Learn (organization-update).
// The notification lists must not be null.
var OrganizationUpdatable = []string{"marketingNotificationEmails", "privacyProfile", "securityComplianceNotificationMails", "securityComplianceNotificationPhones", "technicalNotificationMails"}

// UpdateOrganization changes properties of the organization. A property that is not in OrganizationUpdatable is refused. A property
// that is not given keeps its value (privacyProfile is merged: statementUrl and contactEmail).
func (w *World) UpdateOrganization(id string, props map[string]any, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !strings.EqualFold(str(w.Organization["id"]), id) {
		return badRequest("Invalid tenant identifier; it must match that of the requested tenant.")
	}
	set := map[string]any{}
	for name, v := range props {
		if !slices.Contains(OrganizationUpdatable, name) {
			// A guess: Graph refuses another property, but the text of its error was not checked (the proxy of Graph Explorer reads only).
			return badRequest("The property '%s' of an organization cannot be updated. These can: %s.", name, strings.Join(OrganizationUpdatable, ", "))
		}
		switch name {
		case "privacyProfile":
			p, ok := v.(map[string]any)
			if !ok {
				return badRequest("The property 'privacyProfile' must be an object with contactEmail and statementUrl.")
			}
			merged, _ := w.Organization["privacyProfile"].(Object)
			merged = maps.Clone(merged)
			if merged == nil {
				merged = Object{}
			}
			for k, pv := range p {
				if k != "contactEmail" && k != "statementUrl" {
					return badRequest("The property '%s' of privacyProfile does not exist. It has contactEmail and statementUrl.", k)
				}
				if pv != nil {
					if _, ok := pv.(string); !ok {
						return badRequest("The property '%s' of privacyProfile must be a string.", k)
					}
				}
				merged[k] = pv
			}
			set[name] = merged
		default: // a list of texts, not null
			list, ok := v.([]any)
			if !ok {
				return badRequest("The property '%s' must be a list of texts, and it cannot be null.", name)
			}
			for _, item := range list {
				if _, ok := item.(string); !ok {
					return badRequest("The property '%s' must be a list of texts.", name)
				}
			}
			set[name] = list
		}
	}
	maps.Copy(w.Organization, set)
	w.record(Event{Type: evOrgSet, Actor: a, Set: deepCopy(set)})
	return nil
}

func organizationEventText(e Event) (string, bool) {
	if e.Type != evOrgSet {
		return "", false
	}
	return fmt.Sprintf("Changed the organization: %s", setText(e.Set)), true
}

func (w *World) applyOrganizationEvent(e Event) bool {
	if e.Type != evOrgSet {
		return false
	}
	maps.Copy(w.Organization, e.Set)
	return true
}
