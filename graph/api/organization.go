package api

import "net/http"

// OrganizationBackend is what the organization resource needs from the data. A tenant has
// one organization. A tool reads it to learn the name and the domains of its tenant.
type OrganizationBackend interface {
	// ListOrganizations gives the organizations of the tenant: one.
	ListOrganizations() []Object
	// GetOrganization finds the organization by ID.
	GetOrganization(id string) (Object, bool)
}

// Organization is the resource for /organization.
func Organization(b OrganizationBackend) Resource { return organization{b} }

type organization struct{ b OrganizationBackend }

func (o organization) Register(r *Router) {
	env := r.Env()
	s := r.Scope("/organization", "Organization")

	s.Handle("GET", "", "List the organizations of the tenant. There is one. It has the id of the tenant, its name, and its verified domains (one is the default).", []string{"$select", "$top", "$count", "$orderby"},
		func(w http.ResponseWriter, req *http.Request) {
			ServeList(w, req, env, o.b.ListOrganizations(), ListOptions{NoConsistency: true, Context: "organization"})
		})
	s.Handle("GET", "/$count", "Count the organizations of the tenant.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			ServeCount(w, req, o.b.ListOrganizations(), ListOptions{NoConsistency: true})
		})
	s.Handle("GET", "/{id}", "Get the organization, by the id of the tenant.", []string{"$select"},
		func(w http.ResponseWriter, req *http.Request) {
			item, ok := o.b.GetOrganization(req.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("organization", req.PathValue("id")))
				return
			}
			WriteJSON(w, http.StatusOK, one("organization", item, req, nil, nil, ""))
		})
}
