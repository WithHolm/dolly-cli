package repo

import (
	"fmt"
	"slices"
	"strings"
)

// The API that an app registration exposes: the Application ID URI (identifierUris), the delegated permissions (api.oauth2PermissionScopes), and the clients
// that need no consent for some of them (api.preAuthorizedApplications).
//
// The rules are from the pages of Microsoft Learn "permissionScope resource type" and "apiApplication resource type" (the commit of graft/cost/raw/SOURCE.json):
// the value of a scope has the same rules as the value of an app role, a scope is disabled in one call and removed in a later one, and the type is User (a user
// can consent for himself or herself) or Admin. A write cannot be tried with the Explorer proxy, so the texts of the errors are dollycli's own. All of it is written
// with PATCH /applications, as in Graph, and the service principal of the app follows (syncAPIFromApp).

// Scope types.
const (
	ScopeUser  = "User"
	ScopeAdmin = "Admin"
)

func (s AppScope) typ() string {
	if s.Type == "" {
		return ScopeAdmin
	}
	return s.Type
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// object gives the scope as Graph writes it.
func (s AppScope) object() Object {
	return Object{
		"id": s.ID, "value": s.Value, "type": s.typ(), "isEnabled": !s.Disabled,
		"adminConsentDisplayName": s.Name, "adminConsentDescription": s.Description,
		"userConsentDisplayName": nullIfEmpty(s.UserName), "userConsentDescription": nullIfEmpty(s.UserDescription),
	}
}

func scopeObjects(scopes []AppScope) []any {
	out := make([]any, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, s.object())
	}
	return out
}

// scopeFromObject reads a scope that was checked, or that the data has. It does not check.
func scopeFromObject(m map[string]any) AppScope {
	s := AppScope{
		ID: str(m["id"]), Value: str(m["value"]), Name: firstNonEmpty(str(m["adminConsentDisplayName"]), str(m["value"])), Description: str(m["adminConsentDescription"]),
		UserName: str(m["userConsentDisplayName"]), UserDescription: str(m["userConsentDescription"]), Type: str(m["type"]),
	}
	if on, ok := m["isEnabled"].(bool); ok && !on {
		s.Disabled = true
	}
	return s
}

func scopesFromList(v any) []AppScope {
	var out []AppScope
	for _, m := range anyObjects(v) {
		out = append(out, scopeFromObject(m))
	}
	return out
}

// valueProblem says what is wrong with the value of a role or a scope, or "".
func valueProblem(s string) string {
	switch {
	case s == "":
		return "is empty"
	case len(s) > 120:
		return "has more than 120 characters"
	case strings.HasPrefix(s, "."):
		return "must not start with a dot"
	case !roleValueRe.MatchString(s):
		return "has a character that is not allowed (no space; letters, digits, and : ! # $ % & ' ( ) * + , - . / ; < = > ? @ [ ] ^ _ ` { | } ~)"
	}
	return ""
}

