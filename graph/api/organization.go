package api

import "net/http"

// OrganizationBackend is what the organization resource needs from the data. A tenant has
// one organization. A tool reads it to learn the name and the domains of its tenant.
type OrganizationBackend interface {
	// ListOrganizations gives the organizations of the tenant: one.
	ListOrganizations() []Object
	// GetOrganization finds the organization by ID.
	GetOrganization(id string) (Object, bool)
	// UpdateOrganization changes properties of the organization (see repo.OrganizationUpdatable).
	UpdateOrganization(id string, props Object, a Actor) *Error
}

// DefaultOrganizationProperties are the properties that /organization gives when the call has no $select. They are the ones that the real
// Graph gave for a tenant (checked with the sample tenant of Graph Explorer, in the order of Graph). A property that is not here, such as
// mobileDeviceManagementAuthority, is given when $select asks for it.
var DefaultOrganizationProperties = []string{
	"id", "deletedDateTime", "businessPhones", "city", "country", "countryLetterCode", "createdDateTime", "defaultUsageLocation",
	"displayName", "isMultipleDataLocationsForServicesEnabled", "marketingNotificationEmails", "onPremisesLastSyncDateTime",
	"onPremisesSyncEnabled", "partnerTenantType", "postalCode", "preferredLanguage", "securityComplianceNotificationMails",
	"securityComplianceNotificationPhones", "state", "street", "technicalNotificationMails", "tenantType", "directorySizeQuota",
	"assignedPlans", "onPremisesSyncStatus", "privacyProfile", "provisionedPlans", "verifiedDomains",
}

// Organization is the resource for /organization.
func Organization(b OrganizationBackend) Resource { return organization{b} }

type organization struct{ b OrganizationBackend }

// tenantQuery refuses what Graph refuses on /organization (checked with the real Graph): $filter, $orderby, and $skip. It takes
// $count=true out of the call, because Graph leaves it out of the answer, with the header ConsistencyLevel or without it.
func tenantQuery(req *http.Request) *Error {
	q := req.URL.Query()
	switch {
	case q.Has("$filter"):
		return Unsupported("Filtered searches against this resource are not supported.")
	case q.Has("$orderby"):
		return Unsupported("Sorting is not supported for tenant queries.")
	case q.Has("$skip"):
		return BadRequest("'$skip' is not supported by the service.")
	}
	if q.Has("$count") {
		q.Del("$count")
		req.URL.RawQuery = q.Encode()
	}
	return nil
}

func (o organization) Register(r *Router) {
	env := r.Env()
	s := r.Scope("/organization", "Organization")

	s.Handle("GET", "", "List the organizations of the tenant. There is one. It has the properties of the organization resource: the address, the notification lists, the plans, the privacy profile, the size of the directory, and the verified domains (one is the default). Graph does not take $filter, $orderby, or $skip here, and it leaves $count out.", []string{"$select", "$top"},
		func(w http.ResponseWriter, req *http.Request) {
			if e := tenantQuery(req); e != nil {
				WriteError(w, e)
				return
			}
			ServeList(w, req, env, o.b.ListOrganizations(), ListOptions{Context: "organization", Resource: "organization", Default: DefaultOrganizationProperties})
		})
	s.Handle("GET", "/$count", "Graph does not count the organizations. It gives the error Request_UnsupportedQuery, with or without the header ConsistencyLevel.", nil,
		func(w http.ResponseWriter, _ *http.Request) {
			WriteError(w, Unsupported("Raw count (/$count) is not supported because it is used with the unsupported entity 'Organization'. Refer to https://aka.ms/graph-docs/advanced-queries for more information"))
		})
	s.Handle("GET", "/{id}", "Get the organization, by the id of the tenant. Another id is the error Request_BadRequest.", []string{"$select"},
		func(w http.ResponseWriter, req *http.Request) {
			item, ok := o.b.GetOrganization(req.PathValue("id"))
			if !ok {
				WriteError(w, BadRequest("Invalid tenant identifier; it must match that of the requested tenant."))
				return
			}
			WriteJSON(w, http.StatusOK, one("organization", item, req, DefaultOrganizationProperties, nil, "organization"))
		})
	s.Handle("PATCH", "/{id}", "Change properties of the organization. These can change: marketingNotificationEmails, privacyProfile (merged), securityComplianceNotificationMails, securityComplianceNotificationPhones, and technicalNotificationMails. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				e = o.b.UpdateOrganization(req.PathValue("id"), props, env.Actor(req))
			}
			respond(w, e)
		})
}
