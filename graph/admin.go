package graph

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/withholm/dolly-cli/graph/cost"
	"github.com/withholm/dolly-cli/graph/permissions"
)

// The admin API is for the person who runs the server. The `dolly graph`
// commands and the Dolly page call it. It lets a person change the world
// between two runs of a tool, for example to rename a department and see what
// a sync does, and it shows the calls that the tool made. (The data that it
// changes is in the repo package.)
//
// The paths are under /_dolly/ on the server. AdminHandler gives the same
// routes without that start, for the status page.

// AdminHandler gives the admin API with paths like /state and /requests.
func (s *Server) AdminHandler() http.Handler { return s.admin }

func (s *Server) adminRoutes() {
	a := s.admin
	a.HandleFunc("GET /info", s.adminInfo)
	a.HandleFunc("GET /state", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, s.world.Snapshot()) })
	a.HandleFunc("POST /rename", s.adminRename)
	a.HandleFunc("POST /user", s.adminUser)
	a.HandleFunc("POST /reset", func(w http.ResponseWriter, _ *http.Request) {
		if err := s.world.Reset(adminActor); err != nil {
			writeError(w, apiErr(http.StatusInternalServerError, "generalException", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, s.world.Snapshot())
	})
	// A profile says what a generated tenant looks like (see repo.Profile). The portal reads the default one, shows what a profile would make,
	// and makes the data from it.
	a.HandleFunc("GET /profile/default", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, DefaultProfile()) })
	// The profile that the data was made from, or the default one for data that has none (data from before profiles, or from the default).
	a.HandleFunc("GET /profile/current", func(w http.ResponseWriter, _ *http.Request) {
		if p := s.world.CurrentProfile(); p != nil {
			writeJSON(w, http.StatusOK, p)
			return
		}
		writeJSON(w, http.StatusOK, DefaultProfile())
	})
	a.HandleFunc("POST /profile/preview", s.adminProfile(false))
	a.HandleFunc("POST /generate", s.adminProfile(true))
	a.HandleFunc("GET /history", s.adminHistory)
	a.HandleFunc("POST /rollback", s.adminRollback)
	a.HandleFunc("GET /api", func(w http.ResponseWriter, _ *http.Request) {
		endpoints, unsupported := s.API()
		if unsupported == nil {
			unsupported = []UnsupportedCall{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"endpoints": endpoints, "unsupported": unsupported, "options": QueryOptions,
			// the pages of the reference, from the markdown files
			"pages": s.DocPages(),
			// what $filter and $search do: the forms that dolly answers, and what Graph says about the properties
			"filterForms": FilterForms(), "searchForm": SearchDoc(), "filterProperties": filterProperties(endpoints),
			// for a directory object, what each operator needs on each property, and what sorts: Microsoft's tables
			"filterRules": filterRules(endpoints),
		})
	})
	a.HandleFunc("DELETE /stats", func(w http.ResponseWriter, _ *http.Request) {
		s.resetStats()
		w.WriteHeader(http.StatusNoContent)
	})
	// the ID of the tenant, and the service principals that clients sign in as (see repo/principal.go).
	// {appId} is the application (client) ID, not the object ID.
	a.HandleFunc("GET /tenant", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"id": s.world.TenantID()})
	})
	a.HandleFunc("PUT /tenant", s.adminSetTenant)
	// the client that the portal signs in as, to call the Graph API (see repo.PortalAppID)
	a.HandleFunc("GET /portal", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"appId": PortalAppID})
	})
	a.HandleFunc("GET /service-principals", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"servicePrincipals": s.world.ListServicePrincipalInfo()})
	})
	// the change notifications: the subscriptions, and what was sent to them
	a.HandleFunc("GET /subscriptions", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, s.adminSubscriptions()) })
	a.HandleFunc("POST /subscriptions", s.adminCreateSubscription)
	a.HandleFunc("POST /subscriptions/{id}/emulate", s.adminEmulate)
	a.HandleFunc("DELETE /subscriptions/{id}", func(w http.ResponseWriter, r *http.Request) {
		var e *apiError
		if err := s.notifier.Delete(r.PathValue("id")); err != nil {
			e = asAPIError(err)
		}
		s.adminChanged(w, e)
	})
	// the app registrations, for the page of the same name (a service principal with the same client ID is the instance of each)
	a.HandleFunc("GET /applications", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"applications": s.world.ListApplicationInfo()})
	})
	a.HandleFunc("PUT /service-principals/{appId}/mode", s.adminSetServicePrincipalMode)
	a.HandleFunc("PUT /service-principals/{appId}/flags", s.adminSetServicePrincipalFlags)
	a.HandleFunc("POST /service-principals/{appId}/credential/arm", func(w http.ResponseWriter, r *http.Request) {
		s.adminChanged(w, s.world.ArmCredential(r.PathValue("appId"), adminActor))
	})
	a.HandleFunc("DELETE /service-principals/{appId}/credential", func(w http.ResponseWriter, r *http.Request) {
		s.adminChanged(w, s.world.ClearCredential(r.PathValue("appId"), adminActor))
	})
	// permissions: the resources, their permissions to pick from, and what a service principal has
	// (see repo/grants.go). The page uses these; the same changes are in the Graph API with a token.
	a.HandleFunc("GET /resources", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"resources": s.world.Resources()})
	})
	a.HandleFunc("GET /resources/{appId}/permissions", s.adminResourcePermissions)
	a.HandleFunc("GET /service-principals/{appId}/grants", func(w http.ResponseWriter, r *http.Request) {
		g, ok := s.world.GrantsOf(r.PathValue("appId"))
		if !ok {
			writeError(w, apiErr(http.StatusNotFound, "Request_ResourceNotFound", "No service principal has the client ID '"+r.PathValue("appId")+"'."))
			return
		}
		writeJSON(w, http.StatusOK, g)
	})
	a.HandleFunc("POST /service-principals/{appId}/grants", s.adminGrantPermission)
	a.HandleFunc("DELETE /service-principals/{appId}/grants", s.adminRevokePermission)
	// what each service principal has asked for, in resource units (see package cost)
	a.HandleFunc("GET /usage", func(w http.ResponseWriter, _ *http.Request) {
		usage := []cost.Usage{}
		if s.limiter != nil {
			usage = s.limiter.Usage()
		}
		writeJSON(w, http.StatusOK, map[string]any{"servicePrincipals": usage, "throttle": s.behavior().Throttle})
	})
	// the use over time, in buckets of 10 seconds: of one client (?app=<client ID>) or of all of them, with the limit of the tenant
	a.HandleFunc("GET /usage/history", func(w http.ResponseWriter, r *http.Request) {
		if s.limiter == nil {
			writeError(w, apiErr(http.StatusServiceUnavailable, "serviceNotAvailable", "Dolly does not count the cost of the calls: its cost data cannot be read."))
			return
		}
		out := map[string]any{"history": s.limiter.History(r.URL.Query().Get("app"))}
		if line, ok := s.limiter.RULimit(s.world.UserCount()); ok {
			out["limit"] = line
		}
		writeJSON(w, http.StatusOK, out)
	})
	// the permissions that the calls of a client need (see needs.go), and forget them
	a.HandleFunc("GET /service-principals/{appId}/needs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.needsOf(r.PathValue("appId")))
	})
	a.HandleFunc("DELETE /service-principals/{appId}/needs", func(w http.ResponseWriter, r *http.Request) {
		s.forgetNeeds(r.PathValue("appId"))
		w.WriteHeader(http.StatusNoContent)
	})
	a.HandleFunc("DELETE /usage", func(w http.ResponseWriter, _ *http.Request) {
		if s.limiter != nil {
			s.limiter.Reset()
		}
		s.forgetNeeds("") // what the clients did goes with the use
		w.WriteHeader(http.StatusNoContent)
	})
	a.HandleFunc("GET /requests", s.adminRequests)
	a.HandleFunc("DELETE /requests", func(w http.ResponseWriter, _ *http.Request) {
		s.calls.clear()
		w.WriteHeader(http.StatusNoContent)
	})
	a.HandleFunc("GET /behavior", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, s.behavior()) })
	a.HandleFunc("PUT /behavior", s.adminSetBehavior)
}