// parseScopeList reads and checks a list of scopes from a call. resource is the name for the texts of the errors.
func parseScopeList(v any, resource string) ([]AppScope, *apiError) {
	bad := func(format string, a ...any) *apiError {
		return badRequest("Invalid value specified for property 'oauth2PermissionScopes' of resource '%s': %s", resource, fmt.Sprintf(format, a...))
	}
	list, ok := v.([]any)
	if !ok {
		return nil, bad("it must be a list of scopes.")
	}
	var out []AppScope
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, bad("item %d is not a scope.", i+1)
		}
		for k := range m {
			switch k {
			case "id", "value", "type", "isEnabled", "adminConsentDisplayName", "adminConsentDescription", "userConsentDisplayName", "userConsentDescription":
			default:
				return nil, bad("a scope has no property '%s'.", k)
			}
		}
		s := scopeFromObject(m)
		switch {
		case !IsGUID(s.ID):
			return nil, bad("item %d needs an id that is a GUID.", i+1)
		case valueProblem(s.Value) != "":
			return nil, bad("the value '%s' of the scope %s %s.", s.Value, s.ID, valueProblem(s.Value))
		case s.Type != ScopeUser && s.Type != ScopeAdmin:
			return nil, bad("the type of the scope %s must be %s or %s.", s.ID, ScopeUser, ScopeAdmin)
		case strings.TrimSpace(str(m["adminConsentDisplayName"])) == "":
			return nil, bad("the scope %s needs adminConsentDisplayName.", s.ID)
		case strings.TrimSpace(str(m["adminConsentDescription"])) == "":
			return nil, bad("the scope %s needs adminConsentDescription.", s.ID)
		}
		if v, has := m["isEnabled"]; has {
			if _, ok := v.(bool); !ok {
				return nil, bad("isEnabled of the scope %s must be true or false.", s.ID)
			}
		}
		out = append(out, s)
	}
	ids, values := map[string]bool{}, map[string]bool{}
	for _, s := range out {
		if ids[strings.ToLower(s.ID)] {
			return nil, bad("two scopes have the same id %s.", s.ID)
		}
		ids[strings.ToLower(s.ID)] = true
		if values[strings.ToLower(s.Value)] {
			return nil, bad("two scopes have the same value '%s'.", s.Value)
		}
		values[strings.ToLower(s.Value)] = true
	}
	return out, nil
}

// checkScopeRemoval checks that every scope that goes away was disabled before.
func checkScopeRemoval(old, now []AppScope, resource string) *apiError {
	for _, o := range old {
		if o.Disabled || slices.ContainsFunc(now, func(s AppScope) bool { return isID(s.ID, o.ID) }) {
			continue
		}
		return badRequest("Invalid value specified for property 'oauth2PermissionScopes' of resource '%s': the scope '%s' must be disabled (isEnabled false) in one call before a later call removes it.", resource, o.Value)
	}
	return nil
}

func toAnyList(list []map[string]any) []any {
	out := make([]any, len(list))
	for i, m := range list {
		out[i] = m
	}
	return out
}

