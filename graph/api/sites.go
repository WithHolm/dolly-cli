package api

import (
	"net/http"
	"strings"
)

// SitesBackend is what the sites resource needs from the data: the SharePoint sites of the tenant, and the drives on them. A site is an object as
// Graph shows it (id, name, displayName, webUrl, and so on). The lists are copies. Graph has no call here that makes or changes a site: the portal
// does (the admin API).
type SitesBackend interface {
	// RootSite gives the root site of the tenant.
	RootSite() (Object, bool)
	// GetSite finds a site by its ID: the host and two GUIDs, with commas between.
	GetSite(id string) (Object, bool)
	// SiteByPath finds a site by the host and the server relative path.
	SiteByPath(host, path string) (Object, bool)
	// GroupSite gives the team site of a Microsoft 365 group. The first bool is false when there is no such group, and the second when it has no site.
	GroupSite(groupID string) (o Object, groupFound, siteFound bool)
	// SearchSites gives the sites that match a text. "*" and "" give every site.
	SearchSites(text string) []Object
	// Subsites gives the sites below a site. The bool is false when there is no such site.
	Subsites(id string) ([]Object, bool)
	// SiteDrives gives the drives of a site. The bool is false when there is no such site.
	SiteDrives(id string) ([]Object, bool)
	// SiteDrive gives one drive of a site, or the first (the default) when driveID is empty.
	SiteDrive(siteID, driveID string) (o Object, siteFound, driveFound bool)
}

// Sites is the resource for /sites, and for /groups/{id}/sites/root.
func Sites(b SitesBackend) Resource { return sites{b} }

type sites struct{ b SitesBackend }

// ItemNotFound is the error of the services that sit on SharePoint (sites, drives, and their items): the real Graph answers
// 404 itemNotFound, "Item not found", and not the error of the directory.
func ItemNotFound() *Error {
	return &Error{http.StatusNotFound, "itemNotFound", "Item not found"}
}

func (s sites) Register(r *Router) {
	env := r.Env()
	st := r.Scope("/sites", "Sites")
	opts := ListOptions{NoConsistency: true}

	one := func(w http.ResponseWriter, req *http.Request, o Object) {
		WriteJSON(w, http.StatusOK, SelectFields(o, SplitList(req.URL.Query().Get("$select")), nil))
	}
	// site finds the site that a call names: "root" is the root site of the tenant.
	site := func(id string) (Object, bool) {
		if strings.EqualFold(id, "root") {
			return s.b.RootSite()
		}
		if host, ok := strings.CutSuffix(id, ":"); ok { // /sites/{hostname}: is the root site of that host
			return s.b.SiteByPath(host, "")
		}
		return s.b.GetSite(id)
	}
	siteID := func(o Object) string { return Str(o["id"]) }
	subsites := func(w http.ResponseWriter, req *http.Request, o Object) {
		items, _ := s.b.Subsites(siteID(o))
		ServeList(w, req, env, items, opts)
	}
	drives := func(w http.ResponseWriter, req *http.Request, o Object) {
		items, _ := s.b.SiteDrives(siteID(o))
		ServeList(w, req, env, items, opts)
	}
	drive := func(w http.ResponseWriter, req *http.Request, o Object, driveID string) {
		d, _, ok := s.b.SiteDrive(siteID(o), driveID)
		if !ok {
			WriteError(w, ItemNotFound())
			return
		}
		one(w, req, d)
	}

	// The calls that take the site as {id}. The ID can be "root".
	withSite := func(f func(http.ResponseWriter, *http.Request, Object)) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			o, ok := site(req.PathValue("id"))
			if !ok {
				WriteError(w, ItemNotFound())
				return
			}
			f(w, req, o)
		}
	}

	all := func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		text := q.Get("search")
		if text == "" {
			text = q.Get("$search")
		}
		ServeList(w, req, env, s.b.SearchSites(text), opts)
	}
	st.Handle("GET", "", "List the sites, or search them: ?search=text matches the name, the title, the description, and the address of a site. With no text it lists every site. Only $orderby=createdDateTime is documented to work with a search.", []string{"search", "$select", "$orderby"}, all)
	st.Handle("GET", "/getAllSites()", "List every site of the tenant.", []string{"$select", "$orderby"}, all)
	st.Handle("GET", "/{id}", "Get a site by its ID: the host and two GUIDs, with commas between. 'root' is the root site of the tenant.", []string{"$select"}, withSite(one))
	st.Handle("GET", "/{id}/sites", "List the subsites of a site.", []string{"$select", "$orderby"}, withSite(subsites))
	st.Handle("GET", "/{id}/drives", "List the drives (document libraries) of a site.", []string{"$select", "$orderby"}, withSite(drives))
	st.Handle("GET", "/{id}/drive", "Get the default drive of a site: the first one, Documents.", []string{"$select"},
		withSite(func(w http.ResponseWriter, req *http.Request, o Object) { drive(w, req, o, "") }))
	st.Handle("GET", "/{id}/drives/{drive}", "Get one drive of a site.", []string{"$select"},
		withSite(func(w http.ResponseWriter, req *http.Request, o Object) { drive(w, req, o, req.PathValue("drive")) }))

	// A site is also found by its link: /sites/{hostname}:/{relative-path}, and below it :/drive, :/drives, and :/sites. The host ends with a colon.
	st.Handle("GET", "/{host}/{path...}", "Get a site by its host name and its server relative path: /sites/contoso.sharepoint.com:/sites/intranet. After the path, :/drive, :/drives, and :/sites give the drive, the drives, and the subsites of that site.", []string{"$select"},
		func(w http.ResponseWriter, req *http.Request) {
			host, ok := strings.CutSuffix(req.PathValue("host"), ":")
			if !ok {
				WriteError(w, Unsupported("The path of this call is not one that dollycli answers."))
				return
			}
			path, suffix, _ := strings.Cut(req.PathValue("path"), ":/")
			o, found := s.b.SiteByPath(host, strings.TrimSuffix(path, ":"))
			if !found {
				WriteError(w, ItemNotFound())
				return
			}
			switch suffix {
			case "":
				one(w, req, o)
			case "drive":
				drive(w, req, o, "")
			case "drives":
				drives(w, req, o)
			case "sites":
				subsites(w, req, o)
			default:
				WriteError(w, Unsupported("The path of this call is not one that dollycli answers."))
			}
		})

	// The team site of a Microsoft 365 group.
	g := r.Scope("/groups", "Sites")
	g.Handle("GET", "/{id}/sites/{site}", "Get the team site of a Microsoft 365 group: 'root' for the site of the group, or the ID of the site. A group has one site.", []string{"$select"},
		func(w http.ResponseWriter, req *http.Request) {
			o, groupFound, siteFound := s.b.GroupSite(req.PathValue("id"))
			switch {
			case !groupFound:
				WriteError(w, NotFound("group", req.PathValue("id")))
			case !siteFound:
				WriteError(w, ItemNotFound())
			case !strings.EqualFold(req.PathValue("site"), "root") && !strings.EqualFold(req.PathValue("site"), Str(o["id"])):
				WriteError(w, ItemNotFound())
			default:
				one(w, req, o)
			}
		})
}