func (s *Server) adminInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"graphUrl": s.graphURL, "dir": s.world.Dir()})
}

// HistoryEntry is a change in the list of the page and of the CLI.
type HistoryEntry struct {
	Event
	Text string `json:"text"`
}

// adminHistory gives the changes, newest first. ?limit= cuts the list.
func (s *Server) adminHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	events := s.world.History()
	out := make([]HistoryEntry, 0, min(limit, len(events)))
	for i := len(events) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, HistoryEntry{Event: events[i], Text: events[i].Text()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out, "total": len(events), "kept": s.world.Dir() != ""})
}

// adminRollback takes the world back. The body says where:
//
//	{"ago": "1h"}                      the state as it was one hour ago
//	{"to": "2026-10-10T01:00:00Z"}     the state at a time
//	{"before": 17}                     the state before change 17
//	{"seed": true}                     the seed, with no change
func (s *Server) adminRollback(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Ago    string `json:"ago"`
		To     string `json:"to"`
		Before int    `json:"before"`
		Seed   bool   `json:"seed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, badRequest("The body is not valid JSON."))
		return
	}
	var target int
	switch {
	case b.Seed:
		target = 0
	case b.Before > 0:
		target = b.Before - 1
	case b.Ago != "":
		d, err := time.ParseDuration(b.Ago)
		if err != nil || d <= 0 {
			writeError(w, badRequest("ago %q is not a time span. Use a value like 30m or 2h.", b.Ago))
			return
		}
		target = s.world.RollbackTarget(time.Now().Add(-d))
	case b.To != "":
		t, err := time.Parse(time.RFC3339, b.To)
		if err != nil {
			writeError(w, badRequest("to %q is not a time. Use a form like 2026-10-10T01:00:00Z.", b.To))
			return
		}
		target = s.world.RollbackTarget(t)
	default:
		writeError(w, badRequest("Rollback needs ago, to, before, or seed."))
		return
	}
	if err := s.world.Rollback(target, adminActor); err != nil {
		writeError(w, badRequest("%s", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"target": target, "state": s.world.Snapshot()})
}

func (s *Server) adminRename(w http.ResponseWriter, r *http.Request) {
	var b struct{ Field, From, To string }
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.Field == "" || b.From == "" || b.To == "" {
		writeError(w, badRequest("Rename needs field, from, and to."))
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"changed": s.world.Rename(b.Field, b.From, b.To, adminActor)})
}

// adminProfile reads {"profile": {...}, "users": 200, "seed": 1}. With apply it makes the data from the profile, in place of the data that
// the server has (and the history is gone). Without it, it only says what the profile would make. A profile from outside is checked in
// full, and its size is limited.
func (s *Server) adminProfile(apply bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Profile json.RawMessage `json:"profile"`
			Users   int             `json:"users"`
			Seed    int64           `json:"seed"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&b); err != nil {
			writeError(w, badRequest("The body must be JSON with a profile, and optionally users and seed."))
			return
		}
		o := Options{Users: b.Users, Seed: b.Seed}
		if len(b.Profile) > 0 && string(b.Profile) != "null" {
			p, err := ParseProfile(bytes.NewReader(b.Profile))
			if err != nil {
				writeError(w, badRequest("%s", err))
				return
			}
			o.Profile = p
		}
		if o.Users < 0 || o.Users > 100000 {
			writeError(w, badRequest("users must be a whole number from 1 to 100000."))
			return
		}
		if !apply {
			pv, err := PreviewProfile(o.Profile, o.Users, o.Seed)
			if err != nil {
				writeError(w, badRequest("%s", err))
				return
			}
			writeJSON(w, http.StatusOK, pv)
			return
		}
		if o.Users == 0 {
			o.Users = s.world.Snapshot().Users
		}
		if o.Seed == 0 {
			o.Seed = s.world.Snapshot().Seed
		}
		st, err := s.world.Regenerate(o)
		if err != nil {
			writeError(w, apiErr(http.StatusInternalServerError, "generalException", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, st)
	}
}