// normalizeApplicationAPI checks the api of a call on an application, and writes it as the data keeps it. app is the application that is changed (nil for a new
// one): the scopes and the clients that it has now count for what the call does not say.
func normalizeApplicationAPI(body map[string]any, app Object) *apiError {
	v, has := body["api"]
	if !has || v == nil {
		return nil
	}
	bad := func(format string, a ...any) *apiError {
		return badRequest("Invalid value specified for property 'api' of resource 'Application': %s", fmt.Sprintf(format, a...))
	}
	in, ok := v.(map[string]any)
	if !ok {
		return bad("it must be an object.")
	}
	var old map[string]any
	if app != nil {
		old, _ = app["api"].(map[string]any)
	}
	out := map[string]any{}
	for k, val := range in {
		switch k {
		case "acceptMappedClaims":
			if _, isBool := val.(bool); val != nil && !isBool {
				return bad("acceptMappedClaims must be true or false.")
			}
		case "requestedAccessTokenVersion":
			switch n := val.(type) {
			case nil:
			case float64:
				if n != 1 && n != 2 {
					return bad("requestedAccessTokenVersion must be null, 1, or 2.")
				}
			case int:
				if n != 1 && n != 2 {
					return bad("requestedAccessTokenVersion must be null, 1, or 2.")
				}
			default:
				return bad("requestedAccessTokenVersion must be null, 1, or 2.")
			}
		case "knownClientApplications":
			list, isList := val.([]any)
			if !isList {
				return bad("knownClientApplications must be a list of client IDs.")
			}
			for _, x := range list {
				if s, _ := x.(string); !IsGUID(s) {
					return bad("knownClientApplications has '%v', which is not a client ID (a GUID).", x)
				}
			}
		case "oauth2PermissionScopes", "preAuthorizedApplications":
		default:
			return bad("api has no property '%s'.", k)
		}
		out[k] = val
	}

	// the scopes: checked, and written in full
	oldScopes := scopesFromList(old["oauth2PermissionScopes"])
	scopes := oldScopes
	if raw, has := in["oauth2PermissionScopes"]; has {
		var e *apiError
		if scopes, e = parseScopeList(raw, "Application"); e != nil {
			return e
		}
		if e := checkScopeRemoval(oldScopes, scopes, "Application"); e != nil {
			return e
		}
		out["oauth2PermissionScopes"] = scopeObjects(scopes)
	}

	// the clients that need no consent: each scope that it names must be a scope of this API. A scope that is gone goes out of the list.
	raw, preGiven := in["preAuthorizedApplications"]
	if _, scopesGiven := in["oauth2PermissionScopes"]; preGiven || scopesGiven {
		pre := anyObjects(old["preAuthorizedApplications"]) // a call that changes only the scopes keeps the clients
		if preGiven {
			list, isList := raw.([]any)
			if !isList {
				return bad("preAuthorizedApplications must be a list.")
			}
			pre = nil
			for _, x := range list {
				m, isObj := x.(map[string]any)
				if !isObj {
					return bad("preAuthorizedApplications has an item that is not an object.")
				}
				for k := range m {
					if k != "appId" && k != "delegatedPermissionIds" {
						return bad("preAuthorizedApplications has no property '%s'.", k)
					}
				}
				pre = append(pre, m)
			}
		}
		seen := map[string]bool{}
		norm := []any{}
		for _, m := range pre {
			appID := strings.ToLower(str(m["appId"]))
			if !IsGUID(appID) {
				return bad("a pre-authorized client needs an appId that is a GUID.")
			}
			if seen[appID] {
				return bad("the client %s is pre-authorized twice.", appID)
			}
			seen[appID] = true
			ids := []any{}
			for _, id := range anyStrings(m["delegatedPermissionIds"]) {
				if slices.ContainsFunc(scopes, func(s AppScope) bool { return isID(s.ID, id) }) {
					ids = append(ids, id)
				} else if preGiven {
					return bad("delegatedPermissionIds of the client %s has '%s', which is not a scope of this app.", appID, id)
				} // else the scope was removed: the client loses it
			}
			norm = append(norm, map[string]any{"appId": appID, "delegatedPermissionIds": ids})
		}
		out["preAuthorizedApplications"] = norm
	}
	body["api"] = out
	return nil
}

// checkIdentifierURIs checks the Application ID URIs of a call: api://... or https://..., each once, and no other app registration has them. selfID is the
// application that is changed. (A guess: the real Graph also asks for a verified domain in an https URI. The answer cannot be tried here.) Call it with the lock held.
func (w *World) checkIdentifierURIs(v any, selfID string) *apiError {
	list, _ := v.([]any)
	seen := map[string]bool{}
	for _, x := range list {
		s, _ := x.(string)
		low := strings.ToLower(strings.TrimSpace(s))
		if rest, ok := strings.CutPrefix(low, "api://"); !ok || rest == "" {
			if rest, ok := strings.CutPrefix(low, "https://"); !ok || rest == "" {
				return badRequest("Invalid value specified for property 'identifierUris' of resource 'Application': '%v' is not an Application ID URI. Use api://<name> or https://<domain>/<path>.", x)
			}
		}
		key := strings.TrimSuffix(low, "/")
		if seen[key] {
			return badRequest("Invalid value specified for property 'identifierUris' of resource 'Application': '%s' is in the list twice.", s)
		}
		seen[key] = true
		for _, a := range w.Applications {
			if str(a["id"]) == selfID {
				continue
			}
			for _, u := range anyStrings(a["identifierUris"]) {
				if strings.TrimSuffix(strings.ToLower(strings.TrimSpace(u)), "/") == key {
					return badRequest("Another object with the same value for property identifierUris already exists.")
				}
			}
		}
	}
	return nil
}

// normalizeServicePrincipalScopes checks the oauth2PermissionScopes of a call on a service principal, and writes them in full.
func normalizeServicePrincipalScopes(sp *ServicePrincipal, body map[string]any) *apiError {
	v, has := body["oauth2PermissionScopes"]
	if !has {
		return nil
	}
	scopes, e := parseScopeList(v, "ServicePrincipal")
	if e != nil {
		return e
	}
	if e := checkScopeRemoval(sp.Scopes, scopes, "ServicePrincipal"); e != nil {
		return e
	}
	body["oauth2PermissionScopes"] = scopeObjects(scopes)
	return nil
}

