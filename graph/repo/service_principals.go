package repo

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Create, change, and delete a service principal, and the object that Graph gives for one. A service principal is the instance of an app in a tenant. The
// object has the properties that the real Graph gives (checked with the sample tenant of Graph Explorer). Learn says which of them a call can set (the pages
// "Create servicePrincipal" and "Update serviceprincipal"). What Graph answers to a write cannot be tried there: the answers follow Learn (201, 204, 204), and
// the texts of the errors are guesses, except where a comment says that they are the real ones.

// Events that the writes make. A service principal that a client made by signing in has evSPCreate, with no Field. One made by POST has Field "graph".
const (
	evSPUpdate = "serviceprincipal.update" // Set has the properties that changed (AppID says which)
	evSPDelete = "serviceprincipal.delete"
)

// ServicePrincipalProps are the properties that a call can set, with their kinds. The rest is read-only.
var ServicePrincipalProps = map[string]propKind{
	"accountEnabled": kBool, "appRoleAssignmentRequired": kBool, "displayName": kString, "homepage": kString, "loginUrl": kString, "logoutUrl": kString,
	"notes": kString, "description": kString, "preferredSingleSignOnMode": kString, "preferredTokenSigningKeyThumbprint": kString, "tokenEncryptionKeyId": kString,
	"replyUrls": kList, "servicePrincipalNames": kList, "tags": kList, "notificationEmailAddresses": kList, "alternativeNames": kList, "addIns": kList,
	"appRoles": kList, "oauth2PermissionScopes": kList, "keyCredentials": kList, "samlSingleSignOnSettings": kObject, "customSecurityAttributes": kObject,
}

// the properties that cannot be null
var spNotNull = []string{"accountEnabled", "appRoleAssignmentRequired", "displayName", "replyUrls", "servicePrincipalNames", "tags", "notificationEmailAddresses",
	"alternativeNames", "addIns", "appRoles", "oauth2PermissionScopes", "keyCredentials"}

// the single sign-on modes of Learn
var ssoModes = []string{"password", "saml", "notSupported", "oidc"}

// the tenant that owns the apps of Microsoft (Microsoft Graph, the clients of Microsoft)
const microsoftTenantID = "f8cdef31-a31e-4b4a-93e4-5f571e91255a"

func (sp *ServicePrincipal) prop(name string, def any) any {
	if v, ok := sp.Props[name]; ok {
		return v
	}
	return def
}

// spObject gives the service principal as Graph writes it. A list leaves out the roles and the scopes of Microsoft Graph (full is false).
// Call it with the lock held.
func (w *World) spObject(sp *ServicePrincipal, full bool) Object {
	o := sp.object(full)
	created := sp.Created
	if created == "" {
		created = seedEpoch.Format("2006-01-02T15:04:05Z")
	}
	owner := stableGUID("dolly/appOwner/" + sp.AppID)
	switch {
	case w.appsByAppID[strings.ToLower(sp.AppID)] != nil:
		owner = str(w.Organization["id"]) // registered in this tenant
	case sp.Builtin != "" || sp.System:
		owner = microsoftTenantID
	}
	defaults := Object{
		"accountEnabled": true, "addIns": []any{}, "alternativeNames": []any{}, "appDescription": nil, "appDisplayName": sp.Name, "appOwnerOrganizationId": owner,
		"appRoleAssignmentRequired": false, "applicationTemplateId": nil, "createdByAppId": nil, "createdDateTime": created, "deletedDateTime": nil,
		"description": nil, "disabledByMicrosoftStatus": nil, "homepage": nil,
		"info":       Object{"logoUrl": nil, "marketingUrl": nil, "privacyStatementUrl": nil, "supportUrl": nil, "termsOfServiceUrl": nil},
		"isDisabled": nil, "keyCredentials": []any{}, "loginUrl": nil, "logoutUrl": nil, "notes": nil, "notificationEmailAddresses": []any{},
		"passwordCredentials": []any{}, "preferredSingleSignOnMode": nil, "preferredTokenSigningKeyThumbprint": nil, "replyUrls": []any{},
		"resourceSpecificApplicationPermissions": []any{}, "samlSingleSignOnSettings": nil, "servicePrincipalType": "Application", "signInAudience": "AzureADMyOrg",
		"tokenEncryptionKeyId": nil, "verifiedPublisher": Object{"displayName": nil, "verifiedPublisherId": nil, "addedDateTime": nil},
	}
	for k, v := range defaults {
		if _, has := o[k]; !has {
			o[k] = v
		}
	}
	for k, v := range sp.Props {
		o[k] = deepCopyValue(v)
	}
	return o
}

func deepCopyValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return deepCopy(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = deepCopyValue(e)
		}
		return out
	}
	return v
}

// findServicePrincipal finds a service principal by object ID or by client ID (the alternate key). Call it with the lock held.
func (w *World) findServicePrincipal(ref string) *ServicePrincipal {
	ref = strings.ToLower(ref)
	if sp := w.sps[ref]; sp != nil {
		return sp
	}
	return w.byAppID[ref]
}

// protected says why a service principal cannot be changed or deleted, or "".
func protected(sp *ServicePrincipal) string {
	switch {
	case sp.Builtin != "":
		return fmt.Sprintf("%s is built in, and its permissions are the list of dollycli: it cannot be changed or deleted.", sp.Name)
	case sp.System:
		return fmt.Sprintf("%s is a service principal that dollycli needs: it cannot be changed or deleted.", sp.Name)
	}
	return ""
}

// CreateServicePrincipal makes the instance of an app. It needs appId, and the app must be registered in this tenant (an app registration here), or be one of
// Microsoft. The service principal takes its name, its roles, its scopes, its identifier URIs, and its reply URLs from the application.
func (w *World) CreateServicePrincipal(props map[string]any, a Actor) (Object, *apiError) {
	body := maps.Clone(props)
	delete(body, "@odata.type")
	appID := strings.ToLower(strings.TrimSpace(str(body["appId"])))
	delete(body, "appId")
	switch {
	case appID == "":
		return nil, missing("appId")
	case !IsGUID(appID):
		return nil, badRequest("Invalid value specified for property 'appId' of resource 'ServicePrincipal'.")
	}
	for _, k := range []string{"id", "createdDateTime", "appOwnerOrganizationId", "servicePrincipalType", "appDisplayName"} {
		if _, has := body[k]; has {
			return nil, badRequest("Property '%s' is read-only.", k)
		}
	}
	if e := checkServicePrincipalBody(body); e != nil {
		return nil, e
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.byAppID[appID] != nil {
		return nil, badRequest("Another object with the same value for property appId already exists.")
	}
	app := w.appsByAppID[appID]
	if app == nil {
		// the text of the real Graph, for an appId that is not an application
		return nil, badRequest("The appId '%s' of the service principal does not reference a valid application object.", appID)
	}
	sp := &ServicePrincipal{ID: newGUID(), AppID: appID, Name: str(app["displayName"]), Created: w.now().UTC().Format("2006-01-02T15:04:05Z")}
	for _, u := range anyStrings(app["identifierUris"]) {
		sp.IdentifierURIs = append(sp.IdentifierURIs, u)
	}
	for _, r := range anyObjects(app["appRoles"]) {
		sp.Roles = append(sp.Roles, AppRole{ID: str(r["id"]), Name: str(r["displayName"])})
	}
	if apiObj, ok := app["api"].(map[string]any); ok {
		for _, s := range anyObjects(apiObj["oauth2PermissionScopes"]) {
			sp.Scopes = append(sp.Scopes, AppScope{ID: str(s["id"]), Value: str(s["value"]), Name: str(s["adminConsentDisplayName"]), Description: str(s["adminConsentDescription"])})
		}
	}
	var reply []any
	for _, key := range []string{"web", "spa", "publicClient"} {
		if o, ok := app[key].(map[string]any); ok {
			reply = append(reply, anySlice(o["redirectUris"])...)
		}
	}
	if len(reply) > 0 {
		sp.Props = map[string]any{"replyUrls": reply}
	}
	if e := applyServicePrincipalSet(sp, deepCopy(body)); e != nil {
		return nil, e
	}
	w.addServicePrincipal(sp)
	cp := *sp
	w.record(Event{Type: evSPCreate, Actor: a, AppID: appID, Field: "graph", SP: &cp})
	return w.spObject(sp, true), nil
}

func checkServicePrincipalBody(body map[string]any) *apiError {
	if e := checkProps(body, ServicePrincipalProps, "ServicePrincipal"); e != nil {
		return e
	}
	for _, k := range spNotNull {
		if v, has := body[k]; has && v == nil {
			return badRequest("Property '%s' cannot be null.", k)
		}
	}
	if v, has := body["preferredSingleSignOnMode"]; has && v != nil {
		if s, _ := v.(string); !slices.Contains(ssoModes, s) {
			return badRequest("Invalid value specified for property 'preferredSingleSignOnMode' of resource 'ServicePrincipal'. Use %s.", strings.Join(ssoModes, ", "))
		}
	}
	return nil
}

// applyServicePrincipalSet sets properties of a change on a service principal. The properties that the type keeps (name, tags, roles, scopes, names) go
// to their fields, and the rest to Props. A change that was checked cannot fail, so a replay never does.
func applyServicePrincipalSet(sp *ServicePrincipal, set map[string]any) *apiError {
	for k, v := range set {
		switch k {
		case "displayName":
			sp.Name = str(v)
		case "tags":
			sp.Enterprise, sp.ExtraTags = false, nil
			for _, t := range anyStrings(v) {
				switch {
				case t == EnterpriseTag:
					sp.Enterprise = true
				case t != "dolly:auto" && !slices.Contains(sp.ExtraTags, t):
					sp.ExtraTags = append(sp.ExtraTags, t)
				}
			}
		case "servicePrincipalNames":
			sp.IdentifierURIs = nil
			for _, n := range anyStrings(v) {
				if !strings.EqualFold(n, sp.AppID) {
					sp.IdentifierURIs = append(sp.IdentifierURIs, n)
				}
			}
		case "appRoles":
			sp.Roles = nil
			for _, r := range anyObjects(v) {
				id := firstNonEmpty(str(r["id"]), newGUID())
				sp.Roles = append(sp.Roles, AppRole{ID: id, Name: firstNonEmpty(str(r["displayName"]), str(r["value"]))})
			}
		case "oauth2PermissionScopes":
			sp.Scopes = nil
			for _, s := range anyObjects(v) {
				id := firstNonEmpty(str(s["id"]), newGUID())
				sp.Scopes = append(sp.Scopes, AppScope{ID: id, Value: str(s["value"]), Name: firstNonEmpty(str(s["adminConsentDisplayName"]), str(s["value"])), Description: str(s["adminConsentDescription"])})
			}
		default:
			if sp.Props == nil {
				sp.Props = map[string]any{}
			}
			sp.Props[k] = deepCopyValue(v)
		}
	}
	return nil
}

// UpdateServicePrincipal changes properties of a service principal, found by object ID or client ID. Gives nil when it worked (the answer is 204).
func (w *World) UpdateServicePrincipal(ref string, props map[string]any, a Actor) *apiError {
	body := maps.Clone(props)
	delete(body, "@odata.type")
	for k := range body {
		if _, writable := ServicePrincipalProps[k]; !writable {
			if _, known := spObjectProps[k]; known {
				return badRequest("Property '%s' cannot be changed.", k)
			}
		}
	}
	if e := checkServicePrincipalBody(body); e != nil {
		return e
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	sp := w.findServicePrincipal(ref)
	if sp == nil {
		return notFound("servicePrincipal", ref)
	}
	if why := protected(sp); why != "" {
		return badRequest("%s", why)
	}
	if len(body) == 0 {
		return nil
	}
	applyServicePrincipalSet(sp, deepCopy(body))
	w.record(Event{Type: evSPUpdate, Actor: a, AppID: sp.AppID, PrincipalName: sp.Name, Set: deepCopy(body), SP: &ServicePrincipal{ID: sp.ID, AppID: sp.AppID}})
	return nil
}

// spObjectProps are the names of the properties of the object that a call cannot set: Graph says "cannot be changed" for them, and "does not exist" for another name.
var spObjectProps = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range []string{"id", "appId", "appDisplayName", "appOwnerOrganizationId", "applicationTemplateId", "createdByAppId", "createdDateTime", "deletedDateTime",
		"disabledByMicrosoftStatus", "info", "isDisabled", "passwordCredentials", "resourceSpecificApplicationPermissions", "servicePrincipalType", "signInAudience",
		"verifiedPublisher", "appDescription"} {
		m[k] = true
	}
	return m
}()

// DeleteServicePrincipal takes a service principal away, with the role assignments and the consents that it was part of. Its app registration stays.
func (w *World) DeleteServicePrincipal(ref string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	sp := w.findServicePrincipal(ref)
	if sp == nil {
		return notFound("servicePrincipal", ref)
	}
	if why := protected(sp); why != "" {
		return badRequest("%s", why)
	}
	gone := &ServicePrincipal{ID: sp.ID, AppID: sp.AppID}
	w.removeServicePrincipal(sp.ID)
	w.record(Event{Type: evSPDelete, Actor: a, AppID: gone.AppID, PrincipalName: sp.Name, SP: gone})
	return nil
}

// removeServicePrincipal takes a service principal out of the data, and what refers to it: the role assignments (as the resource, as the principal), and the
// consents (as the client, as the resource). Call it with the lock held.
func (w *World) removeServicePrincipal(id string) {
	sp := w.sps[strings.ToLower(id)]
	if sp == nil {
		return
	}
	delete(w.sps, strings.ToLower(sp.ID))
	delete(w.byAppID, strings.ToLower(sp.AppID))
	w.SPs = slices.DeleteFunc(w.SPs, func(x *ServicePrincipal) bool { return x == sp })
	w.Assignments = slices.DeleteFunc(w.Assignments, func(x Assignment) bool { return isID(x.ResourceID, sp.ID) || isID(x.PrincipalID, sp.ID) })
	w.Grants = slices.DeleteFunc(w.Grants, func(g Grant) bool { return isID(g.ClientID, sp.ID) || isID(g.ResourceID, sp.ID) })
}

// ---- the history ----

func servicePrincipalWriteText(e Event) (string, bool) {
	who := firstNonEmpty(e.PrincipalName, e.AppID)
	switch e.Type {
	case evSPUpdate:
		return fmt.Sprintf("Changed the service principal %s: %s", who, setText(e.Set)), true
	case evSPDelete:
		return fmt.Sprintf("Deleted the service principal %s", who), true
	}
	return "", false
}

func (w *World) applyServicePrincipalWriteEvent(e Event) bool {
	switch e.Type {
	case evSPUpdate:
		if sp := w.spByAppID(e.AppID); sp != nil {
			applyServicePrincipalSet(sp, deepCopy(e.Set))
		}
	case evSPDelete:
		if sp := w.spByAppID(e.AppID); sp != nil {
			w.removeServicePrincipal(sp.ID)
		}
	default:
		return false
	}
	return true
}

// ---- small helpers for values that came from JSON ----

func anySlice(v any) []any {
	l, _ := v.([]any)
	return l
}

func anyStrings(v any) []string {
	var out []string
	for _, x := range anySlice(v) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func anyObjects(v any) []map[string]any {
	var out []map[string]any
	for _, x := range anySlice(v) {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