func (s *Server) adminUser(w http.ResponseWriter, r *http.Request) {
	var b struct {
		User string         `json:"user"`
		Set  map[string]any `json:"set"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.User == "" || len(b.Set) == 0 {
		writeError(w, badRequest("User needs user and set."))
		return
	}
	// the same checks as PATCH /users/{id}: a UPN that is taken, and a value of the wrong type
	if e := s.world.UpdateUser(b.User, b.Set, adminActor); e != nil {
		writeError(w, e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"changed": true})
}

// adminRequests gives the calls after ?since=, and the cursor for the next ask.
func (s *Server) adminRequests(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > maxCalls {
		limit = 500
	}
	calls, cursor := s.calls.since(since, limit)
	if calls == nil {
		calls = []Call{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"calls": calls, "cursor": cursor})
}

func (s *Server) adminSetBehavior(w http.ResponseWriter, r *http.Request) {
	var b Behavior
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, badRequest("The body is not valid JSON."))
		return
	}
	if err := b.check(); err != nil {
		writeError(w, badRequest("%s", err.Error()))
		return
	}
	s.bmu.Lock()
	s.beh = b
	s.bmu.Unlock()
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) adminSetTenant(w http.ResponseWriter, r *http.Request) {
	var b struct{ ID string }
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, badRequest("Set the tenant needs id."))
		return
	}
	if e := s.world.SetTenantID(b.ID, adminActor); e != nil {
		writeError(w, e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": s.world.TenantID()})
}

func (s *Server) adminSetServicePrincipalMode(w http.ResponseWriter, r *http.Request) {
	var b struct{ Mode string }
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, badRequest("Set the mode needs mode."))
		return
	}
	s.adminChanged(w, s.world.SetServicePrincipalMode(r.PathValue("appId"), b.Mode, adminActor))
}

// adminSetServicePrincipalFlags sets the flags that the body names: exemptFromThrottle and hideCalls. A flag that the body
// does not have stays as it is.
func (s *Server) adminSetServicePrincipalFlags(w http.ResponseWriter, r *http.Request) {
	var b map[string]bool
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || len(b) == 0 {
		writeError(w, badRequest("Set the flags needs exemptFromThrottle or hideCalls, each true or false."))
		return
	}
	appID := r.PathValue("appId")
	for _, flag := range []string{FlagExemptFromThrottle, FlagHideCalls} {
		if on, ok := b[flag]; ok {
			if e := s.world.SetServicePrincipalFlag(appID, flag, on, adminActor); e != nil {
				s.adminChanged(w, e)
				return
			}
		}
	}
	s.adminChanged(w, nil)
}

func permissionKind(s string) (permissions.Kind, *apiError) {
	switch k := permissions.Kind(s); k {
	case permissions.Application, permissions.Delegated:
		return k, nil
	}
	return "", badRequest("The kind must be 'application' or 'delegated'.")
}

// adminResourcePermissions lists the permissions of a resource, for the picker: ?kind=, ?q= (text in the
// value or the name), and ?limit= (100 when it is not given).
func (s *Server) adminResourcePermissions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind, e := permissionKind(q.Get("kind"))
	if e != nil {
		writeError(w, e)
		return
	}
	limit := 100
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n >= 0 && n <= 2000 {
		limit = n
	}
	list, total, ok := s.world.ResourcePermissions(r.PathValue("appId"), kind, q.Get("q"), limit)
	if !ok {
		writeError(w, apiErr(http.StatusNotFound, "Request_ResourceNotFound", "No resource has the client ID '"+r.PathValue("appId")+"'."))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": list, "total": total})
}

func (s *Server) adminGrantPermission(w http.ResponseWriter, r *http.Request) {
	var b struct{ ResourceAppID, Kind, Value string }
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.ResourceAppID == "" || b.Value == "" {
		writeError(w, badRequest("Grant needs resourceAppId, kind, and value."))
		return
	}
	kind, e := permissionKind(b.Kind)
	if e == nil {
		e = s.world.GrantPermission(r.PathValue("appId"), b.ResourceAppID, kind, b.Value, adminActor)
	}
	s.adminChanged(w, e)
}

// adminRevokePermission takes a permission away. It reads ?resourceAppId=, ?kind=, and ?value=,
// because a DELETE has no body that every client sends.
func (s *Server) adminRevokePermission(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind, e := permissionKind(q.Get("kind"))
	if e == nil && (q.Get("resourceAppId") == "" || q.Get("value") == "") {
		e = badRequest("Revoke needs resourceAppId, kind, and value.")
	}
	if e == nil {
		e = s.world.RevokePermission(r.PathValue("appId"), q.Get("resourceAppId"), kind, q.Get("value"), adminActor)
	}
	s.adminChanged(w, e)
}

// adminChanged answers a change that has no body to give back: 204, or the error.
func (s *Server) adminChanged(w http.ResponseWriter, e *apiError) {
	if e != nil {
		writeError(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