// setScopes sets the scopes of a service principal. A scope that is gone, or has a new value, is taken out of the consents that have it, and a consent that
// has no scope left is deleted. Call it with the lock held.
func (w *World) setScopes(sp *ServicePrincipal, scopes []AppScope) {
	var gone []string
	for _, o := range sp.Scopes {
		if !slices.ContainsFunc(scopes, func(s AppScope) bool { return isID(s.ID, o.ID) && s.Value == o.Value }) {
			gone = append(gone, o.Value)
		}
	}
	sp.Scopes = scopes
	if len(gone) == 0 {
		return
	}
	var keep []Grant
	for _, g := range w.Grants {
		if !isID(g.ResourceID, sp.ID) {
			keep = append(keep, g)
			continue
		}
		var left []string
		for _, v := range strings.Fields(g.Scope) {
			if !slices.ContainsFunc(gone, func(x string) bool { return strings.EqualFold(x, v) }) {
				left = append(left, v)
			}
		}
		if len(left) > 0 {
			g.Scope = strings.Join(left, " ")
			keep = append(keep, g)
		}
	}
	w.Grants = keep
}

// syncAPIFromApp gives the service principal of an application the Application ID URIs and the scopes of the application. Call it with the lock held.
func (w *World) syncAPIFromApp(app Object) {
	sp := w.byAppID[strings.ToLower(str(app["appId"]))]
	if sp == nil || sp.Builtin != "" {
		return
	}
	sp.IdentifierURIs = anyStrings(app["identifierUris"])
	api, _ := app["api"].(map[string]any)
	w.setScopes(sp, scopesFromList(api["oauth2PermissionScopes"]))
}

// versionOf reads requestedAccessTokenVersion: 1, 2, or 0 for null.
func versionOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// AccessTokenVersion gives the version of the access tokens of an API (1 or 2, from requestedAccessTokenVersion of its app registration, null is 1), and its first
// Application ID URI ("" when it has none, and for an API with no app registration in this tenant).
func (w *World) AccessTokenVersion(resourceAppID string) (int, string) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	app := w.appsByAppID[strings.ToLower(resourceAppID)]
	if app == nil {
		return 1, ""
	}
	api, _ := app["api"].(map[string]any)
	version := 1
	if versionOf(api["requestedAccessTokenVersion"]) == 2 {
		version = 2
	}
	uri := ""
	if uris := anyStrings(app["identifierUris"]); len(uris) > 0 {
		uri = uris[0]
	}
	return version, uri
}

// personalAudience says whether an app can be signed in to with a personal Microsoft account.
func personalAudience(audience string) bool {
	return audience == "AzureADandPersonalMicrosoftAccount" || audience == "PersonalMicrosoftAccount"
}

// applyTokenVersionRule keeps the rule of Learn (apiApplication): when signInAudience lets personal Microsoft accounts in, requestedAccessTokenVersion must be 2. A call
// that says another version is refused. A call that makes the app personal, and says nothing about the version, sets it to 2 (a guess: Learn gives the rule, not what
// the portal or the service do). app is the application that is changed, nil for a new one.
func applyTokenVersionRule(body map[string]any, app Object) *apiError {
	audience := "AzureADMyOrg"
	_, audienceGiven := body["signInAudience"]
	switch {
	case audienceGiven:
		audience = str(body["signInAudience"])
	case app != nil:
		audience = str(app["signInAudience"])
	}
	if !personalAudience(audience) {
		return nil
	}
	api, _ := body["api"].(map[string]any)
	if v, given := api["requestedAccessTokenVersion"]; given {
		if versionOf(v) != 2 {
			return badRequest("Invalid value specified for property 'requestedAccessTokenVersion' of resource 'Application': it must be 2 when signInAudience is %s.", audience)
		}
		return nil
	}
	current := 0
	if app != nil {
		old, _ := app["api"].(map[string]any)
		current = versionOf(old["requestedAccessTokenVersion"])
	}
	if current != 2 && (audienceGiven || app == nil) {
		if api == nil {
			api = map[string]any{}
			body["api"] = api
		}
		api["requestedAccessTokenVersion"] = float64(2)
	}
	return nil
}

// SetAccessTokenVersion sets requestedAccessTokenVersion of an application: 1, 2, or 0 for null.
func (w *World) SetAccessTokenVersion(ref string, version int, a Actor) *apiError {
	var v any
	switch version {
	case 0:
	case 1, 2:
		v = float64(version)
	default:
		return badRequest("Invalid value specified for property 'requestedAccessTokenVersion' of resource 'Application': it must be null, 1, or 2.")
	}
	return w.changeAPI(ref, func(_, set map[string]any) *apiError {
		set["requestedAccessTokenVersion"] = v
		return nil
	}, a)
}

// PreAuthorizedScopes gives the values of the scopes of an API that need no consent from a client: the client is pre-authorized for them, and they are enabled.
func (w *World) PreAuthorizedScopes(clientAppID, resourceAppID string) []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	app, sp := w.appsByAppID[strings.ToLower(resourceAppID)], w.byAppID[strings.ToLower(resourceAppID)]
	if app == nil || sp == nil {
		return nil
	}
	api, _ := app["api"].(map[string]any)
	var out []string
	for _, p := range anyObjects(api["preAuthorizedApplications"]) {
		if !strings.EqualFold(str(p["appId"]), clientAppID) {
			continue
		}
		for _, id := range anyStrings(p["delegatedPermissionIds"]) {
			for _, s := range sp.Scopes {
				if isID(s.ID, id) && !s.Disabled && !slices.Contains(out, s.Value) {
					out = append(out, s.Value)
				}
			}
		}
	}
	return out
}

// ---- what the portal does with the API of an app registration ----
//
// As for the app roles (roles.go): each change writes through UpdateApplication, so the rules and the history are those of a call of Graph.

// scopeProps are the properties of a scope that a person can set. The id is made.
var scopeProps = []string{"value", "type", "isEnabled", "adminConsentDisplayName", "adminConsentDescription", "userConsentDisplayName", "userConsentDescription"}

// APISettings gives what an application exposes: the URIs, the scopes, the clients that need no consent, and the token version.
func (w *World) APISettings(ref string) (Object, bool) {
	app, ok := w.GetApplication(ref)
	if !ok {
		return nil, false
	}
	api, _ := app["api"].(map[string]any)
	list := func(v any) any {
		if v == nil {
			return []any{}
		}
		return v
	}
	return Object{
		"identifierUris": list(app["identifierUris"]), "scopes": list(api["oauth2PermissionScopes"]),
		"preAuthorizedApplications": list(api["preAuthorizedApplications"]), "requestedAccessTokenVersion": api["requestedAccessTokenVersion"],
	}, true
}

// changeAPI reads the api of an application (cur), lets change say what it sets (set), and writes only that. The rest stays as it is, so a scope that goes away
// goes out of the clients that were pre-authorized for it (normalizeApplicationAPI).
func (w *World) changeAPI(ref string, change func(cur, set map[string]any) *apiError, a Actor) *apiError {
	app, ok := w.GetApplication(ref)
	if !ok {
		return notFound("application", ref)
	}
	cur, _ := app["api"].(map[string]any)
	set := map[string]any{}
	if e := change(cur, set); e != nil {
		return e
	}
	return w.UpdateApplication(ref, map[string]any{"api": set}, a)
}

func checkScopeProps(props map[string]any) *apiError {
	for k := range props {
		if !slices.Contains(scopeProps, k) {
			return badRequest("Property '%s' does not exist on a scope, or it cannot be set.", k)
		}
	}
	return nil
}

// AddScope adds a scope to an application. It gets an id, it is enabled, and it is for administrators unless the call says otherwise. It gives the scope as
// the data keeps it.
func (w *World) AddScope(ref string, props map[string]any, a Actor) (Object, *apiError) {
	if e := checkScopeProps(props); e != nil {
		return nil, e
	}
	scope := map[string]any{"id": newGUID(), "type": ScopeAdmin, "isEnabled": true}
	for k, v := range props {
		scope[k] = v
	}
	e := w.changeAPI(ref, func(cur, set map[string]any) *apiError {
		set["oauth2PermissionScopes"] = append(anySlice(cur["oauth2PermissionScopes"]), scope)
		return nil
	}, a)
	if e != nil {
		return nil, e
	}
	s, _ := w.APISettings(ref)
	for _, x := range anyObjects(s["scopes"]) {
		if str(x["id"]) == scope["id"] {
			return x, nil
		}
	}
	return nil, nil
}

// EditScope changes the properties that a call names, in one scope. A scope is disabled this way (isEnabled false).
func (w *World) EditScope(ref, scopeID string, props map[string]any, a Actor) *apiError {
	if e := checkScopeProps(props); e != nil {
		return e
	}
	return w.changeAPI(ref, func(cur, set map[string]any) *apiError {
		list := anyObjects(cur["oauth2PermissionScopes"])
		for _, s := range list {
			if isID(str(s["id"]), scopeID) {
				for k, v := range props {
					s[k] = v
				}
				set["oauth2PermissionScopes"] = toAnyList(list)
				return nil
			}
		}
		return notFound("scope", scopeID)
	}, a)
}

// RemoveScope takes a scope out of an application. As in Graph, the scope must have been disabled before.
func (w *World) RemoveScope(ref, scopeID string, a Actor) *apiError {
	return w.changeAPI(ref, func(cur, set map[string]any) *apiError {
		list := anyObjects(cur["oauth2PermissionScopes"])
		i := slices.IndexFunc(list, func(s map[string]any) bool { return isID(str(s["id"]), scopeID) })
		if i < 0 {
			return notFound("scope", scopeID)
		}
		set["oauth2PermissionScopes"] = toAnyList(slices.Delete(list, i, i+1))
		return nil
	}, a)
}

// SetIdentifierURIs sets the Application ID URIs of an application.
func (w *World) SetIdentifierURIs(ref string, uris []string, a Actor) *apiError {
	list := make([]any, len(uris))
	for i, u := range uris {
		list[i] = u
	}
	return w.UpdateApplication(ref, map[string]any{"identifierUris": list}, a)
}

// SetPreAuthorized makes a client need no consent for the scopes (by id), or changes them.
func (w *World) SetPreAuthorized(ref, clientAppID string, scopeIDs []string, a Actor) *apiError {
	if len(scopeIDs) == 0 {
		return badRequest("A pre-authorized client needs at least one scope.")
	}
	ids := make([]any, len(scopeIDs))
	for i, id := range scopeIDs {
		ids[i] = id
	}
	return w.changeAPI(ref, func(cur, set map[string]any) *apiError {
		list := anyObjects(cur["preAuthorizedApplications"])
		entry := map[string]any{"appId": clientAppID, "delegatedPermissionIds": ids}
		if i := slices.IndexFunc(list, func(m map[string]any) bool { return strings.EqualFold(str(m["appId"]), clientAppID) }); i >= 0 {
			list[i] = entry
		} else {
			list = append(list, entry)
		}
		set["preAuthorizedApplications"] = toAnyList(list)
		return nil
	}, a)
}

// RemovePreAuthorized takes a client out of the list.
func (w *World) RemovePreAuthorized(ref, clientAppID string, a Actor) *apiError {
	return w.changeAPI(ref, func(cur, set map[string]any) *apiError {
		list := anyObjects(cur["preAuthorizedApplications"])
		i := slices.IndexFunc(list, func(m map[string]any) bool { return strings.EqualFold(str(m["appId"]), clientAppID) })
		if i < 0 {
			return notFound("preAuthorizedApplication", clientAppID)
		}
		set["preAuthorizedApplications"] = toAnyList(slices.Delete(list, i, i+1))
		return nil
	}, a)
}
